#!/usr/bin/env bash
# Removes a mem0 nightly stack whose job is gone, from a timer on the runner host.
#
# The workflow sweeps at the start and at the end of every run, and neither
# covers a run that loses its runner (a host shutdown, a killed container): the
# end never happens and the start is the next night. Until then the dead stack
# holds 12 to 15GB of a 35GB disk, and any other run that builds before the
# midnight prune stacks on top of it. Runs 43944775568 and 44152849160 both
# ended that way and sat on the disk until 00:00.
#
# A stack is orphaned when its objects exist and no run.sh is alive. The stack's
# own containers say nothing either way: they are not children of the job and
# keep running after it dies, which is the reason this exists.
set -uo pipefail

STAMP="${STAMP:-/run/nightly-stack-orphaned}"
SWEEP="${SWEEP:-/usr/local/sbin/sweep-nightly-stacks.sh}"
# NOTE: longer than the gap between run.sh exiting and the workflow's own
# teardown finishing (save results, upload, sweep). Shorter, and this removes
# containers from under that teardown, whose `docker rm` then fails the step
# on a run that was otherwise fine.
GRACE_SECONDS=600

stack_exists() {
  # NOTE: not grep -q. It exits at the first match, docker dies of SIGPIPE, and
  # pipefail then reports a host with a stack on it as a host with none.
  { docker ps -a --format '{{.Names}}'; docker images --format '{{.Repository}}'; } \
    | grep '^mem0-nightly-' > /dev/null
}

# The runner containers share the host's process table, so the job's run.sh is
# visible here by its command line.
if pgrep -f 'tests/quality/nightly/run\.sh' > /dev/null || ! stack_exists; then
  rm -f "$STAMP"
  exit 0
fi

if [ ! -e "$STAMP" ]; then
  date +%s > "$STAMP"
  exit 0
fi

orphaned_for=$(( $(date +%s) - $(cat "$STAMP") ))
[ "$orphaned_for" -ge "$GRACE_SECONDS" ] || exit 0

echo "mem0 nightly stack with no run.sh for ${orphaned_for}s, sweeping"
rm -f "$STAMP"
exec bash "$SWEEP"
