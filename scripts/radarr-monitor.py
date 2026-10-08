#!/usr/bin/env python3
"""Make sure a hand-written list of films is monitored in Radarr, so a
watchlist pasted from anywhere ends up searched for without clicking through
the UI one title at a time.

Input is one film per line as "Title (Year)", from a file or stdin. Blank lines
and lines without a year are skipped. Each line resolves to the first Radarr
lookup hit within a year of the given year, then:

  OK       already in Radarr and monitored, nothing to do
  MONITOR  in Radarr but unmonitored: set monitored, search if it has no file
  ADD      not in Radarr: add monitored to /movies and search
  NOMATCH  no lookup hit near that year; prints the top hits so you can fix it

Dry run by default; --apply makes the changes.

  radarr-monitor.py watchlist.txt
  radarr-monitor.py --apply watchlist.txt
  printf 'Ikiru (1952)\\nTokyo Story (1953)\\n' | radarr-monitor.py --apply
"""
import fileinput
import re
import sys

import media_clients as m

# NOTE: "HD REMUX + WEB", the profile most of the library uses. If profile ids
# get renumbered, adds land on whatever profile takes id 7.
QUALITY_PROFILE_ID = 7
ROOT_FOLDER = "/movies"
LINE = re.compile(r"^\s*(.+?)\s*\((\d{4})\)\s*$")


def films(lines):
    for line in lines:
        hit = LINE.match(line)
        if hit:
            yield hit.group(1), int(hit.group(2))


def main(argv):
    apply = "--apply" in argv
    paths = [a for a in argv if a != "--apply"]
    lib = {x["tmdbId"]: x for x in m.get("radarr", "/api/v3/movie")}
    for title, year in films(fileinput.input(paths)):
        res = m.get("radarr", "/api/v3/movie/lookup", term=title)
        hits = [r for r in res if abs(r.get("year", 0) - year) <= 1]
        if not hits:
            print(f"NOMATCH  {title} ({year}): top={[(r['title'], r.get('year')) for r in res[:3]]}")
            continue
        r = hits[0]
        tag = f"{r['title']} ({r['year']}) tmdb={r['tmdbId']}"
        have = lib.get(r["tmdbId"])
        if have and have["monitored"]:
            print(f"OK       {tag} hasFile={have['hasFile']}")
            continue
        if have:
            print(f"MONITOR  {tag}")
            if not apply:
                continue
            have["monitored"] = True
            m._request("radarr", f"/api/v3/movie/{have['id']}", {}, data=have, method="PUT")
            if not have["hasFile"]:
                m.post("radarr", "/api/v3/command", {"name": "MoviesSearch", "movieIds": [have["id"]]})
            continue
        print(f"ADD      {tag}")
        if not apply:
            continue
        r.update(qualityProfileId=QUALITY_PROFILE_ID, rootFolderPath=ROOT_FOLDER, monitored=True,
                 minimumAvailability="released",
                 addOptions={"searchForMovie": True, "monitor": "movieOnly"})
        m.post("radarr", "/api/v3/movie", r)


if __name__ == "__main__":
    main(sys.argv[1:])
