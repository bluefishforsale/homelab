#!/usr/bin/env python3
"""Prunes what nzbget leaves behind on data01, so failed and abandoned downloads stop eating the pool.

nzbget keeps a failed job's intermediate files for a manual retry and never
deletes them when its history entry expires, and finished downloads Sonarr or
Radarr never imported sit in incoming forever. On 2026-10-19 that was 724 GiB
of intermediates and 1.25 TiB of incoming.
"""
import re

ORPHAN_AGE = 86400

_NZBID = re.compile(r"\.#(\d+)$")


def targets(entries, queue_ids, history, now):
    """(area, name, rule) for every entry that may be deleted; everything else is kept."""
    out = []
    for area, name, mtime in entries:
        m = _NZBID.search(name)
        if area != "intermediate" or not m:
            continue
        nzbid = int(m.group(1))
        if nzbid in queue_ids or nzbid in history:
            continue
        if now - mtime > ORPHAN_AGE:
            out.append((area, name, "orphan"))
    return out
