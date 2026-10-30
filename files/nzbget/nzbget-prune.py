#!/usr/bin/env python3
"""Prunes what nzbget leaves behind, so failed and abandoned downloads stop eating its scratch SSD and data01.

nzbget keeps a failed job's intermediate files for a manual retry and never
deletes them when its history entry expires, and finished downloads Sonarr or
Radarr never imported sit in incoming forever. On 2026-10-19 that was 724 GiB
of intermediates and 1.25 TiB of incoming.
"""
import base64
import json
import os
import re
import shutil
import sys
import tempfile
import time
import urllib.request

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


def _rpc(method):
    url = f"{os.environ['NZBGET_URL']}/jsonrpc/{method}"
    basic = base64.b64encode(f"{os.environ['NZBGET_USER']}:{os.environ['NZBGET_PASS']}".encode()).decode()
    req = urllib.request.Request(url, headers={"Authorization": f"Basic {basic}"})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)["result"]


def _read_nzbget():
    queue_ids = {g["NZBID"] for g in _rpc("listgroups")}
    history = {g["NZBID"]: g["Status"] for g in _rpc("history")}
    return queue_ids, history


def _scan(areas):
    entries = []
    for area, path in areas.items():
        with os.scandir(path) as it:
            for e in it:
                entries.append((area, e.name, e.stat(follow_symlinks=False).st_mtime))
    return entries


def _size(path):
    if not os.path.isdir(path) or os.path.islink(path):
        return os.lstat(path).st_size
    total = 0
    for root, _, files in os.walk(path):
        for f in files:
            try:
                total += os.lstat(os.path.join(root, f)).st_size
            except FileNotFoundError:
                pass
    return total


def _write_metrics(path, deleted, now):
    lines = ["# HELP nzbget_prune_last_success_timestamp_seconds End of the last run that read nzbget and finished.",
             "# TYPE nzbget_prune_last_success_timestamp_seconds gauge",
             f"nzbget_prune_last_success_timestamp_seconds {now:.0f}",
             "# HELP nzbget_prune_deleted_bytes Bytes deleted by the last run, by rule.",
             "# TYPE nzbget_prune_deleted_bytes gauge",
             "# HELP nzbget_prune_deleted_items Entries deleted by the last run, by rule.",
             "# TYPE nzbget_prune_deleted_items gauge"]
    for rule in ("orphan", "failed", "stale-incoming"):
        items, size = deleted.get(rule, (0, 0))
        lines += [f'nzbget_prune_deleted_bytes{{rule="{rule}"}} {size}',
                  f'nzbget_prune_deleted_items{{rule="{rule}"}} {items}']
    # Root writes here; a fixed tmp name would follow a planted symlink.
    fd, tmp = tempfile.mkstemp(dir=os.path.dirname(path), suffix=".tmp")
    with os.fdopen(fd, "w") as f:
        f.write("\n".join(lines) + "\n")
    os.chmod(tmp, 0o644)
    os.rename(tmp, path)


def main():
    dry_run = "--dry-run" in sys.argv[1:]
    data = os.environ.get("DATA", "/data01")
    areas = {"intermediate": os.environ["INTERMEDIATE"],
             "incoming/movies": f"{data}/incoming/movies",
             "incoming/tv": f"{data}/incoming/tv"}

    failure = []

    def read():
        try:
            return _read_nzbget()
        except Exception as e:
            failure.append(e)
            raise

    now = time.time()
    chosen = plan(read, _scan(areas), now)
    if failure:
        print(f"nzbget unreadable, pruning nothing: {failure[0]}", file=sys.stderr)
        return 1

    deleted = {}
    for area, name, rule in chosen:
        path = os.path.join(areas[area], name)
        size = _size(path)
        print(f"{'would prune' if dry_run else 'prune'} {rule} {size / 2**30:.1f}GiB {path}")
        if dry_run:
            continue
        if os.path.isdir(path) and not os.path.islink(path):
            shutil.rmtree(path)
        else:
            os.unlink(path)
        items, total = deleted.get(rule, (0, 0))
        deleted[rule] = (items + 1, total + size)

    if not dry_run:
        _write_metrics(os.environ["TEXTFILE"], deleted, time.time())
    return 0


if __name__ == "__main__":
    sys.exit(main())
