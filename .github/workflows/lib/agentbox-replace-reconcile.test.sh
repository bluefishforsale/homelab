#!/usr/bin/env bash
# Guard rails on the [replace] issue reconciler.
# Run from repo root: bash .github/workflows/lib/agentbox-replace-reconcile.test.sh
#
# This is the only automation in the fleet that closes hardware issues, on the
# pool where losing data is the worst possible outcome. The dangerous failure is
# not a crash — it is closing the ticket for a drive that is still dying, which
# looks like success. So the tests are written as "what must it refuse to do".
#
# Prometheus and gh are stubbed; the real decision logic runs unmodified.
set -uo pipefail
cd "$(dirname "$0")/../../.." || exit 1

python3 - "$PWD/files/agentbox/replace-issue-reconcile.py" <<'PY'
import importlib.util, sys, types

spec = importlib.util.spec_from_file_location("rec", sys.argv[1])
rec = importlib.util.module_from_spec(spec)
spec.loader.exec_module(rec)

PASS = FAIL = 0
def ok(m):
    global PASS; PASS += 1; print(f"PASS: {m}")
def bad(m):
    global FAIL; FAIL += 1; print(f"FAIL: {m}")

def run(issues, live, healthy=True, absent_map=None, prom_fails=False):
    """Drive main() with stubs; return (closed, flagged) issue numbers."""
    absent_map = absent_map or {}
    closed, flagged = [], []

    def fake_prom(q):
        if prom_fails:
            return None
        if q == "zpool_health":
            return [] if healthy else [{"metric": {"pool": "data01"}, "value": [0, "1"]}]
        if q.startswith("group by"):
            return [{"metric": {"device": d}} for d in live]
        if q.startswith("absent_over_time"):
            dev = q.split('device="', 1)[1].split('"', 1)[0]
            # non-empty result == the series has been absent for the whole window
            return [{"metric": {}, "value": [0, "1"]}] if absent_map.get(dev) else []
        raise AssertionError(f"unexpected query: {q}")

    rec.prom = fake_prom
    rec.open_replace_issues = lambda: issues
    rec.act = lambda i, r, c, a: closed.append(i["number"])
    rec.flag = lambda i, r, c, a: flagged.append(i["number"])
    sys.argv = ["rec"]
    rec.main()
    return sorted(closed), sorted(flagged)

def issue(n, dev):
    return {"number": n, "marker": f"ocean.home/{dev}", "device": dev}

D_GONE, D_LIVE = "wwn-0x5000c500b2281882", "wwn-0x5000c500b345abd9"

# 1. A drive still reporting is never closed, no matter what else is true.
#    absent_map deliberately says "absent for the whole window" for a device
#    that IS in `live` — a contradiction Prometheus should never produce. That
#    is the point: membership must refuse on its own. Without this the absence
#    check masks the membership check, and deleting the membership guard leaves
#    the suite green (verified by mutation).
c, f = run([issue(1, D_LIVE)], live=[D_LIVE], absent_map={D_LIVE: True})
ok("a device still in the pool is kept open") if c == [] and f == [1] \
    else bad(f"still-in-pool must not close (closed={c})")

# 2. Zero error counters are NOT grounds to close. Counters reset on `zpool
#    clear` and on replace, so a dying drive reads clean. This is the assertion
#    that keeps the automation honest.
c, f = run([issue(1, D_LIVE)], live=[D_LIVE], absent_map={D_LIVE: True})
ok("zero counters alone never close an issue") if c == [] \
    else bad("closed a drive that is still present (counters are not health)")

# 3. Gone from the metrics for the full window, pool healthy -> physically swapped.
c, f = run([issue(1, D_GONE)], live=[D_LIVE], absent_map={D_GONE: True})
ok("a device absent for the whole window is closed") if c == [1] \
    else bad(f"should close a swapped drive (closed={c})")

# 4. Absent right now but seen inside the window: a scrape gap or exporter
#    restart, not a swap. Closing here would retire a live drive's ticket.
c, f = run([issue(1, D_GONE)], live=[D_LIVE], absent_map={D_GONE: False})
ok("a brief absence is not treated as a swap") if c == [] and f == [1] \
    else bad(f"closed on a transient gap (closed={c})")

# 5. A kernel name is not stable across reboots, so its absence proves nothing.
c, f = run([issue(1, "sde")], live=[D_LIVE], absent_map={"sde": True})
ok("a kernel-named device is never closed") if c == [] and f == [1] \
    else bad(f"closed on an unstable device id (closed={c})")

# 6. A degraded pool may be a half-finished swap. Do nothing at all.
c, f = run([issue(1, D_GONE)], live=[D_LIVE], healthy=False, absent_map={D_GONE: True})
ok("nothing is touched while a pool is unhealthy") if c == [] and f == [] \
    else bad(f"acted during a degraded pool (closed={c}, flagged={f})")

# 7. Prometheus unreachable must mean "do nothing", never "nothing found".
c, f = run([issue(1, D_GONE)], live=[], prom_fails=True)
ok("an unreachable Prometheus closes nothing") if c == [] and f == [] \
    else bad(f"acted without evidence (closed={c}, flagged={f})")

# 8. Duplicates collapse to the lowest number, and only the redundant ones close.
c, f = run([issue(5, D_LIVE), issue(6, D_LIVE)], live=[D_LIVE])
ok("duplicates close the higher number and keep the original") if c == [6] and f == [5] \
    else bad(f"duplicate handling wrong (closed={c}, flagged={f})")

print()
print(f"passed: {PASS}  failed: {FAIL}")
sys.exit(1 if FAIL else 0)
PY
