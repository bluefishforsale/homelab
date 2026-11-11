#!/usr/bin/env python3
"""Tell terrac, loudly, when disc-sourced x265 10-bit movie encodes are worth taking.

Radarr scores `x265 (HD)` at -10000 because most 1080p x265 movie releases are
re-encodes that strip grain. That is a bet on today's release groups, not a law:
the day vetted groups ship x265 SDR straight from the Blu-ray at about 10 Mbps
as routine, movies should prefer them the way TV already does (Sonarr's
`x265 10-bit SDR` +2000). Nothing here changes a score or grabs a file; it only
watches and pushes.

Two signals, both weekly:
  - Radarr searches for a few grainy reference films. A candidate is 1080p,
    Blu-ray sourced, x265/HEVC, SDR, at 7-13 Mbps. Only candidates carrying a
    TRaSH `HD Bluray Tier` format count, since a tier is TRaSH vetting the group
    as encoding from the disc. A new tiered group, a group moving up a tier, or
    a Tier 01/02 group on two or more films pushes at ntfy's top priority.
  - TRaSH's own definitions of `x265 (HD)` and the HD Bluray tiers. Any edit
    pushes loud too, since that is TRaSH changing its mind.
A quiet digest goes once a month so silence still proves the watcher runs.

  x265-watch.py run       # search, compare with last state, push what changed
  x265-watch.py run --dry # same, print instead of pushing, keep the old state
"""
import argparse
import base64
import datetime as dt
import hashlib
import json
import os
import re
import sys
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import media_clients as mc  # noqa: E402

STATE = os.path.expanduser(os.environ.get("X265_WATCH_STATE", "~/.local/state/x265-watch.json"))
SEARCH_TIMEOUT = 600
# Grainy, dark films where a starved encode shows first. By TMDb id, so a
# rename or re-add in Radarr does not lose them.
REFERENCE = {438631: "Dune (2021)", 693134: "Dune: Part Two (2024)", 872585: "Oppenheimer (2023)",
             335984: "Blade Runner 2049 (2017)", 76341: "Mad Max: Fury Road (2015)", 414906: "The Batman (2022)"}
# Around the ~10 Mbps movie target: below is the micro-encode crowd, above is
# not saving anything over the x264 encodes already taken.
MBPS = (7.0, 13.0)
X265 = re.compile(r"[xh][ ._-]?265|\bHEVC(\b|\d)", re.IGNORECASE)
NOT_WANTED = re.compile(r"\b(HDR(10)?(\+|Plus)?|DV|DoVi|Dolby[ ._-]?Vision|HLG|UHD|REMUX)\b", re.IGNORECASE)
TIER = re.compile(r"^HD Bluray Tier 0(\d)$")
TRASH = "https://raw.githubusercontent.com/TRaSH-Guides/Guides/master/docs/json/radarr/cf/{}.json"
TRASH_FORMATS = ("x265-hd", "x265-no-hdrdv", "hd-bluray-tier-01", "hd-bluray-tier-02", "hd-bluray-tier-03")


def mbps(rel, runtime):
    return rel["size"] * 8 / (runtime * 60) / 1e6


def candidate(rel, runtime):
    q = rel["quality"]["quality"]
    return (q.get("source") == "bluray" and q.get("resolution") == 1080
            and bool(X265.search(rel["title"])) and not NOT_WANTED.search(rel["title"])
            and MBPS[0] <= mbps(rel, runtime) <= MBPS[1])


def tier(rel):
    for cf in rel.get("customFormats", []):
        m = TIER.match(cf["name"])
        if m:
            return int(m.group(1))
    return None


def observe(results):
    """{film: (runtime, releases)} -> {group: {"tier": best, "films": [...]}} for tiered candidates."""
    seen = {}
    for film, (runtime, rels) in results.items():
        for r in rels:
            t = tier(r)
            if t is None or not candidate(r, runtime):
                continue
            g = seen.setdefault(r.get("releaseGroup") or r["title"], {"tier": t, "films": []})
            g["tier"] = min(g["tier"], t)
            if film not in g["films"]:
                g["films"].append(film)
    for g in seen.values():
        g["films"].sort()
    return seen


def changes(before, after):
    out = []
    for group, now in sorted(after.items()):
        was = before.get(group)
        if was is None:
            out.append(f"New vetted x265 SDR group: {group} (Tier 0{now['tier']}) on {', '.join(now['films'])}")
            continue
        if now["tier"] < was["tier"]:
            out.append(f"{group} moved up to Tier 0{now['tier']} (was 0{was['tier']})")
        if now["tier"] <= 2 and len(now["films"]) >= 2 > len(was["films"]):
            out.append(f"{group} (Tier 0{now['tier']}) now has x265 SDR on {len(now['films'])} reference films")
    return out


def trash_changes(before, after):
    return [f"TRaSH changed its `{name}` definition" for name, h in sorted(after.items())
            if name in before and before[name] != h]


def digest_due(state, month):
    return state.get("digest_month") != month


def trash_hashes():
    out = {}
    for name in TRASH_FORMATS:
        try:
            with urllib.request.urlopen(TRASH.format(name), timeout=30) as r:
                body = json.load(r)
        except Exception as e:  # noqa: BLE001  (a blip skips this run's comparison, see trash_changes)
            print(f"trash {name}: {e}", file=sys.stderr)
            continue
        out[name] = hashlib.sha256(json.dumps(body, sort_keys=True).encode()).hexdigest()
    return out


def search():
    movies = {m["tmdbId"]: m for m in mc.get("radarr", "/api/v3/movie")}
    results = {}
    for tmdb, film in REFERENCE.items():
        m = movies.get(tmdb)
        if not m:
            print(f"{film}: not in Radarr, skipped", file=sys.stderr)
            continue
        rels = mc.get("radarr", "/api/v3/release", movieId=m["id"], timeout=SEARCH_TIMEOUT)
        results[film] = (m["runtime"], rels)
        print(f"{film}: {len(rels)} releases, "
              f"{sum(candidate(r, m['runtime']) for r in rels)} x265 SDR in band")
    return results


def push(title, body, priority, tags):
    url = os.environ["ALERT_NTFY_URL"]
    headers = {"Title": title, "Priority": str(priority), "Tags": tags}
    if os.environ.get("ALERT_NTFY_USER"):
        cred = f"{os.environ['ALERT_NTFY_USER']}:{os.environ.get('ALERT_NTFY_PASS', '')}"
        headers["Authorization"] = "Basic " + base64.b64encode(cred.encode()).decode()
    urllib.request.urlopen(urllib.request.Request(url, data=body.encode(), headers=headers), timeout=30)


def run(dry):
    state = {}
    if os.path.exists(STATE):
        with open(STATE) as f:
            state = json.load(f)
    seen = observe(search())
    hashes = trash_hashes()
    first = not state
    loud = [] if first else changes(state.get("groups", {}), seen) + trash_changes(state.get("trash", {}), hashes)
    month = dt.date.today().strftime("%Y-%m")
    summary = "\n".join(f"{g}: Tier 0{v['tier']} on {', '.join(v['films'])}" for g, v in sorted(seen.items())) \
        or "No vetted x265 SDR group in the 7-13 Mbps band."
    digest = first or digest_due(state, month)
    out = []
    if loud:
        out.append(("Movies: x265 10-bit may be worth taking now",
                    "\n".join(loud) + "\n\nNext: grain test against the approved x264 copy, then decide on "
                    "Radarr's x265 (HD) -10000.\n\n" + summary, 5, "rotating_light,movie_camera"))
    if digest:
        out.append(("x265 watch: monthly digest", summary, 2, "movie_camera"))
    for title, body, prio, tags in out:
        print(f"[p{prio}] {title}\n{body}\n")
        if not dry:
            push(title, body, prio, tags)
    if dry:
        return
    # Groups merge rather than replace, so a group missing from one week's
    # indexer results is not "new" again when it reappears.
    groups = state.get("groups", {})
    for g, v in seen.items():
        old = groups.get(g, {"tier": v["tier"], "films": []})
        groups[g] = {"tier": min(old["tier"], v["tier"]), "films": sorted(set(old["films"]) | set(v["films"]))}
    trash = {**state.get("trash", {}), **hashes}
    os.makedirs(os.path.dirname(STATE), exist_ok=True)
    with open(STATE + ".tmp", "w") as f:
        json.dump({"groups": groups, "trash": trash,
                   "digest_month": month if digest else state.get("digest_month")}, f, indent=1)
    os.replace(STATE + ".tmp", STATE)


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("--dry", action="store_true")
    a = p.parse_args()
    run(a.dry)


if __name__ == "__main__":
    main()
