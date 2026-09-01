#!/usr/bin/env python3
"""Retire [replace] hardware issues whose drive is demonstrably gone.

Rendered from files/agentbox/replace-issue-reconcile.py.

The alert receiver opens a [replace] issue when a drive throws errors, and
deliberately never closes it: a failing disk needs hands, so a human closes it
after the swap. But nothing emits an alert when a human swaps a disk, so a
webhook-driven receiver structurally cannot notice, and the issues accumulate
forever. Ten had piled up by 2026-09-01, every one of them stale.

This closes exactly two categories, both mechanical:

  1. The device no longer exists in the metrics at all for a full day, and the
     pool is healthy -> the drive was physically pulled. Unambiguous.
  2. Two open issues carry the same component marker -> one is redundant. Keep
     the lowest number, close the rest.

WHAT IT WILL NOT DO, and why:

  It never closes an issue because the error counters read zero. ZFS counters
  reset on `zpool clear` and on replace, so zero is evidence of a reset, not of
  a healthy disk. Closing on that would retire the warning about a drive that is
  still dying, on the pool where losing data is the worst outcome in the fleet.
  Those get a comment and a label so a human decides with SMART in hand.

  It never mutates ZFS, never SSHes to a storage host, and never reopens
  anything. Read-only Prometheus queries and issue comments/closes only.

Dry-run unless --yes, mirroring scripts/media-reclaim-delete.py.
"""
import argparse
import collections
import json
import os
import re
import subprocess
import sys
import urllib.parse
import urllib.request

# Config comes from the environment, NOT from ansible templating, and this file
# is installed with `copy` rather than `template` on purpose: the PromQL below
# contains `{{device="..."}}` (an f-string brace escape), which jinja would try
# to evaluate as an expression. Env vars sidestep that whole class.
PROM = os.environ.get("HOMELAB_PROM", "http://192.168.1.143:9090")
ISSUE_REPO = os.environ.get("ALERT_ISSUE_REPO", "bluefishforsale/homelab")
COMP_MARKER = "replace-component:"
STALE_LABEL = "premise-stale"
# A series vanishes briefly whenever the exporter restarts or a scrape gaps, so
# absence has to persist before it means "the drive is gone". A day is far longer
# than any restart and far shorter than the time a real swap sits unnoticed.
ABSENT_WINDOW = "24h"
# Only stable by-id names are trusted here, matching the receiver: a kernel name
# is not stable across reboots, so its absence proves nothing.
STABLE_DEV_RE = re.compile(r"^(wwn-|ata-|nvme-|scsi-|dm-uuid-|usb-)")


def prom(query):
    """Instant query -> list of result dicts. Returns None on any failure, and
    callers must treat None as 'do nothing' rather than 'nothing found'."""
    url = f"{PROM}/api/v1/query?" + urllib.parse.urlencode({"query": query})
    try:
        with urllib.request.urlopen(url, timeout=20) as r:
            d = json.load(r)
    except Exception as e:                      # noqa: BLE001 - report and bail
        print(f"prometheus query failed ({e}); doing nothing", file=sys.stderr)
        return None
    if d.get("status") != "success":
        print(f"prometheus returned {d.get('status')}; doing nothing", file=sys.stderr)
        return None
    return d["data"]["result"]


def gh_json(*args):
    r = subprocess.run(["gh", *args], capture_output=True, text=True)
    if r.returncode != 0:
        print(f"gh {' '.join(args)} failed: {r.stderr.strip()}", file=sys.stderr)
        return None
    return json.loads(r.stdout or "[]")


def open_replace_issues():
    issues = gh_json("issue", "list", "--repo", ISSUE_REPO, "--state", "open",
                     "--limit", "200", "--json", "number,title,body")
    if issues is None:
        return None
    out = []
    for i in issues:
        if not i["title"].startswith("[replace]"):
            continue
        m = re.search(re.escape(COMP_MARKER) + r"(\S+?)\s*-->", i["body"] or "")
        if not m:
            continue
        out.append({"number": i["number"], "marker": m.group(1),
                    "device": m.group(1).split("/")[-1]})
    return sorted(out, key=lambda x: x["number"])


def act(issue, reason, comment, apply):
    tag = "CLOSE" if apply else "would close"
    print(f"  #{issue['number']:<5} {issue['device']:30} {tag}: {reason}")
    if not apply:
        return
    subprocess.run(["gh", "issue", "comment", str(issue["number"]),
                    "--repo", ISSUE_REPO, "--body", comment],
                   capture_output=True, text=True)
    subprocess.run(["gh", "issue", "close", str(issue["number"]),
                    "--repo", ISSUE_REPO, "--reason", "completed"],
                   capture_output=True, text=True)


def flag(issue, reason, comment, apply):
    print(f"  #{issue['number']:<5} {issue['device']:30} keep open: {reason}")
    if not apply:
        return
    # Comment once. A second identical comment every day is how a useful signal
    # becomes noise a human filters out.
    existing = gh_json("issue", "view", str(issue["number"]), "--repo", ISSUE_REPO,
                       "--json", "comments") or {}
    if any(STALE_LABEL in (c.get("body") or "") for c in existing.get("comments", [])):
        return
    subprocess.run(["gh", "issue", "comment", str(issue["number"]),
                    "--repo", ISSUE_REPO, "--body", comment],
                   capture_output=True, text=True)
    subprocess.run(["gh", "issue", "edit", str(issue["number"]), "--repo", ISSUE_REPO,
                    "--add-label", STALE_LABEL], capture_output=True, text=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--yes", action="store_true", help="actually comment/close")
    args = ap.parse_args()
    apply = args.yes
    print(f"[replace] reconcile against {PROM} ({'APPLY' if apply else 'dry run'})")

    issues = open_replace_issues()
    if issues is None:
        print("could not list issues; doing nothing", file=sys.stderr)
        return 1
    if not issues:
        print("no open [replace] issues")
        return 0

    # A degraded pool means a swap may be half-done. Retiring paperwork in the
    # middle of that is exactly when a wrong close does the most harm.
    health = prom('zpool_health')
    if health is None:
        return 1
    unhealthy = [r["metric"].get("pool") for r in health if float(r["value"][1]) > 0]
    if unhealthy:
        print(f"pool(s) not healthy: {unhealthy}; doing nothing")
        return 0

    present = prom("group by (device) (zpool_device_errors)")
    if present is None:
        return 1
    live = {r["metric"].get("device") for r in present}
    print(f"devices currently reporting: {len(live)}")

    seen = collections.defaultdict(list)
    for i in issues:
        seen[i["marker"]].append(i)

    for marker, group in seen.items():
        first, rest = group[0], group[1:]
        for dup in rest:
            act(dup, f"duplicate of #{first['number']} (same component marker)",
                f"Closing as a duplicate of #{first['number']}: both track "
                f"`{dup['device']}` under the same `{COMP_MARKER}` marker. The "
                f"receiver minted both during one alert burst because GitHub's "
                f"issue list does not show an issue created seconds earlier.",
                apply)

        dev = first["device"]
        if not STABLE_DEV_RE.match(dev):
            flag(first, "kernel device name, absence proves nothing",
                 f"`{dev}` is a kernel device name, not a stable by-id name, so "
                 f"its absence from the metrics cannot show the drive was "
                 f"replaced. Leaving this for a human ({STALE_LABEL}).", apply)
            continue

        if dev in live:
            flag(first, "still in the pool",
                 f"`{dev}` is still reporting metrics, so the drive has not been "
                 f"replaced. Note that zero error counters would NOT mean it is "
                 f"healthy: ZFS counters reset on `zpool clear` and on replace. "
                 f"Check SMART before closing ({STALE_LABEL}).", apply)
            continue

        gone = prom(f'absent_over_time(zpool_device_errors{{device="{dev}"}}[{ABSENT_WINDOW}])')
        if gone is None:
            return 1
        if not gone:
            flag(first, f"absent now but seen within {ABSENT_WINDOW}",
                 f"`{dev}` is not reporting right now, but it was seen within the "
                 f"last {ABSENT_WINDOW}, so this could be a scrape gap or an "
                 f"exporter restart rather than a swap. Will re-check tomorrow.",
                 apply)
            continue

        act(first, f"gone from the metrics for {ABSENT_WINDOW}, pool healthy",
            f"Closing automatically: `{dev}` has reported no "
            f"`zpool_device_errors` series for {ABSENT_WINDOW} "
            f"(`absent_over_time(zpool_device_errors{{device=\"{dev}\"}}[{ABSENT_WINDOW}])`), "
            f"and every pool reports healthy. A device that has left the pool "
            f"entirely was physically replaced. Reopen if that is wrong.", apply)
    return 0


if __name__ == "__main__":
    sys.exit(main())
