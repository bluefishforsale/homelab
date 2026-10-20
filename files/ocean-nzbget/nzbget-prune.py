#!/usr/bin/env python3
"""Prunes what nzbget leaves behind on data01, so failed and abandoned downloads stop eating the pool.

nzbget keeps a failed job's intermediate files for a manual retry and never
deletes them when its history entry expires, and finished downloads Sonarr or
Radarr never imported sit in incoming forever. On 2026-10-19 that was 724 GiB
of intermediates and 1.25 TiB of incoming.
"""
import re

DAY = 86400
ORPHAN_AGE = 1 * DAY
FAILED_AGE = 7 * DAY
INCOMING_AGE = 30 * DAY

_NZBID = re.compile(r"\.#(\d+)$")


def targets(entries, queue_ids, history, now):
    """(area, name, rule) for every entry that may be deleted; everything else is kept."""
    out = []
    for area, name, mtime in entries:
        if not name or name.startswith(".") or "/" in name:
            continue
        if area.startswith("incoming/"):
            if now - mtime > INCOMING_AGE:
                out.append((area, name, "stale-incoming"))
            continue
        m = _NZBID.search(name)
        if area != "intermediate" or not m:
            continue
        nzbid = int(m.group(1))
        if nzbid in queue_ids:
            continue
        age = now - mtime
        status = history.get(nzbid)
        if status is None and age > ORPHAN_AGE:
            out.append((area, name, "orphan"))
        elif status is not None and status.startswith("FAILURE") and age > FAILED_AGE:
            out.append((area, name, "failed"))
    return out


def plan(read_nzbget, entries, now):
    """targets() against nzbget's live state; nothing at all if that state can't be read."""
    # NOTE: without the queue every intermediate dir reads as an orphan,
    # including the download in progress, so a failed read must delete nothing.
    try:
        queue_ids, history = read_nzbget()
    except Exception:
        return []
    return targets(entries, queue_ids, history, now)
