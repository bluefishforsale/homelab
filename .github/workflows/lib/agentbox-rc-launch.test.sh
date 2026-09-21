#!/usr/bin/env bash
# The RC launcher has to survive its own restart. Guard the shape of that.
# Run from repo root: bash .github/workflows/lib/agentbox-rc-launch.test.sh
#
# A lane killed before it deregisters leaves the folder claimed on claude.ai,
# and every restart then dies on "409: This folder is already served by a
# terminal `claude remote-control` on this device". Nothing alerts: the unit is
# activating, the watchdog restarts it, and restarting is the thing that does
# not work. It just stays dark for minutes. So assert the recovery shape.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
LAUNCH="$ROOT/files/agentbox/rc-launch.sh"
PASS=0
FAIL=0
ok()  { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# The launch decision is one `first || fallback` line. Everything below reads
# its two halves, so a rewrite that drops the fallback fails loudly here.
dispatch=$(grep -v '^[[:space:]]*#' "$LAUNCH" | grep -m1 '^rc .*||')
first=${dispatch%%||*}
fallback=${dispatch#*||}

# 1. The reclaim path exists at all. Without it a stale registration just 409s
#    every RestartSec until the server side ages it out.
case "$fallback" in
  *--continue*) ok "launcher falls back to --continue to reclaim a stale registration" ;;
  "") bad "no 'first || fallback' launch line; a stale 409 can only wait itself out" ;;
  *) bad "fallback is not --continue: '$fallback'" ;;
esac

# 2. Order: fresh session first, --continue only as the fallback. --continue
#    reattaches to a SESSION, so it comes up "Single session, exits when
#    complete" instead of a 0/32 capacity server. Leading with it silently
#    demotes every lane for as long as anything is recorded (~4h).
case "$first" in
  *--continue*) bad "--continue is tried first; lanes would demote to single-session" ;;
  *--name*--spawn=same-dir*) ok "a fresh --spawn=same-dir session is tried first" ;;
  *) bad "first attempt is not a fresh named same-dir session: '$first'" ;;
esac

# 3. --continue is rejected by the CLI alongside --spawn/--capacity/
#    --create-session-in-dir, so the reclaim attempt must not carry them.
case "$fallback" in
  *--spawn*|*--capacity*|*--create-session-in-dir*)
    bad "reclaim attempt carries a spawn flag; the CLI refuses that with --continue" ;;
  *) ok "--continue is not combined with the spawn flags" ;;
esac

# 4. expect exits 0 even when its child failed, so without an explicit wait the
#    409 reads as success and the fallback can never fire.
if grep -q 'catch wait' "$LAUNCH"; then
  ok "child exit status is propagated out of expect"
else
  bad "expect swallows the child status; a 409 would look like a clean exit"
fi

echo
echo "passed: $PASS  failed: $FAIL"
[[ "$FAIL" -eq 0 ]]
