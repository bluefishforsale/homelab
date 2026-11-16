#!/usr/bin/env bash
# Removes every mem0 nightly stack on this docker host: containers, networks,
# volumes and the images each run builds.
#
# run.sh in the mem0 checkout tears its own stack down from an EXIT trap, and
# that is the whole problem: a job that hits its timeout, is cancelled, or loses
# its host to a reboot never runs the trap. The containers are not children of
# the job, so they outlive it. Three timeouts and a reboot left four stacks on
# gh-runner-01 by 2026-11-15, about 2GB of images each plus an ollama layer,
# and the root disk reached 91%.
#
# By name prefix rather than by compose project. `compose down` leaves the
# bench container behind because it is a `compose run` one-off, and a project
# whose containers are already gone still owns images and a network.
#
# NOTE: this removes EVERY nightly stack, not just the calling run's. That is
# only safe because the workflow's concurrency group allows one run at a time.
# Drop that group and a second run's sweep takes the first run's stack with it.
set -uo pipefail

prefix='^mem0-nightly-'
left=0

# NOTE: the caller sets `left`, not this function. It runs on the right of a
# pipe, which is a subshell, so an assignment in here would be lost and a failed
# removal would report a clean sweep.
sweep() { # sweep <remove command...>, names on stdin
  local names
  names=$(grep -- "$prefix" || true)
  [ -n "$names" ] || return 0
  # shellcheck disable=SC2086  # one name per word is the point
  "$@" $names > /dev/null
}

# -v takes the anonymous volumes with it, which is where ollama keeps its model.
docker ps -a --format '{{.Names}}' | sweep docker rm -f -v || left=1
docker network ls --format '{{.Name}}' | sweep docker network rm || left=1
docker volume ls --format '{{.Name}}' | sweep docker volume rm || left=1
docker images --format '{{.Repository}}:{{.Tag}}' | sweep docker rmi || left=1

exit "$left"
