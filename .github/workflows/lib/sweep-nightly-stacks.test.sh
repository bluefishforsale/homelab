#!/usr/bin/env bash
# Self-check for sweep-nightly-stacks.sh. Stubs the docker CLI so the sweep runs
# without a daemon. Each test is named after the bug it pins.
# Run from repo root: bash .github/workflows/lib/sweep-nightly-stacks.test.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/sweep-nightly-stacks.sh"
pass=0 fail=0

ok() { echo "PASS: $1"; pass=$((pass + 1)); }
no() { echo "FAIL: $1 ($2)"; fail=$((fail + 1)); }
check() { if grep -qxF "$2" "$CALLS"; then ok "$1"; else no "$1" "no call '$2' in: $(tr '\n' ';' < "$CALLS")"; fi; }
refute() { if grep -qF "$2" "$CALLS"; then no "$1" "unexpected '$2'"; else ok "$1"; fi; }
exited() { if [ "$RC" = "$2" ]; then ok "$1"; else no "$1" "exit $RC, wanted $2"; fi; }

# Runs the sweep with <stub-body> as the docker CLI. Every removal the sweep
# asks for lands in $CALLS, one docker invocation per line.
run_with_stub() {
  BIN=$(mktemp -d)
  { echo '#!/usr/bin/env bash'; echo "CALLS='$CALLS'"; cat "$1"; } > "$BIN/docker"
  chmod +x "$BIN/docker"
  PATH="$BIN:$PATH" bash "$SCRIPT" > /dev/null 2>&1
  RC=$?
  rm -rf "$BIN"
}

CALLS=$(mktemp); STUB=$(mktemp)

# 2026-11-15: three nightly runs died at the job timeout and one to a host
# reboot. Their stacks stayed up for days, `compose down` skipped the bench
# containers because they are `compose run` one-offs, and gh-runner-01 hit 91%.
cat > "$STUB" <<'STUBEOF'
case "$1 $2" in
  "ps -a") printf '%s\n' mem0-nightly-111-mem0-1 mem0-nightly-111-bench mem0-nightly-222-ollama-1 github-runner-1 node-exporter ;;
  "network ls") printf '%s\n' bridge mem0-nightly-111_default github-runners_default ;;
  "volume ls") printf '%s\n' mem0-nightly-222_pgdata github-runners_cache ;;
  "images --format") printf '%s\n' mem0-nightly-111-mem0:latest mem0-nightly-111-bench:latest ollama/ollama:0.40.4 ;;
  *) echo "$*" >> "$CALLS" ;;
esac
STUBEOF
: > "$CALLS"
run_with_stub "$STUB"
exited "a host with leftovers sweeps clean and succeeds" 0
check "service and one-off containers go, with their anonymous volumes" "rm -f -v mem0-nightly-111-mem0-1 mem0-nightly-111-bench mem0-nightly-222-ollama-1"
check "the run's network goes" "network rm mem0-nightly-111_default"
check "the run's named volume goes" "volume rm mem0-nightly-222_pgdata"
check "the run's built images go, the one-off's included" "rmi mem0-nightly-111-mem0:latest mem0-nightly-111-bench:latest"
refute "the runners sharing this docker host are never touched" "github-runner"
refute "a pulled base image is left for the next run" "ollama/ollama"

# The usual case. An empty list must not turn into a bare `docker rm`, which
# errors and would fail every clean run.
cat > "$STUB" <<'STUBEOF'
case "$1 $2" in
  "ps -a") printf '%s\n' github-runner-1 ;;
  "network ls") printf '%s\n' bridge ;;
  "volume ls"|"images --format") : ;;
  *) echo "$*" >> "$CALLS" ;;
esac
STUBEOF
: > "$CALLS"
run_with_stub "$STUB"
exited "nothing to sweep is a success" 0
if [ -s "$CALLS" ]; then no "nothing to sweep removes nothing" "$(cat "$CALLS")"; else ok "nothing to sweep removes nothing"; fi

# A stack that will not go is the disk filling again. The step has to say so
# rather than report a clean sweep, and it still has to try the rest.
cat > "$STUB" <<'STUBEOF'
case "$1 $2" in
  "ps -a") printf '%s\n' mem0-nightly-111-mem0-1 ;;
  "network ls") printf '%s\n' mem0-nightly-111_default ;;
  "volume ls"|"images --format") : ;;
  "rm -f") echo "$*" >> "$CALLS"; exit 1 ;;
  *) echo "$*" >> "$CALLS" ;;
esac
STUBEOF
: > "$CALLS"
run_with_stub "$STUB"
exited "a removal that fails fails the step" 1
check "a failed container removal does not stop the network sweep" "network rm mem0-nightly-111_default"

# Each kind of removal has to be able to fail the step on its own. With only
# the container case pinned, losing the status of the other three still passed.
for doomed in "network rm" "volume rm" "rmi mem0"; do
  cat > "$STUB" <<STUBEOF
case "\$1 \$2" in
  "ps -a"|"network ls"|"volume ls") printf '%s\n' mem0-nightly-111_thing ;;
  "images --format") printf '%s\n' mem0-nightly-111-mem0:latest ;;
  "$doomed"*) exit 1 ;;
  *) echo "\$*" >> "\$CALLS" ;;
esac
STUBEOF
  : > "$CALLS"
  run_with_stub "$STUB"
  exited "a failed '${doomed%% mem0}' fails the step" 1
done

# A docker that cannot list is not a host with nothing on it. Reading an
# unreachable daemon as "clean" is the same absence-is-not-health mistake the
# sweep exists to stop.
cat > "$STUB" <<'STUBEOF'
echo "Cannot connect to the Docker daemon" >&2
exit 1
STUBEOF
: > "$CALLS"
run_with_stub "$STUB"
exited "a docker that cannot list fails the step" 1

rm -f "$CALLS" "$STUB"
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
