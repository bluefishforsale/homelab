#!/usr/bin/env bash
# Guards against the failure that ran through this repo in five places:
# reporting success without having verified anything.
# Run from repo root: bash .github/workflows/lib/absence-is-not-health.test.sh
#
# These are shape assertions on files, not behaviour tests, because the two
# workflows only run on a self-hosted runner against live hosts. The shapes are
# the exact ones that were wrong, so a regression re-breaks a named assertion
# rather than silently going green again.
set -uo pipefail
cd "$(dirname "$0")/../../.." || exit 1

PASS=0; FAIL=0
ok()  { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

HC=.github/workflows/health-check.yml
MA=.github/workflows/main-apply.yml
M0=files/ocean-mem0/mem0-metrics.sh

# 1. health-check.yml gated its verdict purely on `[ -f results ]`, so a step
#    that died before writing contributed nothing and the run exited 0
#    "healthy". It must now notice a target it failed to check.
if grep -q 'UNCHECKED' "$HC" && grep -q 'steps.ocean.outcome' "$HC"; then
  ok "health-check notices a target whose check never completed"
else
  bad "health-check does not track unchecked targets (absence reads as health)"
fi

# Anchored to the EXIT path, not merely to the variable: $UNCHECKED also
# appears in the status block above, so a looser grep passed against a version
# whose exit had been neutered to `if false`. Verified by mutation.
if sed -n '/did not manage to check/,/^          \[ \$TOTAL_UNHEALTHY/p' "$HC" | grep -q 'exit 1' \
   && sed -n '/# Exit non-zero if anything/,/exit 0/p' "$HC" | grep -q 'if \[ -n "\$UNCHECKED" \]'; then
  ok "health-check exits non-zero when it could not look"
else
  bad "health-check can still exit 0 having checked nothing"
fi

# 2. main-apply counted a service that recovered on the third attempt as both
#    healthy and failed, because `break` leaves attempt at 3 either way.
if grep -qE 'if \[ \$attempt -eq 3 \]' "$MA"; then
  bad "main-apply still infers failure from the loop counter (double-counts a slow recovery)"
else
  ok "main-apply tracks success explicitly, not via the attempt counter"
fi

# 3. The mem0 collector turned an unmeasurable value into 0. A present series
#    reading 0 is indistinguishable from a real empty store, and absent() never
#    fires on it. Worst case: newest_memory_age_seconds 0 = "a memory just
#    arrived" while Postgres was unreachable.
if grep -q 'mem0_collector_up' "$M0"; then
  ok "mem0 collector publishes an explicit up/down signal"
else
  bad "mem0 collector has no way to say 'I could not measure'"
fi

if grep -qE '^memories=\$\(num ' "$M0"; then
  bad "mem0 collector still coerces a failed query to 0"
else
  ok "mem0 collector omits a metric it could not measure"
fi

# The named metrics must be emitted through the skip-if-empty helper, not with
# a bare echo that would publish an empty or zero value.
for m in mem0_memories_total mem0_newest_memory_age_seconds mem0_embed_probe_seconds; do
  if grep -qE "^\s*echo \"$m " "$M0"; then
    bad "$m is echoed unconditionally (publishes a value even when unmeasured)"
  else
    ok "$m is only published when measured"
  fi
done

# last_SUCCESS must not be stamped by a run that failed to measure anything,
# or a freshness alert is told the data is current when nothing was collected.
if grep -qE 'if \[ "\$DEGRADED" -eq 0 \]' "$M0"; then
  ok "last_success timestamp is only stamped on a fully successful run"
else
  bad "last_success timestamp is stamped even after a failed collection"
fi

echo
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -eq 0 ]
