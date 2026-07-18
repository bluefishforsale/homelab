#!/usr/bin/env python3
"""Delete movies/shows via Radarr/Sonarr with unmonitor + import-exclusion, so
the *arr app drops the files AND never re-grabs them. Refuses anything with an
Overseerr request (someone asked for it), unless --ignore-overseerr.

DRY-RUN BY DEFAULT. Nothing is deleted without --yes.

  # preview the low-value-TV cut from the analysis:
  media-reclaim-delete.py --service tv --unmonitored --status ended --min-gb 150
  # execute a specific set:
  media-reclaim-delete.py --service tv --ids 1068,1089,1096 --yes

Filters (AND together; omit --ids to select by flags):
  --service movies|tv   (required)
  --ids a,b,c           explicit *arr ids, skips other filters
  --unmonitored         only monitored==false
  --status ended        sonarr status match (ended/continuing/...)
  --min-gb N            only titles >= N GB
"""
import argparse
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import media_clients as mc  # noqa: E402

# exclusion param name differs: radarr=addImportExclusion, sonarr=addImportListExclusion
APP = {"movies": ("radarr", "movie", "tmdbId", "addImportExclusion"),
       "tv": ("sonarr", "series", "tvdbId", "addImportListExclusion")}


def size_of(it):
    s = it.get("sizeOnDisk")
    if s is None:
        s = (it.get("statistics") or {}).get("sizeOnDisk", 0)
    return int(s or 0)


def requested_ids(service, block_any):
    """Set of tmdbId (movies) / tvdbId (tv) with a protecting Overseerr request.

    Default protects only *in-flight* wants: request pending approval
    (status==1) or media not yet satisfied (media.status 2/3/4 =
    pending/processing/partial). A fulfilled+available request (status 5) is a
    finished want and does not veto a prune. --block-any-request restores the
    blanket "requested ever" veto.
    """
    want = "movie" if service == "movies" else "tv"
    idfield = "tmdbId" if service == "movies" else "tvdbId"
    out = set()
    skip, take = 0, 100
    while True:  # paginate: never trust one page to hold every request
        page = mc.get("overseerr", "/api/v1/request", take=take, skip=skip, filter="all")
        results = page.get("results", [])
        for r in results:
            m = r.get("media") or {}
            if m.get("mediaType") != want or not m.get(idfield):
                continue
            inflight = r.get("status") == 1 or m.get("status") in (2, 3, 4)
            if block_any or inflight:
                out.add(m[idfield])
        total = (page.get("pageInfo") or {}).get("results", 0)
        skip += take
        if not results or skip >= total:
            return out


def select(items, a):
    if a.ids:
        want = set(a.ids)
        found = [i for i in items if i["id"] in want]
        missing = want - {i["id"] for i in found}
        if missing:
            print(f"warning: ids not in library, skipped: {sorted(missing)}", file=sys.stderr)
        return found
    out = items
    if a.unmonitored:
        out = [i for i in out if not i.get("monitored")]
    if a.status:
        out = [i for i in out if i.get("status") == a.status]
    if a.min_gb:
        out = [i for i in out if size_of(i) >= a.min_gb * 1e9]
    return out


def main():
    p = argparse.ArgumentParser(usage=__doc__)
    p.add_argument("--service", required=True, choices=APP)
    p.add_argument("--ids", type=lambda s: [int(x) for x in s.split(",")])
    p.add_argument("--unmonitored", action="store_true")
    p.add_argument("--status")
    p.add_argument("--min-gb", type=float)
    p.add_argument("--ignore-overseerr", action="store_true")
    p.add_argument("--block-any-request", action="store_true",
                   help="veto on any request ever, not just in-flight ones")
    p.add_argument("--yes", action="store_true")
    a = p.parse_args()

    svc, kind, idfield, excl = APP[a.service]
    items = mc.get(svc, f"/api/v3/{kind}")
    targets = sorted(select(items, a), key=size_of, reverse=True)
    if not targets:
        sys.exit("no titles matched")

    reqset = set() if a.ignore_overseerr else requested_ids(a.service, a.block_any_request)
    print("ACT\tGB\tCUM_TB\tMON\tID\tTITLE")
    cum = 0
    todo = []
    for it in targets:
        gb = size_of(it) / 1e9
        blocked = it.get(idfield) in reqset
        if not blocked:
            cum += size_of(it)
            todo.append(it)
        act = "SKIP-req" if blocked else "DELETE"
        print(f"{act}\t{gb:.0f}\t{cum/1e12:.2f}\t{it.get('monitored')}\t{it['id']}"
              f"\t{it.get('title')} ({it.get('year','')})")
    print(f"\n{len(todo)} titles, {cum/1e12:.2f} TB to free "
          f"({len(targets)-len(todo)} skipped as Overseerr-requested)")

    if not a.yes:
        print("DRY RUN: re-run with --yes to delete files + add import exclusion")
        return
    ok = err = 0
    for it in todo:
        try:
            mc.delete(svc, f"/api/v3/{kind}/{it['id']}", deleteFiles="true", **{excl: "true"})
            ok += 1
            print(f"deleted {it['id']} {it.get('title')}")
        except Exception as e:
            err += 1
            print(f"FAILED {it['id']} {it.get('title')}: {e}", file=sys.stderr)
    print(f"done: {ok} deleted, {err} failed, ~{cum/1e12:.2f} TB freed")


def selftest():
    class A:
        ids = None; unmonitored = True; status = "ended"; min_gb = 150
    items = [
        {"id": 1, "monitored": False, "status": "ended", "statistics": {"sizeOnDisk": 200e9}},
        {"id": 2, "monitored": True, "status": "ended", "statistics": {"sizeOnDisk": 300e9}},
        {"id": 3, "monitored": False, "status": "continuing", "statistics": {"sizeOnDisk": 300e9}},
        {"id": 4, "monitored": False, "status": "ended", "statistics": {"sizeOnDisk": 100e9}},
    ]
    got = {i["id"] for i in select(items, A)}
    assert got == {1}, got  # unmonitored+ended+>=150GB -> only id 1
    print("ok")


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--selftest":
        selftest()
    else:
        main()
