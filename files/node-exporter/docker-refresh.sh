#!/usr/bin/env bash
# docker-refresh.sh — refresh the images of ONE running compose project.
#
# Floating tags (:latest, :main, :stable) never update on their own: `compose
# up -d` fetches only when an image is missing locally, and `--quiet-pull` only
# silences a pull that was already going to happen. So a host keeps running
# whatever it first pulled, which is how prometheus sat on a June image while
# restarting weekly. `pull` then `up -d` is the whole mechanism: compose
# compares image IDs and leaves unchanged services alone, so nothing restarts
# unless its image actually moved.
#
# One systemd instance per project (docker-refresh@<project>.timer), staggered
# by RandomizedDelaySec. A project that fails or wedges is then its own failed
# unit, which the existing SystemdUnitFailed alert already covers, and it
# cannot stop the other projects from refreshing.
#
# Usage: docker-refresh.sh <compose-project>
set -uo pipefail

PROJECT="${1:?usage: docker-refresh.sh <compose-project>}"

# The pull is the slow half and the only one that can run long: ocean measures
# 650 Mbps down, so 45m covers a from-scratch multi-gigabyte project many times
# over, and anything past it is a throttling registry rather than a big image.
# Bounding it HERE rather than leaving it to the unit's TimeoutStartSec is the
# point: systemd killing the unit mid-`up -d` would land between stopping the
# old container and starting the new one, and leave the service down. A pull
# that runs out of budget instead fails before anything is recreated, and the
# layers it did fetch stay cached for the next run.
PULL_TIMEOUT=2700
# The recreate half. Pulls nothing (the pull above already did), so this is
# container stop/start time; 10m covers a project with many services. The unit's
# TimeoutStartSec has to stay above PULL_TIMEOUT + UP_TIMEOUT or systemd becomes
# the thing that kills a recreate midway.
UP_TIMEOUT=600

# Compose files backing a running project, one per line. Deliberately NOT
# fault-tolerant: unparseable output has to fail loudly, because resolving to
# "no such project" would report a clean run having done nothing.
#
# ALL of them, not just the first. Compose returns them comma-separated for a
# project built from a base plus overrides, and acting on the first alone would
# recreate the project from a partial definition: services only the override
# defines vanish, and settings it changes silently revert. There are no such
# projects on the fleet today, which is exactly why this has to be right before
# there are.
config_files() {
  docker compose ls --format json | python3 -c '
import json, sys
for p in json.load(sys.stdin):
    if p["Name"] == sys.argv[1]:
        for f in p.get("ConfigFiles", "").split(","):
            if f.strip():
                print(f.strip())
' "$PROJECT"
}

# stderr is captured rather than dropped: run non-root, or against a project
# whose .env is mode-restricted, this fails with "permission denied" and
# silently yields nothing, which makes the change trail below disappear without
# a word. Losing the trail is not worth failing the unit over, so it logs and
# carries on.
images() {
  local out
  if ! out=$(docker compose -p "$PROJECT" "${CFG_ARGS[@]}" images --quiet 2>&1); then
    logger -t docker-refresh \
      "project=$PROJECT listing images failed: $(printf '%s' "$out" | tr '\n' ' ' | cut -c1-200)"
    return 0
  fi
  printf '%s\n' "$out" | sort -u
}

# RepoDigests, not the image IDs `images --quiet` returns. Those are two
# different values: an image ID is local content addressing and cannot be
# pulled or pinned anywhere, and the hourly prune deletes the image within the
# hour, after which the ID names nothing that exists. A repo digest is what you
# actually paste into a playbook to go back. Falls back to the ID when an image
# has no digest, which is the case for anything built on the host.
digests() {
  local id d
  for id in $1; do
    # Falling back to the ID matters: inspect failing here would otherwise drop
    # the entry entirely and leave a shorter list that silently misaligns with
    # the other side of the before/after pair.
    d=$(docker image inspect "$id" \
      --format '{{if .RepoDigests}}{{index .RepoDigests 0}}{{else}}{{.Id}}{{end}}' 2>/dev/null)
    printf '%s\n' "${d:-$id}"
  done | paste -sd, -
}

if ! CFG_LIST=$(config_files); then
  logger -t docker-refresh "project=$PROJECT enumeration failed"
  exit 1
fi

# Not running. Starting it here would resurrect something stopped on purpose,
# and a service that is down against its will is already alerted on by its own
# unit, so this is a skip and not a failure.
if [ -z "$CFG_LIST" ]; then
  logger -t docker-refresh "project=$PROJECT not running, skipped"
  exit 0
fi

# One -f per compose file, in the order compose reported them, so overrides
# still layer the way they did when the project was created.
CFG_ARGS=()
while IFS= read -r f; do CFG_ARGS+=(-f "$f"); done <<< "$CFG_LIST"

# A compose file whose variables resolve blank would be recreated gutted. The
# github-runners token reaches its containers as ACCESS_TOKEN: "${GITHUB_TOKEN}",
# supplied only by the EnvironmentFile on github-docker-runners.service, which
# this unit does not have. Recreating from here produced four runners with an
# empty token that crash-looped 3067 times and took CI out for two days on
# 2026-09-06. Refuse rather than recreate.
#
# The playbook also excludes that project so no timer exists at all, but that
# exclusion is a hardcoded name matched against a project Docker derives from a
# directory in a different file. This is the backstop for when those drift
# apart. Guarding on the class (any unset variable) rather than the name means
# it keeps working for projects this repo does not own.
if docker compose -p "$PROJECT" "${CFG_ARGS[@]}" config 2>&1 >/dev/null | grep -q 'variable is not set'; then
  logger -t docker-refresh "project=$PROJECT has unset compose variables, refusing to recreate"
  exit 1
fi

before=$(images)
# --ignore-buildable: a few projects (cloudflare-exporter, ndt-speedtest-exporter)
# build their image on the host from repo source, so the tag exists in no
# registry and pulling it fails. Skipping them here is the difference between a
# weekly no-op and a weekly page. Their update path is a repo change, not a pull.
if ! timeout "$PULL_TIMEOUT" docker compose -p "$PROJECT" "${CFG_ARGS[@]}" pull --quiet --ignore-buildable; then
  logger -t docker-refresh "project=$PROJECT pull failed or exceeded ${PULL_TIMEOUT}s, nothing recreated"
  exit 1
fi
# Bounded for the same reason the pull is, and the comment above about systemd
# killing the unit mid-recreate applies here literally: leaving this unbounded
# just moved the hazard from "any slow pull" to "a recreate that outruns
# whatever is left of TimeoutStartSec", where the kill lands between stopping
# the old container and starting the new one. 10m is generous for a recreate
# that pulls nothing, and it fits inside the unit's budget alongside the pull.
if ! timeout "$UP_TIMEOUT" docker compose -p "$PROJECT" "${CFG_ARGS[@]}" up -d; then
  logger -t docker-refresh "project=$PROJECT up -d failed or exceeded ${UP_TIMEOUT}s"
  exit 1
fi
after=$(images)
updated=$(comm -13 <(printf '%s\n' "$before") <(printf '%s\n' "$after") | grep -c . || true)

# Nothing verified that the new image actually works. `up -d` returns as soon as
# compose has started the containers, so an image that starts and immediately
# dies exits 0 here and the timer stamps a clean run. 25 images on this fleet
# float, prometheus, alertmanager and node-exporter among them, so the service
# that would report the breakage is itself in the set being recreated. Waiting
# for the project to settle turns "the refresh broke it" into a failed unit that
# SystemdUnitFailed already carries.
#
# Only checks when something actually changed: an unchanged project was not
# recreated, so its state is whatever it already was and is not this run's
# business. Health "starting" is not a verdict yet, hence the poll.
settled() {
  docker compose -p "$PROJECT" "${CFG_ARGS[@]}" ps --format json 2>/dev/null | python3 -c '
import json, sys
raw = sys.stdin.read().strip()
if not raw:
    sys.exit(0)
try:
    rows = json.loads(raw)
except json.JSONDecodeError:
    rows = [json.loads(l) for l in raw.splitlines() if l.strip()]
if isinstance(rows, dict):
    rows = [rows]
bad = [r.get("Name", "?") for r in rows
       if r.get("State") != "running" or r.get("Health") == "unhealthy"]
pending = [r.get("Name", "?") for r in rows if r.get("Health") == "starting"]
if pending:
    sys.exit(3)
if bad:
    sys.stderr.write(",".join(bad))
    sys.exit(1)
'
}

if [ "$updated" -gt 0 ]; then
  deadline=$((SECONDS + 120))
  while :; do
    # Captured on its own line: after an `if` whose branch does not run, bash
    # resets $? to 0, so testing the status separately is the only way to tell
    # "still starting" (3) from "broken" (1).
    err=$(settled 2>&1)
    rc=$?
    [ "$rc" -eq 0 ] && break
    if [ "$rc" -ne 3 ] || [ "$SECONDS" -ge "$deadline" ]; then
      logger -t docker-refresh \
        "project=$PROJECT recreated but did not come back healthy: ${err:-still starting after 120s}"
      exit 1
    fi
    sleep 5
  done
fi

# The hourly prune deletes the replaced image within the hour, so this journal
# line is the rollback trail: it names the repo digest to pin in the playbook
# and re-pull when an upgrade goes bad.
[ "$updated" -gt 0 ] && logger -t docker-refresh \
  "project=$PROJECT updated=$updated before=$(digests "$before") after=$(digests "$after")"

exit 0
