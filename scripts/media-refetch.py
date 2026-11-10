#!/usr/bin/env python3
"""Re-download non-conforming movies and TV in batches, keeping Plex's "date added".

About half the library has no English subtitle (the old tdarr flow stripped
them with -sn) or was muxed twice, and only a fresh download fixes that. This
walks the "refetch" bucket from media-conformance.py a batch at a time. Every
item goes through a ledger, so a run can stop anywhere and resume:

  planned   Plex rating key and original added date recorded
  grabbed   the old file deleted and the best acceptable release grabbed
  imported  Radarr/Sonarr imported files that have an English subtitle; a release
            without one is blocklisted and the next is tried, up to 3, after
            which the last download is kept and flagged no-subs
  done      tdarr has run the flow over it and Plex's added date is restored

The old file is only deleted once Radarr or Sonarr has accepted a grab to replace it.

  media-refetch.py plan --movies 25 --max-gb 2000 --tv-gb 1000   # pick the next batch
  media-refetch.py run                              # what the next step would do
  media-refetch.py run --yes                        # do it; re-run until all done
  media-refetch.py status
  media-refetch.py auto --yes                       # unattended: plan when idle, then run

Order: double-muxed files first (they are broken today), then most played in
Tautulli, then the rest.

TV goes a season at a time. A season at least half bad comes back as one season
pack, which replaces every episode in it; otherwise, or when the season is
still airing or no pack is acceptable, each bad episode is searched on its own.
Specials are left alone.
"""
import argparse
import collections
import concurrent.futures
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
# Inside tdarr.py drive's 7-day horizon, so the flow has had its chance.
FLOW_TIMEOUT = dt.timedelta(days=3)
# Marking a grab failed makes the *arr search and grab on its own; give that
# grab this long to reach the queue before searching ourselves.
REDOWNLOAD_GRACE = dt.timedelta(minutes=30)
# With a file on disk every release carries one of these; a release whose only
# rejections are these is one Radarr would take if the file were gone.
EXISTING_FILE = "Existing file on disk"
# Not library content; re-downloading would not fix it and nothing should.
JUNK = re.compile(r"(/sample\.mkv$|-sample\.mkv$|/Plex Versions/|/\.[^/]+$|\.iso$|/VIDEO_TS/)", re.I)
# A bonus disc Sonarr parses as the whole season. It imports nothing, and the
# season's old files are gone by then.
EXTRAS = re.compile(r"\bextras\b", re.I)
# The same disc also comes without the word. An SD DVDRip runs 170-350 MB an
# episode; under this, the release cannot hold what it would replace.
MIN_EPISODE_BYTES = 50e6
TV_SEASON = re.compile(r"^/tv/([^/]+)/Season (\d+)/")
# NOTE: this Sonarr release profile keeps Sonarr's own searches off season
# packs. Packs are grabbed past it on purpose, a season at a time; Sonarr puts
# its ignored-term rejection on every pack, and only that rejection is let by.
PACK_PROFILE = "No season packs"
# A season at least this bad comes back as a pack. Below it only the bad
# episodes are searched, since a pack also replaces the good ones.
PACK_SHARE = 0.5
PLEX_MOVIE, PLEX_EPISODE = 1, 4


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
    """Plex path -> (rating key, added_at, show rating key or None) for every media part."""
    sql = ("SELECT mp.file, md.id, md.added_at, IFNULL(season.parent_id, '') FROM media_parts mp "
           "JOIN media_items mi ON mi.id = mp.media_item_id "
           "JOIN metadata_items md ON md.id = mi.metadata_item_id "
           "LEFT JOIN metadata_items season ON season.id = md.parent_id;")
    out = ssh("sudo -n docker exec -i plex '/usr/lib/plexmediaserver/Plex SQLite' -readonly "
              + shlex.quote(PLEX_DB), stdin=sql)
    items = {}
    for line in out.splitlines():
        path, rk, added, show = line.rsplit("|", 3)
        items[path] = (int(rk), int(added), int(show) if show else None)
    return items


def restore_added(rk, added_at, plex_type):
    """Put a Plex item's added date back and lock it."""
    md = mc.get("plex", f"/library/metadata/{rk}")["MediaContainer"]["Metadata"][0]
    mc._request("plex", f"/library/sections/{md['librarySectionID']}/all",
                {"type": plex_type, "id": rk, "addedAt.value": added_at, "addedAt.locked": 1}, method="PUT")


def plays():
    """Plex rating key -> play count, from Tautulli; a show counts every episode's plays."""
    counts = {}
    libs = mc.get("tautulli", "/api/v2", cmd="get_libraries")["response"]["data"]
    for lib in libs:
        if lib.get("section_type") not in ("movie", "show"):
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


def flowed(file):
    """Where a file is now: tdarr's flow remuxes an .mp4 or .avi to an .mkv beside it."""
    mkv = os.path.splitext(file)[0] + ".mkv"
    out = ssh("sudo -n docker exec tdarr-node ls -d -- " + shlex.quote(file) + " " + shlex.quote(mkv)
              + " 2>/dev/null; true")
    return next(iter(out.splitlines()), file)


def needs_bump(record):
    return bool(record) and record["TranscodeDecisionMaker"] == "Queued" and not record["bumped"]


def transcoding_now():
    """Files a tdarr worker is transcoding right now, by tdarr-node path."""
    nodes = mc.get("tdarr", "/api/v2/get-nodes").values()
    return {w.get("file") for n in nodes for w in n["workers"].values()
            if (w.get("workerType") or "").startswith("transcode")}


def tdarr_files(mode, **kw):
    return mc.post("tdarr", "/api/v2/cruddb", {"data": {"collection": "FileJSONDB", "mode": mode, **kw}})


# 2100-01-01 in ms. tdarr takes the highest bumped value first, so counting down
# from here keeps refetches first-in-first-out and ahead of the UI's bump
# button, which writes the current time.
BUMP_CEILING = 4102444800000


def bump_value(t):
    return BUMP_CEILING - int(t.timestamp() * 1000)


def bump(item, files, yes):
    """Put an item's files at the front of tdarr's queue, ahead of the library backlog."""
    todo = [f for f in files if needs_bump(tdarr_files("getById", docID=f))]
    if not todo:
        return False
    note(item, f"bump {len(todo)} file(s) to the front of the tdarr queue")
    if not yes:
        return False
    for f in todo:
        tdarr_files("update", docID=f, obj={"bumped": bump_value(dt.datetime.now())})
    return True


def pick(rels):
    """The release Radarr or Sonarr ranks first.

    The codec preference lives in their custom formats (files/ocean-recyclarr):
    TV ranks x265 10-bit SDR first, movies rank x265 last. A second opinion here
    would fight them."""
    return min(rels, key=lambda r: r.get("releaseWeight", 0)) if rels else None


def same_film(title, year):
    """False when the release names only other years: Radarr maps a remake's release onto the original."""
    years = {int(y) for y in re.findall(r"\b(19\d\d|20\d\d)\b", title)}
    return not years or any(abs(y - year) <= 1 for y in years)    # festival vs release year


def best_release(movie_id):
    rels = mc.get("radarr", "/api/v3/release", movieId=movie_id)
    year = mc.get("radarr", f"/api/v3/movie/{movie_id}")["year"]
    ok = [r for r in rels if all(x.startswith(EXISTING_FILE) for x in r.get("rejections", []))
          and r.get("downloadAllowed") and same_film(r["title"], year)]
    return pick(ok)


def season_mode(bad, have, airing):
    """'pack' or 'episodes', for a season with `bad` of its `have` episode files failing."""
    if airing:
        return "episodes"      # a pack for a season still airing is usually missing episodes
    return "pack" if bad >= PACK_SHARE * have else "episodes"


def acceptable(rel, allowed=()):
    """A release Sonarr turned down only for the file on disk, or for a reason in `allowed`."""
    # Sonarr: "Existing file meets cutoff", "Existing file on disk is of equal or higher preference".
    return bool(rel.get("downloadAllowed")) and all(
        x.startswith("Existing file") or x in allowed for x in rel.get("rejections", []))


def pack_rejection(profiles):
    """The rejection PACK_PROFILE puts on every season pack; empty if the profile is gone."""
    # The UI shows "No season packs"; the API has it as "no season packs".
    p = next((p for p in profiles if (p.get("name") or "").lower() == PACK_PROFILE.lower()), None)
    return {f"Contains these ignored terms: {', '.join(p['ignored'])}"} if p else set()


# A season search goes to every indexer live and comes back with thousands of
# releases; Young Sheldon S01 took longer than the client's default 120s.
SEARCH_TIMEOUT = 600
# An anime season search is the pack query plus one search per episode, each
# about 50-75s (DAN DA DAN S02: 13 searches, 656s on 2026-10-24).
EPISODE_SEARCH = 90


def show_name(title):
    """A series title as a person reads it: no case, punctuation, year, leading The or season."""
    t = re.sub(r"^\W*the\b", "", title.lower().replace("&", " and "))
    t = re.sub(r"\b(\d+(st|nd|rd|th) season|season \d+)\b", "", t)     # anime: "Dandadan 2nd Season"
    bare = re.sub(r"\b(19|20)\d\d\b", "", t)
    return re.sub(r"[^a-z0-9]", "", bare if re.search(r"[a-z0-9]", bare) else t)    # 1883 is all year


def same_show(rel, series):
    # NOTE: Sonarr trusts an indexer's TVDB id over the release name, so a release
    # of another show tagged with this one's id comes back with no rejection.
    names = {show_name(n) for n in [series["title"], *(a["title"] for a in series.get("alternateTitles", []))]}
    return show_name(rel.get("seriesTitle") or "") in names


def best_tv_release(item):
    if item["pack"]:
        rels = mc.get("sonarr", "/api/v3/release", seriesId=item["series_id"], seasonNumber=item["season"],
                      timeout=SEARCH_TIMEOUT + EPISODE_SEARCH * len(item["episode_ids"]))
        allowed = pack_rejection(mc.get("sonarr", "/api/v3/releaseprofile"))
        ok = [r for r in rels if r.get("fullSeason") and acceptable(r, allowed)]
    else:
        rels = mc.get("sonarr", "/api/v3/release", episodeId=item["episode_ids"][0], timeout=SEARCH_TIMEOUT)
        ok = [r for r in rels if not r.get("fullSeason") and acceptable(r)]
    series = mc.get("sonarr", f"/api/v3/series/{item['series_id']}")
    ok = [r for r in ok if not EXTRAS.search(r["title"]) and same_show(r, series)
          and r["size"] >= MIN_EPISODE_BYTES * len(item["episode_ids"])]
    return pick(ok)


def tv_files(sid, eps):
    """Episode id -> its Sonarr episode file, for those of `eps` that have one."""
    files = {f["id"]: f for f in mc.get("sonarr", "/api/v3/episodefile", seriesId=sid)}
    return {e["id"]: files[e["episodeFileId"]] for e in mc.get("sonarr", "/api/v3/episode", seriesId=sid)
            if e["id"] in eps and e.get("episodeFileId") in files}


def when(stamp):
    return dt.datetime.fromisoformat(stamp.replace("Z", "+00:00"))


def tv_held(ledger, relist=False):
    """Episode ids and season keys a TV plan leaves alone.

    A hand-made list (relist) re-plans finished items: Charmed S03 was refetched,
    then squeezed by the 1.2 Mbps flow before the 10-bit target landed."""
    live = [(k, i) for k, i in ledger.items() if i.get("kind") == "tv" and not (relist and i.get("state") == "done")]
    return ({e for _, i in live for e in i["episode_ids"]},
            {k for k, i in live if k.startswith("tv:")})


def plan_tv(a, refetch, ledger, plex, count, relist=False):
    series = {os.path.basename(s["path"].rstrip("/")): s for s in mc.get("sonarr", "/api/v3/series")}
    held, held_seasons = tv_held(ledger, relist)
    groups, skipped = {}, collections.Counter()
    for f in refetch:
        if f["library"] != "tv":
            continue
        path = f["file"].removeprefix("/media")      # tdarr /media/tv/... -> /tv/...
        m = TV_SEASON.match(path)
        if JUNK.search(path):
            skipped["junk"] += 1
        elif not m or int(m.group(2)) == 0:
            skipped["specials, or not in a season folder"] += 1
        elif m.group(1) not in series:
            skipped["not in Sonarr"] += 1
        elif f"tv:{series[m.group(1)]['id']}:{m.group(2)}" in held_seasons:
            pass                                       # the season is already in a batch
        else:
            groups.setdefault((m.group(1), int(m.group(2))), []).append({**f, "path": path})
    seasons = []
    for (folder, sn), files in groups.items():
        s = series[folder]
        stats = next((x["statistics"] for x in s["seasons"] if x["seasonNumber"] == sn), {})
        airing = s["status"] == "continuing" and sn == max(x["seasonNumber"] for x in s["seasons"])
        pack = season_mode(len(files), stats.get("episodeFileCount", len(files)), airing) == "pack"
        show = next((plex[f["path"]][2] for f in files if f["path"] in plex), None)
        seasons.append({"series": s, "season": sn, "files": files, "pack": pack, "plays": count.get(show, 0),
                        "double": any("double-muxed" in f["reasons"] for f in files),
                        "size_gb": stats.get("sizeOnDisk", 0) / 2**30 if pack else sum(f["size_gb"] for f in files)})
    seasons.sort(key=lambda t: (not t["double"], -t["plays"], t["series"]["title"], t["season"]))

    gb = 0.0
    for t in seasons:
        if gb + t["size_gb"] > a.tv_gb:
            break
        s, sn = t["series"], t["season"]
        eps = [e for e in mc.get("sonarr", "/api/v3/episode", seriesId=s["id"]) if e["seasonNumber"] == sn]
        ids = {}
        for e in eps:
            ids.setdefault(e.get("episodeFileId"), []).append(e)
        by_path = {f["path"]: sorted(ids[f["id"]], key=lambda e: e["episodeNumber"])
                   for f in mc.get("sonarr", "/api/v3/episodefile", seriesId=s["id"]) if f["id"] in ids}

        def label(es):
            return f"{s['title']} S{sn:02d}" + "-".join(f"E{e['episodeNumber']:02d}" for e in es)

        bad = []
        for f in t["files"]:
            es = by_path.get(f["path"])
            if not es or f["path"] not in plex:
                skipped["not matched to a Sonarr episode and a Plex item"] += 1
            elif any(e["id"] in held for e in es):
                pass
            else:
                bad.append({"file": f["file"], "path": f["path"], "reasons": f["reasons"], "size_gb": f["size_gb"],
                            "episode_ids": [e["id"] for e in es], "title": label(es),
                            "added": {str(e["id"]): plex[f["path"]][1] for e in es}})
        if not bad:
            continue
        common = {"kind": "tv", "series_id": s["id"], "season": sn, "plays": t["plays"],
                  "state": "planned", "attempts": 0, "log": []}
        if t["pack"]:
            # The pack replaces every episode in the season, so every one gets its date back.
            added = {str(e["id"]): plex[p][1] for p, es in by_path.items() if p in plex for e in es}
            ledger[f"tv:{s['id']}:{sn}"] = {
                **common, "pack": True, "title": f"{s['title']} S{sn:02d}", "bad": bad,
                "episode_ids": sorted(e["id"] for es in by_path.values() for e in es), "added": added,
                "reasons": sorted({r for b in bad for r in b["reasons"]}), "size_gb": round(t["size_gb"], 1)}
            print(f"plan   {s['title']} S{sn:02d}: pack, {len(bad)} of {len(by_path)} files bad, "
                  f"{t['plays']} plays, {t['size_gb']:.0f} GB")
        else:
            for b in bad:
                ledger[b["file"]] = {**common, **b, "pack": False}
            print(f"plan   {s['title']} S{sn:02d}: {len(bad)} episodes one at a time, {t['plays']} plays")
        gb += t["size_gb"]
    for why, n in sorted(skipped.items()):
        print(f"skip   {n:4}  tv: {why}")
    print(f"tv: {gb:.0f} GB planned, of {len(seasons)} seasons still to re-download")


def arr_hdr_paths(movies, episode_files):
    """tdarr paths of the files Radarr or Sonarr recorded as HDR or Dolby Vision."""
    hdr = lambda f: bool((f.get("mediaInfo") or {}).get("videoDynamicRangeType"))
    out = {"/media" + os.path.join(m["path"], m["movieFile"]["relativePath"])
           for m in movies if m.get("movieFile") and hdr(m["movieFile"])}
    return out | {"/media" + f["path"] for f in episode_files if hdr(f)}


def with_stripped_hdr(rows, arr_hdr):
    """Send files the arrs recorded as HDR to refetch, whatever ffprobe says now.

    The old flow encoded HDR sources without carrying their PQ tags, so the
    result reads as plain 10-bit and plays washed out on an SDR screen. Only the
    arrs' media info, read before the encode, still knows.
    """
    for f in rows:
        if f["file"] in arr_hdr and "hdr" not in f["reasons"]:
            f = {**f, "bucket": "refetch", "reasons": f["reasons"] + ["hdr-stripped"]}
        yield f


# What a grab under the 1080p, 12 Mbps Radarr profile can't bring back, so a
# movie the ledger already refetched is planned again for these.
REGRAB = {"over-1080p", "hdr", "over-bitrate"}


def held_back(movie_id, reasons, held):
    """A movie refetched before is left alone unless its file is now over the size or SDR cap."""
    return movie_id in held and not REGRAB & set(reasons)


def listed_movies(movies, ids, plex, count, ledger):
    """Movies picked by hand (Radarr ids), planned whatever conformance says.

    tdarr's old 8-bit squeeze left films like Arrival conformant on paper and
    banded on screen; only a person watching can tell, so they come as a list."""
    busy = {i["movie_id"] for i in ledger.values() if "movie_id" in i and i.get("state") != "done"}
    todo = []
    for m in movies:
        if m["id"] not in ids or m["id"] in busy:
            continue
        path = os.path.join(m["path"].rstrip("/"), m["movieFile"]["relativePath"])
        if path not in plex:
            continue
        rk, added, _ = plex[path]
        todo.append({"file": "/media" + path, "path": path, "reasons": ["listed"],
                     "size_gb": round(m["movieFile"]["size"] / 1e9, 2), "movie_id": m["id"],
                     "title": f"{m['title']} ({m['year']})", "rating_key": rk, "added_at": added,
                     "plays": count.get(rk, 0)})
    return todo


def cmd_plan(a):
    ledger = load()
    movies = [m for m in mc.get("radarr", "/api/v3/movie") if m.get("hasFile")]
    if a.tv_files:
        # Episode paths as tdarr sees them (/media/tv/...), one per line.
        with open(a.tv_files) as f:
            listed = [{"file": x.split("#")[0].strip(), "library": "tv", "reasons": ["listed"], "size_gb": 0.0}
                      for x in f if x.split("#")[0].strip()]
        a.tv_gb = a.tv_gb or float("inf")
        plan_tv(a, listed, ledger, plex_items(), plays(), relist=True)
        save(ledger)
        return
    if a.ids:
        with open(a.ids) as f:
            ids = {int(x.split("#")[0]) for x in f if x.split("#")[0].strip()}
        todo = sorted(listed_movies(movies, ids, plex_items(), plays(), ledger), key=lambda t: (-t["plays"], t["title"]))
        batch, gb = [], 0.0
        for t in todo:
            if len(batch) >= a.movies or gb + t["size_gb"] > a.max_gb:
                break
            batch.append(t)
            gb += t["size_gb"]
        for t in batch:
            print(f"plan   {t['title']}: listed, {t['plays']} plays")
            ledger[t["file"]] = {**t, "state": "planned", "attempts": 0, "log": []}
        print(f"{len(batch)} planned from {a.ids} ({gb:.0f} GB on disk now), {len(todo) - len(batch)} more waiting")
        save(ledger)
        return
    radarr = {os.path.join(m["path"].rstrip("/"), m["movieFile"]["relativePath"]): m for m in movies}
    folders = {m["path"].rstrip("/") for m in movies}
    plex = plex_items()
    count = plays()
    # One call per series; serially that ran past 8 minutes on agentbox.
    with concurrent.futures.ThreadPoolExecutor(8) as pool:
        episodes = [f for fs in pool.map(lambda s: mc.get("sonarr", "/api/v3/episodefile", seriesId=s["id"]),
                                         mc.get("sonarr", "/api/v3/series") if a.tv_gb else [])
                    for f in fs]
    hdr = arr_hdr_paths(movies, episodes)
    refetch = [f for f in with_stripped_hdr(conformance.sweep(), hdr) if f["bucket"] == "refetch"]
    # NOTE: by id, not path: a replacement lands at a new path (the name carries
    # the quality), so a movie kept without subtitles would be planned again.
    held = {i["movie_id"] for i in ledger.values() if "movie_id" in i}
    todo, skipped = [], {}
    for f in refetch:
        if f["library"] != "movies" or f["file"] in ledger:
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
        if held_back(movie["id"], f["reasons"], held):
            continue
        rk, added, _ = plex[path]
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
    print(f"{len(batch)} planned ({gb:.0f} GB on disk now), {len(todo) - len(batch)} more waiting")
    if a.tv_gb:
        plan_tv(a, refetch, ledger, plex, count)
    save(ledger)


def note(item, msg):
    item["log"].append(f"{now().isoformat(timespec='seconds')} {msg}")
    print(f"{item['state']:8} {item['title']}: {msg}")


def gave_up(grabbed, downloading):
    """A download still in the client's queue is waited on however long it takes."""
    return not downloading and now() - grabbed > GRAB_TIMEOUT


def awaiting_redownload(item):
    return "failed_at" in item and now() - dt.datetime.fromisoformat(item["failed_at"]) < REDOWNLOAD_GRACE


def import_blocked(downloading):
    """Finished, but the *arr will not import it, so it stays queued and reads as downloading forever."""
    return bool(downloading) and all(q.get("status") == "completed" and q.get("trackedDownloadStatus") == "warning"
                                     for q in downloading)


def blocklist(app, downloading):
    for q in {q["downloadId"]: q for q in reversed(downloading)}.values():
        mc.delete(app, f"/api/v3/queue/{q['id']}", removeFromClient="true", blocklist="true",
                  skipRedownload="true")


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
        if awaiting_redownload(item):
            return False
        rel = best_release(mid)
        if not rel:
            note(item, "no acceptable release found; left as is")
            item["state"] = "no-release"
            return True
        note(item, f"grab {rel['title']} ({rel['size'] / 1e9:.1f} GB, {rel['indexer']})")
        if not yes:
            return False
        movie = mc.get("radarr", f"/api/v3/movie/{mid}")
        # Grab first: a refused grab must leave the old file, and the download
        # takes minutes, long before Radarr would import over it.
        mc.post("radarr", "/api/v3/release", {"guid": rel["guid"], "indexerId": rel["indexerId"]})
        if movie.get("hasFile"):
            mc.delete("radarr", f"/api/v3/moviefile/{movie['movieFile']['id']}")
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
            downloading = mc.get("radarr", "/api/v3/queue/details", movieId=mid)
            if failed and not downloading:
                note(item, f"download failed ({failed[-1].get('sourceTitle', '')[:60]}); searching again")
                if yes:
                    item["state"] = "planned"
                return yes
            if import_blocked(downloading):
                note(item, f"Radarr cannot import {downloading[0]['title'][:60]}; blocklisting and searching again")
                if not yes:
                    return False
                blocklist("radarr", downloading)
                item["state"] = "planned"
                return True
            if gave_up(grabbed, downloading):
                note(item, f"nothing imported after {GRAB_TIMEOUT}; check the Radarr queue")
                item["state"] = "stuck"
                return True
            return False
        path = movie["path"] + "/" + mf["relativePath"]
        langs = subs("/media" + path)
        item["attempts"] += 1
        if any(lang in conformance.ENGLISH for lang in langs):
            note(item, f"imported {mf['relativePath']} with subtitles {sorted(set(langs))}")
            item.update(state="imported", imported_at=now().isoformat(), file="/media" + path, path=path)
            return True
        if item["attempts"] >= MAX_ATTEMPTS:
            note(item, f"attempt {item['attempts']} still has no English subtitle; keeping it")
            item.update(state="imported", imported_at=now().isoformat(), file="/media" + path, path=path,
                        no_subs=True)
            return True
        note(item, f"attempt {item['attempts']} has no English subtitle; blocklisting and trying the next")
        if not yes:
            return False
        hist = mc.get("radarr", "/api/v3/history/movie", movieId=mid, eventType=1)  # 1 = grabbed
        latest = max(hist, key=lambda h: h["date"])
        mc.delete("radarr", f"/api/v3/moviefile/{mf['id']}")
        mc.post("radarr", f"/api/v3/history/failed/{latest['id']}", {})
        item.update(state="planned", failed_at=now().isoformat())
        return True

    if item["state"] == "imported":
        item["file"] = flowed(item["file"])
        item["path"] = item["file"].removeprefix("/media")
        why = conformance.reasons(probe(item["file"]))
        if "no-dialogue-track" in why or "video-not-hevc" in why:
            # NOTE: an item waiting here holds off the next batch. Chamber of
            # Secrets came out of the flow HEVC but with no dialogue track and
            # stalled every batch from 2026-10-14 until this timeout existed.
            imported = dt.datetime.fromisoformat(item.get("imported_at", item["grabbed_at"]))
            # A CPU worker takes a day or more over a remux; one still running is not stalled.
            if now() - imported > FLOW_TIMEOUT and item["file"] not in transcoding_now():
                note(item, f"still {','.join(why)} after {FLOW_TIMEOUT}; check the tdarr flow")
                item["state"] = "stuck"
                return True
            return bump(item, [item["file"]], yes)       # tdarr has not run the flow yet
        added = dt.datetime.fromtimestamp(item["added_at"]).date()
        note(item, f"conforms ({','.join(why) or 'clean'}); restore Plex added date {added}")
        if not yes:
            return False
        # NOTE: the delete-and-import gives the movie a new Plex item (which is
        # why its added date reset), so the rating key recorded at plan time is
        # gone. Find the item that holds the new file now.
        current = plex_items().get(item["path"])
        if not current:
            return False                               # Plex has not scanned the new file yet
        restore_added(current[0], item["added_at"], PLEX_MOVIE)
        item["state"] = "done"
        return True
    return False


def step_tv(item, yes, ledger):
    """step() for a season pack or a single episode file, through Sonarr."""
    sid, sn, eps = item["series_id"], item["season"], item["episode_ids"]

    def queue():
        return [q for q in mc.get("sonarr", "/api/v3/queue/details", seriesId=sid) if q.get("episodeId") in eps]

    if item["state"] == "planned":
        queued = queue()
        if queued:
            note(item, f"already downloading {queued[0]['title']}; following it")
            if yes:
                item.update(state="grabbed", grabbed_at=now().isoformat(), release=queued[0]["title"])
            return yes
        if awaiting_redownload(item):
            return False
        rel = best_tv_release(item)
        if not rel and item["pack"]:
            note(item, f"no acceptable season pack; searching its {len(item['bad'])} bad episodes one at a time")
            if not yes:
                return False
            for b in item["bad"]:
                ledger.setdefault(b["file"], {**b, "kind": "tv", "pack": False, "series_id": sid, "season": sn,
                                              "plays": item["plays"], "state": "planned", "attempts": 0, "log": []})
            item["state"] = "split"
            return True
        if not rel:
            note(item, "no acceptable release found; left as is")
            item["state"] = "no-release"
            return True
        note(item, f"grab {rel['title']} ({rel['size'] / 1e9:.1f} GB, {rel['indexer']})")
        if not yes:
            return False
        # NOTE: a pack covers every episode in the season, and Sonarr will not
        # import over a file that already meets the cutoff, so all of them go,
        # but only once the grab is accepted.
        mc.post("sonarr", "/api/v3/release", {"guid": rel["guid"], "indexerId": rel["indexerId"]})
        for fid in {f["id"] for f in tv_files(sid, eps).values()}:
            mc.delete("sonarr", f"/api/v3/episodefile/{fid}")
        item.update(state="grabbed", grabbed_at=now().isoformat(), release=rel["title"])
        return True

    if item["state"] == "grabbed":
        grabbed = when(item["grabbed_at"])
        new = {e: f for e, f in tv_files(sid, eps).items() if when(f["dateAdded"]) > grabbed}
        if len(new) < len(eps):
            downloading = queue()
            failed = [h for h in mc.get("sonarr", "/api/v3/history/series", seriesId=sid, seasonNumber=sn,
                                        eventType=4)                    # 4 = downloadFailed
                      if h.get("episodeId") in eps and when(h["date"]) > grabbed]
            if failed and not new and not downloading:
                note(item, f"download failed ({failed[-1].get('sourceTitle', '')[:60]}); searching again")
                if yes:
                    item["state"] = "planned"
                return yes
            # Finished but Sonarr will not import it (a misparsed "pack" holding one
            # episode): it stays in the queue, so it would read as downloading forever.
            if import_blocked(downloading) and not new:
                note(item, f"Sonarr cannot import {downloading[0]['title'][:60]}; blocklisting and searching again")
                if not yes:
                    return False
                blocklist("sonarr", downloading)
                item["state"] = "planned"
                return True
            # Sonarr imported what it could match and holds the rest of the release as
            # blocked; clear it so the episodes it lacked get searched.
            if import_blocked(downloading):
                note(item, f"Sonarr imported {len(new)} of {len(eps)} from {downloading[0]['title'][:60]}; "
                           "clearing the rest from the queue")
                if not yes:
                    return False
                for q in {q["downloadId"]: q for q in reversed(downloading)}.values():
                    mc.delete("sonarr", f"/api/v3/queue/{q['id']}", removeFromClient="true", blocklist="false")
                downloading = []
            if not new or downloading:
                if gave_up(grabbed, downloading):
                    note(item, f"{len(new)} of {len(eps)} imported after {GRAB_TIMEOUT}; check the Sonarr queue")
                    item["state"] = "stuck"
                    return True
                return False
            # The download is through and some episodes were not in it.
            missing = [e for e in eps if e not in new]
            note(item, f"the release had {len(new)} of {len(eps)} episodes; Sonarr searches the other {len(missing)}")
            if not yes:
                return False
            mc.post("sonarr", "/api/v3/command", {"name": "EpisodeSearch", "episodeIds": missing})
            item["episode_ids"] = eps = sorted(new)
        paths = sorted({f["path"] for f in new.values()})
        bare = [p for p in paths if not any(lang in conformance.ENGLISH for lang in subs("/media" + p))]
        item["attempts"] += 1
        files = ["/media" + p for p in paths]
        if not bare:
            note(item, f"imported {len(paths)} files with English subtitles")
            item.update(state="imported", imported_at=now().isoformat(), files=files)
            return True
        if item["attempts"] >= MAX_ATTEMPTS:
            note(item, f"attempt {item['attempts']} still has {len(bare)} files with no English subtitle; keeping it")
            item.update(state="imported", imported_at=now().isoformat(), files=files, no_subs=len(bare))
            return True
        note(item, f"attempt {item['attempts']} has {len(bare)} files with no English subtitle; "
                   "blocklisting and trying the next")
        if not yes:
            return False
        hist = [h for h in mc.get("sonarr", "/api/v3/history/series", seriesId=sid, seasonNumber=sn,
                                  eventType=1)                          # 1 = grabbed
                if h.get("episodeId") in eps]
        latest = max(hist, key=lambda h: h["date"])
        for fid in {f["id"] for f in new.values()}:
            mc.delete("sonarr", f"/api/v3/episodefile/{fid}")
        mc.post("sonarr", f"/api/v3/history/failed/{latest['id']}", {})
        item.update(state="planned", failed_at=now().isoformat())
        return True

    if item["state"] == "imported":
        files = [flowed(f) for f in item["files"]]
        if files != item["files"]:
            note(item, f"{sum(a != b for a, b in zip(files, item['files']))} files remuxed to .mkv; Sonarr rescans")
            item["files"] = files
            mc.post("sonarr", "/api/v3/command", {"name": "RescanSeries", "seriesId": sid})
        whys = [conformance.reasons(probe(f)) for f in item["files"]]
        waiting = [w for w in whys if "no-dialogue-track" in w or "video-not-hevc" in w]
        if waiting:
            if now() - when(item["imported_at"]) > FLOW_TIMEOUT and not transcoding_now() & set(item["files"]):
                note(item, f"{len(waiting)} files still {','.join(waiting[0])} after {FLOW_TIMEOUT}; "
                           "check the tdarr flow")
                item["state"] = "stuck"
                return True
            return bump(item, item["files"], yes)      # tdarr has not run the flow over all of them yet
        note(item, f"{len(whys)} files through the flow; restore their Plex added dates")
        if not yes:
            return False
        plex = plex_items()
        current = {e: plex.get(f["path"]) for e, f in tv_files(sid, eps).items()}
        if not all(current.values()):
            return False                               # Plex has not scanned every new file yet
        for e, (rk, _, _) in current.items():
            if str(e) in item["added"]:
                restore_added(rk, item["added"][str(e)], PLEX_EPISODE)
        item["state"] = "done"
        return True
    return False


def cmd_run(a):
    ledger = load()
    for item in list(ledger.values()):                 # a season with no pack adds its episodes
        try:
            while (step_tv(item, a.yes, ledger) if item.get("kind") == "tv" else step(item, a.yes)):
                save(ledger)
                if not a.yes:
                    break
        except Exception as e:  # one bad item must not stall the rest of the batch
            print(f"ERROR    {item['title']}: {e}")
    if not a.yes:
        print("dry run: re-run with --yes to act")


def free_tb():
    return int(ssh("df -B1 --output=avail /data01 | tail -1").strip()) / 1e12


def cmd_auto(a):
    """What the agentbox timer runs: start the next batch once the last is through."""
    active = [i for i in load().values() if i["state"] in ("planned", "grabbed", "imported")]
    start, end = (int(h) for h in a.window.split("-"))
    hour = dt.datetime.now().hour
    if not active and not start <= hour < end:
        # NOTE: a batch deletes each old file when it grabs the new one. Started
        # at 20:30 on 2026-10-13, after NZBGet's 18:00 pause, 25 popular movies
        # sat missing from Plex until downloads resumed at 02:00. NZBGet has
        # run 24/7 since #1005, so narrow the window only if it pauses again.
        print(f"not planning: {hour:02d}:00 is outside the {a.window}h batch window")
    elif not active:
        free = free_tb()
        if free < a.min_free_tb:
            print(f"not planning: /data01 has {free:.1f} TB free, floor is {a.min_free_tb} TB")
        else:
            cmd_plan(a)
    cmd_run(a)


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
    p.add_argument("--tv-gb", type=float, default=0, help="TV to plan, in GB on disk; 0 plans none")
    p.add_argument("--show-skipped", action="store_true", help="list every skipped path")
    p.add_argument("--tv-files", help="plan only these episode files (/media/tv/... paths, one per line), even ones refetched before")
    p.add_argument("--ids", help="plan only these Radarr movie ids, one per line (# comments), whatever conformance says")
    p.set_defaults(fn=cmd_plan)
    r = sub.add_parser("run")
    r.add_argument("--yes", action="store_true")
    r.set_defaults(fn=cmd_run)
    au = sub.add_parser("auto")
    au.add_argument("--movies", type=int, default=25)
    au.add_argument("--max-gb", type=float, default=2000)
    au.add_argument("--tv-gb", type=float, default=0, help="TV to plan, in GB on disk; 0 plans none")
    au.add_argument("--window", default="0-24",
                    help="local hours START-END in which a new batch may start; NZBGet downloads 24/7")
    au.add_argument("--min-free-tb", type=float, default=3.0,
                    help="do not start a batch with less than this free on /data01")
    au.add_argument("--yes", action="store_true")
    au.set_defaults(fn=cmd_auto, show_skipped=False)
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
