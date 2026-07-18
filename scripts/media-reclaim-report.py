#!/usr/bin/env python3
"""Profile the /data01/complete library by taste: genre, decade, language,
rating, studio/network, quality, monitored-vs-not. Source of truth is Radarr
(/movie) and Sonarr (/series), which carry genres/size/monitored/path/id, so
the profile lines up with what the *arr apps will later unmonitor + delete.

This reports what you HAVE and what you're still chasing (monitored). It does
not rank by watch history. Deletion + *arr unmonitor is a separate script.

Endpoints + key discovery come from media_clients (RADARR_URL/RADARR_APIKEY etc.
env override, else the live config.xml on ocean).

Usage: media-reclaim-report.py [movies|tv|all]   (default all)
Sizes double-count across multi-valued dims (a title has several genres); the
per-genre size answers "how much disk is my western habit", not a disk sum.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import media_clients as mc  # noqa: E402

TB = 1e12
SVC = {"movies": ("radarr", "/api/v3/movie"), "tv": ("sonarr", "/api/v3/series")}


def size_of(item):
    s = item.get("sizeOnDisk")
    if s is None:
        s = (item.get("statistics") or {}).get("sizeOnDisk", 0)
    return int(s or 0)


def decade(item):
    y = item.get("year") or 0
    return f"{(y // 10) * 10}s" if y else "unknown"


def groupby(items, keyfn):
    """key -> {count, size, mon, mon_size}; keyfn may return a list (multi-valued)."""
    out = {}
    for it in items:
        keys = keyfn(it)
        if not isinstance(keys, list):
            keys = [keys]
        sz = size_of(it)
        mon = bool(it.get("monitored"))
        for k in keys or ["(none)"]:
            b = out.setdefault(k, [0, 0, 0, 0])
            b[0] += 1
            b[1] += sz
            if mon:
                b[2] += 1
                b[3] += sz
    return out


def table(title, groups, limit=None):
    rows = sorted(groups.items(), key=lambda kv: -kv[1][1])
    if limit:
        rows = rows[:limit]
    print(f"\n## {title}")
    print("KEY\tCOUNT\tSIZE_TB\tMON\tMON_TB")
    for k, (n, sz, mon, msz) in rows:
        print(f"{k}\t{n}\t{sz/TB:.2f}\t{mon}\t{msz/TB:.2f}")


def profile(name):
    svc, endpoint = SVC[name]
    items = mc.get(svc, endpoint)
    total = sum(size_of(i) for i in items)
    mon = [i for i in items if i.get("monitored")]
    mon_sz = sum(size_of(i) for i in mon)
    print(f"\n{'='*60}\n{name.upper()}: {len(items)} titles, {total/TB:.2f} TB total")
    print(f"  monitored: {len(mon)} titles / {mon_sz/TB:.2f} TB   "
          f"unmonitored: {len(items)-len(mon)} / {(total-mon_sz)/TB:.2f} TB")

    table("Genre", groupby(items, lambda i: i.get("genres") or []))
    table("Decade", groupby(items, decade))
    table("Language", groupby(items, lambda i: (i.get("originalLanguage") or {}).get("name", "?")))
    if name == "movies":
        table("Certification", groupby(items, lambda i: i.get("certification") or "(none)"))
        table("Studio (top 20)", groupby(items, lambda i: i.get("studio") or "(none)"), limit=20)
        table("Collection (top 20)", groupby(items,
              lambda i: (i.get("collection") or {}).get("title", "(none)")), limit=20)
    else:
        table("Network (top 20)", groupby(items, lambda i: i.get("network") or "(none)"), limit=20)
        table("Status", groupby(items, lambda i: i.get("status") or "?"))
        table("Series type", groupby(items, lambda i: i.get("seriesType") or "?"))


def selftest():
    items = [
        {"genres": ["Western", "Drama"], "year": 1971, "monitored": True, "sizeOnDisk": 100},
        {"genres": ["Western"], "year": 1968, "monitored": False, "sizeOnDisk": 200},
        {"genres": [], "year": 0, "monitored": False, "statistics": {"sizeOnDisk": 50}},
    ]
    g = groupby(items, lambda i: i.get("genres") or [])
    assert g["Western"] == [2, 300, 1, 100], g["Western"]
    assert g["Drama"] == [1, 100, 1, 100], g["Drama"]
    d = groupby(items, decade)
    assert d["1970s"][0] == 1 and d["unknown"][0] == 1, d
    assert size_of(items[2]) == 50  # statistics fallback
    print("ok")


if __name__ == "__main__":
    arg = sys.argv[1] if len(sys.argv) > 1 else "all"
    if arg == "--selftest":
        selftest()
    elif arg == "all":
        profile("movies")
        profile("tv")
    elif arg in SVC:
        profile(arg)
    else:
        sys.exit(__doc__)
