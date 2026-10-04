#!/usr/bin/env python3
"""Emit taste-aware cull candidates for the ocean movie/TV library as a TSV you
edit by hand: delete the rows you want to KEEP, save, then feed column 1 (ID)
to media-reclaim-delete.py --ids.

Signals combined from Radarr/Sonarr (size, monitored, genres, ratings),
Tautulli (whether it's ever been played), and Overseerr (in-flight requests).
The default policy — tuned on this library — is:

  keep everything that is monitored, watched, has an in-flight Overseerr
  request, is a protected genre (default: Horror), or is older than --since;
  and of what's left, keep the well-rated and cull the mediocre.

"Well-rated" differs by service because the scales differ:
  movies: keep if IMDb >= 6 OR RottenTomatoes >= 70 (cull the rest)
  tv:     keep if rating >= 7.5  (Sonarr's single 0-10 score; TV median ~8)

  media-cull-candidates.py movies > movie-cull.tsv
  media-cull-candidates.py tv --since 2011 --keep-genre Horror,Documentary

--by-collection (movies only) swaps the year/rating policy for a franchise one:
judge each entry against the best film in its OWN collection, so the weak tail
of a series is culled while a strong franchise keeps all of it. --since does not
apply, because franchises span decades and the bad sequel is often the old one.

  media-cull-candidates.py movies --by-collection > franchise-cull.tsv

Nothing is deleted here; this only prints candidates. media_clients reaches the
other services from the laptop via the vault, but Overseerr's self-generated key
is not in the vault, so this script needs ocean or OVERSEERR_APIKEY.
"""
import argparse
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import media_clients as mc  # noqa: E402

APP = {
    "movies": {"svc": "radarr", "endpoint": "/api/v3/movie", "idfield": "tmdbId", "section": 1},
    "tv": {"svc": "sonarr", "endpoint": "/api/v3/series", "idfield": "tvdbId", "section": 2},
}


def played_titles(section):
    """Titles with >=1 play, from Tautulli. Movies key on (title, year); shows on title.

    Tautulli reports per-item last_played for movies, and per-show last_played
    for series (episode file_size is empty for shows, so size never comes from
    here — Radarr/Sonarr own size)."""
    data = mc.get("tautulli", "/api/v2", cmd="get_library_media_info",
                  section_id=section, length=100000)["response"]["data"]["data"]
    out = set()
    for i in data:
        if i.get("last_played") or int(i.get("play_count") or 0) > 0:
            out.add((i["title"], str(i.get("year") or "")) if section == 1 else i["title"])
    return out


def inflight_ids(media_type, idfield):
    ids, skip = set(), 0
    while True:
        page = mc.get("overseerr", "/api/v1/request", take=100, skip=skip, filter="all")
        for r in page.get("results", []):
            m = r.get("media") or {}
            if m.get("mediaType") == media_type and m.get(idfield) \
                    and (r.get("status") == 1 or m.get("status") in (2, 3, 4)):
                ids.add(m[idfield])
        skip += 100
        if skip >= (page.get("pageInfo") or {}).get("results", 0):
            return ids


def size_gb(it):
    s = it.get("sizeOnDisk")
    if s is None:
        s = (it.get("statistics") or {}).get("sizeOnDisk", 0)
    return int(s or 0) / 1e9


def rating_ok(it, kind, tv_bar):
    r = it.get("ratings") or {}
    if kind == "movies":
        imdb = r.get("imdb", {}).get("value")
        rt = r.get("rottenTomatoes", {}).get("value")
        return (imdb is not None and imdb >= 6) or (rt is not None and rt >= 70)
    return (r.get("value") or 0) >= tv_bar


def best_in_collection(items, collections):
    """tmdbId -> (collection title, best IMDb among OWNED entries, [owned titles]).

    Radarr leaves `collection` null on /movie, so membership has to come from
    /collection, whose `movies` list includes entries that were never in the
    library. Only owned-with-a-file entries define the franchise's bar; a
    collection the library holds one film from has no tail to cut.
    """
    owned = {it["tmdbId"]: it for it in items if it.get("hasFile")}
    out = {}
    for c in collections:
        mem = [owned[m["tmdbId"]] for m in c.get("movies") or [] if m["tmdbId"] in owned]
        if len(mem) < 2:
            continue
        rated = [(it.get("ratings") or {}).get("imdb", {}).get("value") for it in mem]
        rated = [v for v in rated if v is not None]
        if not rated:
            continue
        for it in mem:
            out[it["tmdbId"]] = (c["title"], max(rated), mem)
    return out


def rating_cols(it, kind):
    r = it.get("ratings") or {}
    if kind == "movies":
        return f"{r.get('imdb', {}).get('value')}\t{r.get('rottenTomatoes', {}).get('value')}"
    return f"{r.get('value')}"


def _weak_in_franchise(it, best, a):
    """Clearly worse than its own franchise's best, and not rescued by RT.

    The RT arm matters: a 6.3 IMDb with RT 76 (Scream 2022, Nobody 2) is a well
    reviewed film sitting next to an outlier, not a bad sequel. Same OR-logic as
    rating_ok, kept separate because the bar here is relative, not absolute.
    """
    r = it.get("ratings") or {}
    imdb = r.get("imdb", {}).get("value")
    rt = r.get("rottenTomatoes", {}).get("value")
    if imdb is None:
        return False
    if rt is not None and rt >= 70:
        return False
    return (best - imdb) >= a.gap and imdb < a.imdb_bar


def main():
    p = argparse.ArgumentParser(usage=__doc__)
    p.add_argument("kind", choices=APP)
    p.add_argument("--since", type=int, default=2011, help="cull only titles from this year onward")
    p.add_argument("--keep-genre", default="Horror", help="comma list of genres to always keep")
    p.add_argument("--tv-bar", type=float, default=7.5, help="tv: keep rating >= this")
    p.add_argument("--by-collection", action="store_true",
                   help="movies: judge each entry against the best in its own franchise")
    p.add_argument("--gap", type=float, default=1.0,
                   help="--by-collection: cull at least this far below the franchise best")
    p.add_argument("--imdb-bar", type=float, default=6.5,
                   help="--by-collection: and below this absolute IMDb score")
    a = p.parse_args()

    if a.by_collection and a.kind != "movies":
        sys.exit("--by-collection needs Radarr collections; sonarr has no equivalent")

    app = APP[a.kind]
    protect = {g.strip() for g in a.keep_genre.split(",") if g.strip()}
    played = played_titles(app["section"])
    # Overseerr generates its own API key into settings.json and it is in
    # neither the vault nor this repo, so unlike the other services it has no
    # off-host fallback. Refuse rather than proceed: in-flight requests are the
    # guard that stops this list recommending something somebody just asked for.
    try:
        inflight = inflight_ids("movie" if a.kind == "movies" else "tv", app["idfield"])
    except Exception as e:
        sys.exit(f"overseerr unreachable ({e}); its in-flight requests are a required "
                 f"safety check, so run this on ocean or set OVERSEERR_APIKEY")
    items = mc.get(app["svc"], app["endpoint"])

    def watched(it):
        k = (it.get("title"), str(it.get("year") or "")) if a.kind == "movies" else it.get("title")
        return k in played

    if a.by_collection:
        franchise = best_in_collection(items, mc.get("radarr", "/api/v3/collection"))
        cull = [it for it in items
                if it["tmdbId"] in franchise
                and not (protect & set(it.get("genres") or []))
                and not watched(it)
                and it.get(app["idfield"]) not in inflight
                and size_gb(it) > 0
                and _weak_in_franchise(it, franchise[it["tmdbId"]][1], a)]
        cull.sort(key=lambda it: (franchise[it["tmdbId"]][0], it.get("year") or 0))
        print(f"# movie franchise cull candidates: unwatched, not-requested, >= {a.gap} IMDb below "
              f"the best in their own collection, below IMDb {a.imdb_bar}, not rescued by RT>=70, "
              f"keep-genre={','.join(sorted(protect))}")
        print(f"# {len(cull)} titles / {sum(size_gb(it) for it in cull)/1000:.2f} TB. DELETE rows "
              f"to KEEP, save, feed col 1 to media-reclaim-delete.py --service movies --ids",
              file=sys.stderr)
        print("ID\tGB\tIMDB\tRT\tYEAR\tTITLE\tFRANCHISE (what stays)")
        for it in cull:
            title, _, mem = franchise[it["tmdbId"]]
            culled = {c["tmdbId"] for c in cull}
            stays = ", ".join(f"{m['title']}({(m.get('ratings') or {}).get('imdb', {}).get('value')})"
                              for m in sorted(mem, key=lambda m: m.get("year") or 0)
                              if m["tmdbId"] not in culled)
            print(f"{it['id']}\t{size_gb(it):.1f}\t{rating_cols(it, 'movies')}\t{it.get('year')}"
                  f"\t{it.get('title')}\t{title} -> {stays}")
        return

    cull = [it for it in items
            if (it.get("year") or 0) >= a.since
            and not (protect & set(it.get("genres") or []))
            and not watched(it)
            and not it.get("monitored")
            and it.get(app["idfield"]) not in inflight
            and size_gb(it) > 0
            and not rating_ok(it, a.kind, a.tv_bar)]
    cull.sort(key=size_gb, reverse=True)

    rate_hdr = "IMDB\tRT" if a.kind == "movies" else "RATING"
    total = sum(size_gb(it) for it in cull)
    print(f"# {a.kind} cull candidates: {a.since}+, unwatched, unmonitored, not-requested, "
          f"keep-genre={','.join(sorted(protect))}, below rating bar")
    print(f"# {len(cull)} titles / {total/1000:.2f} TB. DELETE rows to KEEP, save, feed col 1 to "
          f"media-reclaim-delete.py --service {a.kind} --ids", file=sys.stderr)
    print(f"ID\tGB\t{rate_hdr}\tYEAR\tGENRES\tTITLE")
    for it in cull:
        print(f"{it['id']}\t{size_gb(it):.1f}\t{rating_cols(it, a.kind)}\t{it.get('year')}"
              f"\t{','.join((it.get('genres') or [])[:2])}\t{it.get('title')}")


if __name__ == "__main__":
    main()
