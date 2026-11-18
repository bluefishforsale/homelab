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
swept() { if [ -e "$WORK/swept" ]; then ok "$1"; else no "$1" "the sweep never ran"; fi; }
holds() { local name=$1; shift; if "$@"; then ok "$name"; else no "$name" "not true: $*"; fi; }
spared() { if [ -e "$WORK/swept" ]; then no "$1" "the sweep ran"; else ok "$1"; fi; }

# run_case <run.sh alive: yes|no> <docker's listing> <stamp content, or empty for none> [sweep's exit status]
run_case() {
  WORK=$(mktemp -d)
  mkdir "$WORK/bin"
  printf '#!/usr/bin/env bash\n[ %s = yes ]\n' "$1" > "$WORK/bin/pgrep"
  { echo '#!/usr/bin/env bash'; echo "cat <<'EOF'"; printf '%s\n' "$2"; echo 'EOF'; } > "$WORK/bin/docker"
  printf '#!/usr/bin/env bash\ntouch %s\nexit %s\n' "$WORK/swept" "${4:-0}" > "$WORK/sweep"
  chmod +x "$WORK/bin/pgrep" "$WORK/bin/docker"
  [ -z "$3" ] || echo "$3" > "$WORK/stamp"
  PATH="$WORK/bin:$PATH" STAMP="$WORK/stamp" SWEEP="$WORK/sweep" bash "$SCRIPT" > /dev/null 2>&1
  RC=$?
}

# Runs 43944775568 and 44152849160 lost their runner mid-benchmark. The stack's
# containers kept running with nothing to stop them, and the disk held 12GB of
# it until the midnight prune.
run_case no "mem0-nightly-111-mem0-1" 0
swept "a stack with no run.sh past the grace period is swept"
holds "a sweep clears the stamp, so the next orphan gets its own grace period" test ! -e "$WORK/stamp"

# A dead run whose containers are gone still owns the images, which are most of
# the space.
run_case no "mem0-nightly-111-mem0" 0
swept "images alone count as a stack"

# The benchmark runs for hours with its containers up. Sweeping then would
# kill the run it is there to clean up after.
run_case yes "mem0-nightly-111-mem0-1" 0
spared "a stack whose run.sh is alive is never swept"
holds "a live run clears a stamp left by the stack before it" test ! -e "$WORK/stamp"

# run.sh exits a little before the workflow's own teardown runs. A sweep in
# that gap removes containers from under it and fails a run that was fine.
run_case no "mem0-nightly-111-mem0-1" ""
spared "the first sighting of an orphan only starts the clock"
holds "the first sighting records when" test -s "$WORK/stamp"

run_case no "mem0-nightly-111-mem0-1" "$(date +%s)"
spared "an orphan inside the grace period is left for the workflow's teardown"

# The usual state of the host. A stamp surviving it would let the next stack
# be swept the moment its run.sh exits, with no grace at all.
run_case no "github-runner-1" 0
spared "a host with no nightly stack sweeps nothing"
holds "no stack clears the stamp" test ! -e "$WORK/stamp"

# The unit has to go failed when the disk is not coming back, or nothing says so.
run_case no "mem0-nightly-111-mem0-1" 0 1
holds "a sweep that fails fails the unit" test "$RC" = 1

echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
