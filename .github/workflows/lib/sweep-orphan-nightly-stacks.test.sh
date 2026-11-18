#!/usr/bin/env bash
# Self-check for the runner host's orphan sweep. Stubs docker, pgrep and the
# sweep itself, so it runs without a daemon. Each test is named after the bug
# it pins.
# Run from repo root: bash .github/workflows/lib/sweep-orphan-nightly-stacks.test.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/../../../roles/github_docker_runners/files/sweep-orphan-nightly-stacks.sh"
pass=0 fail=0

ok() { echo "PASS: $1"; pass=$((pass + 1)); }
no() { echo "FAIL: $1 ($2)"; fail=$((fail + 1)); }
holds() { local name=$1; shift; if "$@"; then ok "$name"; else no "$name" "not true: $*"; fi; }
# Both check the exit status too: a script that crashed also "did not sweep".
swept() { if [ -e "$WORK/swept" ]; then ok "$1"; else no "$1" "the sweep never ran, exit $RC"; fi; }
spared() { if [ ! -e "$WORK/swept" ] && [ "$RC" = "${2:-0}" ]; then ok "$1"; else no "$1" "swept or exit $RC, wanted ${2:-0}"; fi; }

# The host as the case describes it. reset, change what the case is about, tick.
reset() {
  WORK=$(mktemp -d)
  mkdir "$WORK/bin"
  PGREP_RC=1 DOCKER_RC=0 SWEEP_RC=0
  CONTAINERS="github-runner-1" NETWORKS="bridge" VOLUMES="" IMAGES="myoung34/github-runner"
}

# One firing of the timer.
tick() {
  printf '#!/usr/bin/env bash\necho "$*" > %s\nexit %s\n' "$WORK/pgrep-args" "$PGREP_RC" > "$WORK/bin/pgrep"
  cat > "$WORK/bin/docker" <<STUBEOF
#!/usr/bin/env bash
[ "$DOCKER_RC" = 0 ] || exit "$DOCKER_RC"
case "\$1" in
  ps) printf '%s\n' "$CONTAINERS" ;;
  network) printf '%s\n' "$NETWORKS" ;;
  volume) printf '%s\n' "$VOLUMES" ;;
  images) printf '%s\n' "$IMAGES" ;;
esac
STUBEOF
  printf '#!/usr/bin/env bash\ntouch %s\nexit %s\n' "$WORK/swept" "$SWEEP_RC" > "$WORK/sweep"
  chmod +x "$WORK/bin/pgrep" "$WORK/bin/docker"
  rm -f "$WORK/swept"
  PATH="$WORK/bin:$PATH" STAMP="$WORK/stamp" SWEEP="$WORK/sweep" bash "$SCRIPT" > "$WORK/out" 2>&1
  RC=$?
}

seen_ago() { echo $(( $(date +%s) - $1 )) > "$WORK/stamp"; }

# Runs 43944775568 and 44152849160 lost their runner mid-benchmark. The stack's
# containers kept running with nothing to stop them, and the disk held 12GB of
# it until the midnight prune.
reset; CONTAINERS="mem0-nightly-111-mem0-1"
tick
spared "the first sighting of an orphan only starts the clock"
holds "the first sighting records when, as a time the next tick can read" grep -qE '^[0-9]+$' "$WORK/stamp"
seen_ago 601
tick
swept "an orphan past the grace period is swept"
holds "the sweep says so in the journal" grep -q sweeping "$WORK/out"

# run.sh exits a little before the workflow's own teardown runs. A sweep in
# that gap removes containers from under it and fails a run that was fine.
reset; CONTAINERS="mem0-nightly-111-mem0-1"; seen_ago 590
tick
spared "an orphan inside the grace period is left for the workflow's teardown"

# Each kind of object is a stack by itself. A dead run whose containers are
# gone still owns the images, which are most of the space.
for kind in CONTAINERS NETWORKS VOLUMES IMAGES; do
  reset; printf -v "$kind" '%s' "mem0-nightly-111-thing"; seen_ago 601
  tick
  swept "${kind} alone count as a stack"
done

# The benchmark runs for hours with its containers up. Sweeping then would
# kill the run it is there to clean up after.
reset; CONTAINERS="mem0-nightly-111-mem0-1"; PGREP_RC=0; seen_ago 601
tick
spared "a stack with a live job is never swept"
holds "a live job clears a stamp left by the stack before it" test ! -e "$WORK/stamp"
holds "the job is looked for by full command line" grep -q -- '^-f ' "$WORK/pgrep-args"
holds "run.sh is found by its path in the checkout" grep -qF 'tests/quality/nightly/run' "$WORK/pgrep-args"
# run.sh started by hand from its own directory has no path in its command
# line. Its compose calls carry the project name.
holds "a hand-started run is found by its compose project" grep -qF 'mem0-nightly-' "$WORK/pgrep-args"

# "I could not look" must not read as "nothing there". For pgrep that reading
# sweeps a live benchmark; for docker it resets the clock on a dead stack
# forever with nothing going failed.
reset; CONTAINERS="mem0-nightly-111-mem0-1"; PGREP_RC=2; seen_ago 601
tick
spared "a pgrep that fails sweeps nothing and fails the unit" 1
reset; DOCKER_RC=1; seen_ago 601
tick
spared "a docker that cannot list fails the unit" 1
holds "a docker that cannot list keeps the stamp" test -s "$WORK/stamp"

# The usual state of the host. A stamp surviving it would let the next stack
# be swept the moment its job exits, with no grace at all.
reset; seen_ago 601
tick
spared "a host with no nightly stack sweeps nothing"
holds "no stack clears the stamp" test ! -e "$WORK/stamp"

# Another project's objects that merely contain the name are not ours.
reset; CONTAINERS="not-mem0-nightly-1"; seen_ago 601
tick
spared "a name that only contains the prefix is not a stack"

# A stamp nothing can read as a time would otherwise fail every tick, or sweep
# at once.
reset; CONTAINERS="mem0-nightly-111-mem0-1"; echo "garbage" > "$WORK/stamp"
tick
spared "an unreadable stamp starts the clock again"
holds "an unreadable stamp is replaced by a time" grep -qE '^[0-9]+$' "$WORK/stamp"

# A stack that will not go is the disk not coming back. SystemdUnitFailed needs
# the unit failed for 5 minutes, so one failed tick followed by a quiet
# "first sighting" would never raise it.
reset; CONTAINERS="mem0-nightly-111-mem0-1"; SWEEP_RC=1; seen_ago 601
tick
holds "a sweep that fails fails the unit" test "$RC" = 1
tick
swept "a failed sweep is retried on the next tick, not after another grace period"
holds "and fails the unit again" test "$RC" = 1

echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
