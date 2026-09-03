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

# Compose file backing a running project, first entry only (compose returns
# them comma-separated). Deliberately NOT fault-tolerant: unparseable output
# has to fail loudly, because resolving to "no such project" would report a
# clean run having done nothing.
config_file() {
  docker compose ls --format json | python3 -c '
import json, sys
for p in json.load(sys.stdin):
    if p["Name"] == sys.argv[1]:
        print(p.get("ConfigFiles", "").split(",")[0].strip())
' "$PROJECT"
}

images() { docker compose -p "$PROJECT" -f "$CFG" images --quiet 2>/dev/null | sort -u; }

if ! CFG=$(config_file); then
  logger -t docker-refresh "project=$PROJECT enumeration failed"
  exit 1
fi

# Not running. Starting it here would resurrect something stopped on purpose,
# and a service that is down against its will is already alerted on by its own
# unit, so this is a skip and not a failure.
if [ -z "$CFG" ]; then
  logger -t docker-refresh "project=$PROJECT not running, skipped"
  exit 0
fi

before=$(images)
# --ignore-buildable: a few projects (cloudflare-exporter, ndt-speedtest-exporter)
# build their image on the host from repo source, so the tag exists in no
# registry and pulling it fails. Skipping them here is the difference between a
# weekly no-op and a weekly page. Their update path is a repo change, not a pull.
if ! timeout "$PULL_TIMEOUT" docker compose -p "$PROJECT" -f "$CFG" pull --quiet --ignore-buildable; then
  logger -t docker-refresh "project=$PROJECT pull failed or exceeded ${PULL_TIMEOUT}s, nothing recreated"
  exit 1
fi
docker compose -p "$PROJECT" -f "$CFG" up -d || exit 1
after=$(images)

# The hourly prune deletes the replaced image within the hour, so this journal
# line is the rollback trail: it names the digest to pin in the playbook and
# re-pull when an upgrade goes bad.
updated=$(comm -13 <(printf '%s\n' "$before") <(printf '%s\n' "$after") | grep -c . || true)
[ "$updated" -gt 0 ] && logger -t docker-refresh \
  "project=$PROJECT updated=$updated before=$(echo "$before" | tr '\n' ',') after=$(echo "$after" | tr '\n' ',')"

exit 0
