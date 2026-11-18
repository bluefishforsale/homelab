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
# A stack is orphaned when its objects exist and nothing is driving them. The
# stack's own containers say nothing either way: they are not children of the
# job and keep running after it dies, which is the reason this exists.
set -uo pipefail

STAMP="${STAMP:-/run/nightly-stack-orphaned}"
SWEEP="${SWEEP:-/usr/local/sbin/sweep-nightly-stacks.sh}"
# NOTE: longer than the gap between run.sh exiting and the workflow's own
# teardown finishing (save results, upload, sweep). Shorter, and this removes
# containers from under that teardown, whose `docker rm` then fails the step
# on a run that was otherwise fine.
GRACE_SECONDS=600

# The host's process table includes the runner containers' processes, so the
# job is visible here by command line: run.sh by its path in the checkout, and
# every compose call it makes by `-p mem0-nightly-<id>`. The second half is what
# covers run.sh started by hand from its own directory.
pgrep -f 'tests/quality/nightly/run\.sh|mem0-nightly-' > /dev/null
case $? in
  0) rm -f "$STAMP"; exit 0 ;;
  1) ;;
  # NOTE: a pgrep that could not look is not a job that is gone. Read as "no
  # match", it sweeps a benchmark that is 14 hours into its run.
  *) echo "pgrep failed, cannot tell whether a nightly run is alive" >&2; exit 1 ;;
esac

list_objects() {
  docker ps -a --format '{{.Names}}' \
    && docker network ls --format '{{.Name}}' \
    && docker volume ls --format '{{.Name}}' \
    && docker images --format '{{.Repository}}'
}
# A docker that cannot list is not a host with nothing on it.
objects=$(list_objects) || { echo "docker could not list, cannot tell whether a stack is left" >&2; exit 1; }

if ! grep -q '^mem0-nightly-' <<< "$objects"; then
  rm -f "$STAMP"
  exit 0
fi

first_seen=$(cat "$STAMP" 2> /dev/null)
if ! [[ "$first_seen" =~ ^[0-9]+$ ]]; then
  date +%s > "$STAMP"
  exit 0
fi

orphaned_for=$(( $(date +%s) - first_seen ))
[ "$orphaned_for" -ge "$GRACE_SECONDS" ] || exit 0

# The stamp stays. After a clean sweep the next tick finds no stack and clears
# it. After a failed one every tick retries and fails, so the unit stays failed
# long enough for SystemdUnitFailed (for: 5m) to see it.
echo "mem0 nightly stack with nothing driving it for ${orphaned_for}s, sweeping"
exec bash "$SWEEP"
