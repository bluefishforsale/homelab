#!/usr/bin/env python3
"""Re-download non-conforming movies in batches, keeping Plex's "date added".

About half the library has no English subtitle (the old tdarr flow stripped
them with -sn) or was muxed twice, and only a fresh download fixes that. This
walks the "refetch" bucket from media-conformance.py a batch at a time. Every
item goes through a ledger, so a run can stop anywhere and resume:

  planned   Plex rating key and original added date recorded
  grabbed   the old file deleted and the best acceptable release grabbed
  imported  Radarr imported a file that has an English subtitle; a release
            without one is blocklisted and the next is tried, up to 3, after
            which the last download is kept and flagged no-subs
  done      tdarr has run the flow over it and Plex's added date is restored

The old file is only deleted once a search has found a release to replace it.

  media-refetch.py plan --movies 25 --max-gb 2000   # pick the next batch (no changes)
  media-refetch.py run                              # what the next step would do
  media-refetch.py run --yes                        # do it; re-run until all done
  media-refetch.py status

Order: double-muxed files first (they are broken today), then most played in
Tautulli, then the rest. Movies only for now; TV comes as season packs.
"""
import argparse
import datetime as dt
import fcntl
import importlib.util
import json
import os
import re
import shlex
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import media_clients as mc  # noqa: E402

_spec = importlib.util.spec_from_file_location("conformance", os.path.join(HERE, "media-conformance.py"))
conformance = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(conformance)

LEDGER = os.path.expanduser(os.environ.get("MEDIA_REFETCH_LEDGER", "~/.local/state/media-refetch.json"))
HOST = os.environ.get("MEDIA_HOST", "ocean.home")
PLEX_DB = ("/config/Library/Application Support/Plex Media Server/"
           "Plug-in Support/Databases/com.plexapp.plugins.library.db")
MAX_ATTEMPTS = 3
GRAB_TIMEOUT = dt.timedelta(hours=12)
# With a file on disk every release carries one of these; a release whose only
# rejections are these is one Radarr would take if the file were gone.
EXISTING_FILE = "Existing file on disk"
# Not library content; re-downloading would not fix it and nothing should.
JUNK = re.compile(r"(/sample\.mkv$|-sample\.mkv$|/Plex Versions/|/\.[^/]+$|\.iso$|/VIDEO_TS/)", re.I)


def now():
    return dt.datetime.now(dt.timezone.utc)


def load():
    try:
        with open(LEDGER) as f:
            return json.load(f)
    except FileNotFoundError:
        return {}


def save(ledger):
    os.makedirs(os.path.dirname(LEDGER), exist_ok=True)
    tmp = LEDGER + ".tmp"
    with open(tmp, "w") as f:
        json.dump(ledger, f, indent=2, sort_keys=True)
    os.replace(tmp, LEDGER)


def ssh(cmd, stdin=None):
    return subprocess.run(["ssh", "-o", "BatchMode=yes", HOST, cmd], input=stdin,
                          capture_output=True, text=True, check=True).stdout


def plex_items():
    """Plex path -> (rating key, added_at) for every media part."""
    sql = ("SELECT mp.file, md.id, md.added_at FROM media_parts mp "
           "JOIN media_items mi ON mi.id = mp.media_item_id "
           "JOIN metadata_items md ON md.id = mi.metadata_item_id;")
    out = ssh("sudo -n docker exec -i plex '/usr/lib/plexmediaserver/Plex SQLite' -readonly "
              + shlex.quote(PLEX_DB), stdin=sql)
    items = {}
    for line in out.splitlines():
        path, rk, added = line.rsplit("|", 2)
        items[path] = (int(rk), int(added))
    return items


def plays():
    """Plex rating key -> play count, from Tautulli."""
    counts = {}
    libs = mc.get("tautulli", "/api/v2", cmd="get_libraries")["response"]["data"]
    for lib in libs:
        if lib.get("section_type") != "movie":
            continue
        rows = mc.get("tautulli", "/api/v2", cmd="get_library_media_info",
                      section_id=lib["section_id"], length=100000)["response"]["data"]["data"]
        counts.update({int(r["rating_key"]): int(r.get("play_count") or 0) for r in rows})
    return counts


def subs(file):
    """Subtitle languages in a file, by its tdarr-node path (/media/...)."""
    out = ssh("sudo -n docker exec tdarr-node ffprobe -v error -select_streams s "
              "-show_entries stream_tags=language -of csv=p=0 " + shlex.quote(file))
    return [line.strip().lower() for line in out.splitlines()]


def probe(file):
    out = ssh("sudo -n docker exec tdarr-node ffprobe -v error -show_streams -of json " + shlex.quote(file))
    return json.loads(out)["streams"]


def best_release(movie_id):
    rels = mc.get("radarr", "/api/v3/release", movieId=movie_id)
    ok = [r for r in rels if all(x.startswith(EXISTING_FILE) for x in r.get("rejections", []))
          and r.get("downloadAllowed")]
    return min(ok, key=lambda r: r.get("releaseWeight", 0)) if ok else None


def cmd_plan(a):
    ledger = load()
    movies = [m for m in mc.get("radarr", "/api/v3/movie") if m.get("hasFile")]
    radarr = {os.path.join(m["path"].rstrip("/"), m["movieFile"]["relativePath"]): m for m in movies}
    folders = {m["path"].rstrip("/") for m in movies}
    plex = plex_items()
    count = plays()
    todo, skipped = [], {}
    for f in conformance.sweep():
        if f["library"] != "movies" or f["bucket"] != "refetch" or f["file"] in ledger:
            continue
        path = f["file"].removeprefix("/media")      # tdarr /media/movies/... -> /movies/...
        movie = radarr.get(path)
        if JUNK.search(path):
            why = "junk (sample, disc image, Plex Versions, hidden temp file)"
        elif not movie and os.path.dirname(path) in folders:
            why = "extra copy beside the file Radarr tracks"
        elif not movie:
            why = "not in Radarr"
        elif path not in plex:
            why = "not in Plex"
        else:
            why = None
        if why:
            skipped.setdefault(why, []).append(path)
            continue
        rk, added = plex[path]
        todo.append({"file": f["file"], "path": path, "reasons": f["reasons"], "size_gb": f["size_gb"],
                     "movie_id": movie["id"], "title": f"{movie['title']} ({movie['year']})",
                     "rating_key": rk, "added_at": added, "plays": count.get(rk, 0)})
    for why, paths in sorted(skipped.items()):
        print(f"skip   {len(paths):4}  {why}")
        for p in paths if a.show_skipped else []:
            print(f"         {p}")
    todo.sort(key=lambda t: ("double-muxed" not in t["reasons"], -t["plays"], t["title"]))
    batch, gb = [], 0.0
    for t in todo:
        if len(batch) >= a.movies or gb + t["size_gb"] > a.max_gb:
            break
        batch.append(t)
        gb += t["size_gb"]
    for t in batch:
        added = dt.datetime.fromtimestamp(t["added_at"]).date()
        print(f"plan   {t['title']}: {','.join(t['reasons'])}, {t['plays']} plays, added {added}")
        ledger[t["file"]] = {**t, "state": "planned", "attempts": 0, "log": []}
    save(ledger)
    print(f"{len(batch)} planned ({gb:.0f} GB on disk now), {len(todo) - len(batch)} more waiting")


def note(item, msg):
    item["log"].append(f"{now().isoformat(timespec='seconds')} {msg}")
    print(f"{item['state']:8} {item['title']}: {msg}")


def step(item, yes):
    """Advance one item by one state. Returns True when something changed."""
    mid = item["movie_id"]
    if item["state"] == "planned":
        # Marking a grab failed can make Radarr search and grab on its own;
        # adopt that download rather than grabbing a second one.
        # NOTE: /queue ignores movieId and returns every download; /queue/details filters.
        queued = mc.get("radarr", "/api/v3/queue/details", movieId=mid)
        if queued:
            note(item, f"already downloading {queued[0]['title']}; following it")
            if yes:
                item.update(state="grabbed", grabbed_at=now().isoformat(), release=queued[0]["title"])
            return yes
        rel = best_release(mid)
        if not rel:
            note(item, "no acceptable release found; left as is")
            item["state"] = "no-release"
            return True
        note(item, f"grab {rel['title']} ({rel['size'] / 1e9:.1f} GB, {rel['indexer']})")
        if not yes:
            return False
        movie = mc.get("radarr", f"/api/v3/movie/{mid}")
        if movie.get("hasFile"):
            mc.delete("radarr", f"/api/v3/moviefile/{movie['movieFile']['id']}")
        mc.post("radarr", "/api/v3/release", {"guid": rel["guid"], "indexerId": rel["indexerId"]})
        item.update(state="grabbed", grabbed_at=now().isoformat(), release=rel["title"])
        return True

    if item["state"] == "grabbed":
        movie = mc.get("radarr", f"/api/v3/movie/{mid}")
        grabbed = dt.datetime.fromisoformat(item["grabbed_at"])
        mf = movie.get("movieFile") or {}
        if not movie.get("hasFile") or dt.datetime.fromisoformat(mf["dateAdded"].replace("Z", "+00:00")) < grabbed:
            # A failed download (missing articles) is blocklisted by Radarr, but
            # its own re-search may find nothing it will take; search again here.
            failed = [h for h in mc.get("radarr", "/api/v3/history/movie", movieId=mid, eventType=4)  # 4 = downloadFailed
                      if dt.datetime.fromisoformat(h["date"].replace("Z", "+00:00")) > grabbed]
            if failed and not mc.get("radarr", "/api/v3/queue/details", movieId=mid):
                note(item, f"download failed ({failed[-1].get('sourceTitle', '')[:60]}); searching again")
                if yes:
                    item["state"] = "planned"
                return yes
            if now() - grabbed > GRAB_TIMEOUT:
                note(item, f"nothing imported after {GRAB_TIMEOUT}; check the Radarr queue")
                item["state"] = "stuck"
                return True
            return False
        path = movie["path"] + "/" + mf["relativePath"]
        langs = subs("/media" + path)
        item["attempts"] += 1
        if any(lang in conformance.ENGLISH for lang in langs):
            note(item, f"imported {mf['relativePath']} with subtitles {sorted(set(langs))}")
            item.update(state="imported", file="/media" + path, path=path)
            return True
        if item["attempts"] >= MAX_ATTEMPTS:
            note(item, f"attempt {item['attempts']} still has no English subtitle; keeping it")
            item.update(state="imported", file="/media" + path, path=path, no_subs=True)
            return True
        note(item, f"attempt {item['attempts']} has no English subtitle; blocklisting and trying the next")
        if not yes:
            return False
        hist = mc.get("radarr", "/api/v3/history/movie", movieId=mid, eventType=1)  # 1 = grabbed
        latest = max(hist, key=lambda h: h["date"])
        mc.delete("radarr", f"/api/v3/moviefile/{mf['id']}")
        mc.post("radarr", f"/api/v3/history/failed/{latest['id']}", {})
        item["state"] = "planned"
        return True

    if item["state"] == "imported":
        why = conformance.reasons(probe(item["file"]))
        if "no-dialogue-track" in why or "video-not-hevc" in why:
            return False                               # tdarr has not run the flow yet
        added = dt.datetime.fromtimestamp(item["added_at"]).date()
        note(item, f"conforms ({','.join(why) or 'clean'}); restore Plex added date {added}")
        if not yes:
            return False
        plex_md = mc.get("plex", f"/library/metadata/{item['rating_key']}")["MediaContainer"]["Metadata"][0]
        mc._request("plex", f"/library/sections/{plex_md['librarySectionID']}/all",
                    {"type": 1, "id": item["rating_key"], "addedAt.value": item["added_at"],
                     "addedAt.locked": 1}, method="PUT")
        item["state"] = "done"
        return True
    return False


def cmd_run(a):
    ledger = load()
    for item in ledger.values():
        while step(item, a.yes):
            save(ledger)
            if not a.yes:
                break
    if not a.yes:
        print("dry run: re-run with --yes to act")


def cmd_status(a):
    ledger = load()
    by = {}
    for item in ledger.values():
        by.setdefault(item["state"], []).append(item)
    for state, items in sorted(by.items()):
        print(f"{state:10} {len(items)}")
        if a.verbose:
            for i in items:
                print(f"   {i['title']}: {i['log'][-1] if i['log'] else ''}")


def lock():
    """Hold an exclusive lock for this process's lifetime, or exit."""
    os.makedirs(os.path.dirname(LEDGER), exist_ok=True)
    fd = open(LEDGER + ".lock", "w")
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        sys.exit("another media-refetch.py is running against this ledger")
    return fd


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("plan")
    p.add_argument("--movies", type=int, default=25)
    p.add_argument("--max-gb", type=float, default=2000)
    p.add_argument("--show-skipped", action="store_true", help="list every skipped path")
    p.set_defaults(fn=cmd_plan)
    r = sub.add_parser("run")
    r.add_argument("--yes", action="store_true")
    r.set_defaults(fn=cmd_run)
    s = sub.add_parser("status")
    s.add_argument("-v", "--verbose", action="store_true")
    s.set_defaults(fn=cmd_status)
    a = ap.parse_args()
    # NOTE: two runs over one ledger both see an item as planned and both grab
    # a release; this happened on batch 1 (Sinners grabbed twice, seconds apart).
    _held = lock()  # noqa: F841
    a.fn(a)


if __name__ == "__main__":
    main()
