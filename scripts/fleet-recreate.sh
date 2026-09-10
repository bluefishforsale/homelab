#!/usr/bin/env bash
# fleet-recreate.sh - bounce every container on a host so it picks up the host's current
# /etc/resolv.conf. ONE host at a time, health-gated between hosts.
#
# WHY: docker generates each container's /etc/resolv.conf at
# /var/lib/docker/containers/<id>/resolv.conf from the HOST's file and bind-mounts it in.
# Change the host's resolvers and every already-running container keeps answering from
# the old list. Measured on ocean 2026-09-09: the host had a single
# `nameserver 192.168.1.2`, and so did prometheus and grafana. Moving the fleet from one
# internal resolver to both (.2 and .3) is therefore not finished when the playbook lands.
# It is finished when every container has been bounced.
#
# RESTART IS ENOUGH, and that is why this does not use `--force-recreate`. Docker writes
# that file while building the network sandbox, which happens on every container START,
# not only at create. Verified on ocean: my_ta_jose was created 2026-07-20T19:13:28Z and
# last started 2026-08-19T22:24:00Z, and its resolv.conf mtime is 2026-08-19 15:24:01
# -0700, the start and not the create. Matches moby, where Sandbox.setupDNS() rewrites
# the file unconditionally from the host's and the sandbox is rebuilt per start.
#
# NOTE: `compose up -d --force-recreate` would also do it, and is the WRONG tool here. It
# re-renders the compose file, so a project whose variables resolve blank comes back
# gutted. That is how four github-runners came up with an empty ACCESS_TOKEN and took CI
# out for two days on 2026-09-06 (see files/node-exporter/docker-refresh.sh, which now
# refuses for that reason). A restart reads no definition, so that class of failure cannot
# happen, and no image moves either, which keeps a DNS migration a DNS migration.
#
# HOST ORDER (the default sweep, and why): observability is the thing you cannot lose
# mid-run, so the host carrying it goes LAST. ocean runs Prometheus, Alertmanager,
# Grafana and Loki, so bouncing them blinds the health gate for every host after it, and
# it is also the largest blast radius (33 compose projects, Plex, the GPU inference
# server). The rest are ordered by how little depends on them: registry-cache-01 (image
# pulls only) -> agentbox -> pihole -> node005 -> node006 (a hypervisor's containers are
# just exporters; the VMs on it are untouched) -> ocean. Expect the gate AFTER ocean to
# come back unverified, because the monitoring stack itself just restarted; ocean is last
# precisely so a blind gate blocks nothing.
#
# DEFAULT-SKIPPED (naming a host explicitly overrides this):
#   dns01, dns02   they ARE the resolvers. Bouncing dns-stack while the fleet still
#                  depends on that node is the dns01 outage again, and both already point
#                  at two nameservers, so they are not what this exists to fix.
#   gh-runner-01   its containers are the CI runner that executes deploys; a restart
#                  kills whatever job is in flight. Run it by hand when CI is idle.
#
# Usage:
#   fleet-recreate.sh plan  [host ...]        what WOULD be bounced (read-only)
#   fleet-recreate.sh apply [-y] [host ...]   bounce it, host by host, gated
#
# With no hosts, both walk the default order. Naming hosts overrides the order AND the
# default skip list: that is the operator taking responsibility for a skipped host.
#
# Env: FLEET_RECREATE_RESOLVERS (192.168.1.2,192.168.1.3) resolvers a host and its
#        containers must carry; a container whose resolv.conf is missing any is stale,
#        and a host missing any is not migrated yet and is skipped untouched.
#      FLEET_RECREATE_PROBE (ocean.home)  name every resolver must answer.
#      FLEET_RECREATE_SKIP                extra hosts to skip, comma-separated.
#      FLEET_RECREATE_SETTLE (90)         seconds homelab-health.sh settles before its
#        per-host verdict. Above its own 45s default because this bounces the host's
#        node-exporter and cadvisor too, so those targets blip by definition.
#      FLEET_RECREATE_TIMEOUT (900)       budget for one host's restart loop.
#      HOMELAB_INVENTORY, SSH_TIMEOUT (lib/inventory.sh); HOMELAB_PROM,
#      HOMELAB_ALERTMANAGER (homelab-health.sh).
#
# Exit: 0 every selected host bounced and healthy · 1 halted, remaining hosts untouched
set -uo pipefail
. "$(dirname "$0")/lib/inventory.sh"

HEALTH="$(dirname "$0")/homelab-health.sh"
PROBE="${FLEET_RECREATE_PROBE:-ocean.home}"
SETTLE="${FLEET_RECREATE_SETTLE:-90}"
RUN_TIMEOUT="${FLEET_RECREATE_TIMEOUT:-900}"
DEFAULT_ORDER="registry-cache-01 agentbox pihole node005 node006 ocean"
DEFAULT_SKIP="dns01 dns02 gh-runner-01"
IFS=, read -ra RESOLVERS <<<"${FLEET_RECREATE_RESOLVERS:-192.168.1.2,192.168.1.3}"

die() { printf '%s\n' "$*" >&2; exit 1; }
# Bounded to the header block (Usage: through the first non-comment line, then drop it)
# rather than every '^#' line in the file the way the sibling scripts do: several of the
# comments below start at column 0, and those would otherwise land in --help.
usage() { sed -n '/^# Usage:/,/^[^#]/p' "$0" | sed '$d; s/^# \{0,1\}//'; exit "${1:-0}"; }

confirm() {  # $1 = prompt; auto-yes if $YES
  [ "${YES:-0}" = "1" ] && return 0
  local ans
  printf '%s [y/N] ' "$1" >&2
  read -r ans </dev/tty || return 1
  [ "$ans" = "y" ] || [ "$ans" = "Y" ]
}

# lib/inventory.sh's on_host is bounded at SSH_TIMEOUT*3 (15s by default), which a host's
# restart loop blows straight through. Same options, caller-chosen budget, and stderr is
# kept so a sudo or docker failure is readable instead of silently empty.
on_host_t() {
  local t=$1 dest=$2; shift 2
  timeout "$t" \
    ssh -n -o ConnectTimeout="$SSH_TIMEOUT" -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
    "$dest" "$@"
}

# Remote predicate: does the resolv.conf at $1 name every required resolver.
#
# NOTE: -w is load-bearing. A plain match for 192.168.1.2 also hits 192.168.1.20
# (gh-runner-01), which would read a stale container as already current, the one failure
# mode of this script that leaves containers on the old resolver while reporting done.
# Trailing newline: the remote shell needs a terminator between `}` and the call that
# every user of this appends, and ocean/node00[56] log in to zsh, not bash.
CARRIES="carries() { $(for r in "${RESOLVERS[@]}"; do printf 'sudo -n grep -qwF %s "$1" && ' "$r"; done)true; }"$'\n'

# "state<TAB>project<TAB>container" per running container, oldest first.
#
# Enumerated from `docker ps`, not `docker compose ls`: node005 runs ipmi-exporter,
# pve-exporter and process-exporter as bare containers, so a compose-project walk would
# silently leave them on the old resolver. A container with no compose project shows "-".
#
# Oldest-first is a cheap stand-in for dependency order, since compose creates a
# dependency before the service that declares it, so postgres is back before its app.
#
# NOTE: `set -o pipefail` remotely is what makes a failed enumeration FAIL. Without it a
# sick docker daemon yields an empty `docker ps`, the loop runs zero times, the pipeline
# exits 0, and the host reads as "no containers, nothing to do": a silent skip that leaves
# every container on the old resolver while the run reports success. The host shells are a
# mix of bash and zsh (ocean and node00[56] are zsh); both honour it, and the xargs is
# there because unquoted word splitting is bash-only.
list_containers() {  # $1 = user@ip
  on_host_t 60 "$1" "$CARRIES"'
    set -o pipefail
    sudo -n docker ps -q | xargs -r sudo -n docker inspect \
      --format "{{.Created}} {{.Name}} {{.ResolvConfPath}} {{index .Config.Labels \"com.docker.compose.project\"}}" \
      | sort | while read -r _ n p prj; do
        carries "$p" && s=ok || s=stale
        printf "%s\t%s\t%s\n" "$s" "${prj:--}" "${n#/}"
      done'
}

# Resolvers that do NOT answer $PROBE from this host. Empty output is the pass.
unanswered() {  # $1 = user@ip
  on_host_t 30 "$1" "for r in ${RESOLVERS[*]}; do"'
    a=$(dig +short +time=2 +tries=1 "@$r" '"$PROBE"' A 2>/dev/null | head -1)
    [ -n "$a" ] || echo "$r"
  done'
}

# NOTE: -t 30, not the default 10. Plex takes ~12s to shut down cleanly and the SIGKILL
# at 10s is what deadlocks its SQLite (plex-autoheal.timer exists because of that).
# One container per iteration so a failure names the container instead of the host.
#
# The remote side carries the verdict in its exit status so the caller can let this stream
# live rather than capturing it: ocean's 60 containers take minutes, and an operator
# watching a destructive sweep should see each name as it goes, not a block at the end.
bounce() {  # $1 = user@ip, $2 = space-separated container names
  on_host_t "$RUN_TIMEOUT" "$1" "rc=0; for c in $2; do"'
    printf "  restart %-32s " "$c"
    if sudo -n docker restart -t 30 "$c" >/dev/null 2>&1; then echo ok; else echo FAILED; rc=1; fi
  done; exit $rc'
}

cmd="${1:-}"; [ $# -gt 0 ] && shift
case "$cmd" in
  plan|apply) ;;
  -h|--help|help) usage 0 ;;
  *) usage 1 ;;
esac

YES=0; hosts=()
for a in "$@"; do
  case "$a" in
    -y) YES=1 ;;
    -*) usage 1 ;;
    *) hosts+=("$a") ;;
  esac
done
explicit=1
if [ "${#hosts[@]}" -eq 0 ]; then explicit=0; read -ra hosts <<<"$DEFAULT_ORDER"; fi

extra_skip="${FLEET_RECREATE_SKIP:-}"
skip=" ${extra_skip//,/ } "
[ "$explicit" -eq 0 ] && skip="$skip$DEFAULT_SKIP "

# Resolve every host up front: a typo should cost nothing, not surface after three hosts
# have already been bounced.
declare -a dests=()
for h in "${hosts[@]}"; do
  d=$(resolve_host "$h") || die "cannot read inventory"
  [ -n "$d" ] || die "host not in inventory: $h"
  dests+=("$d")
done

echo "resolvers: ${RESOLVERS[*]}   probe: $PROBE"
echo "hosts:     ${hosts[*]}"
[ "$explicit" -eq 0 ] && echo "skipping:  $DEFAULT_SKIP (default; name a host explicitly to include it)"

base=""
if [ "$cmd" = apply ]; then
  confirm "bounce EVERY stale container on ${#hosts[@]} host(s), one host at a time?" || die "aborted"
  base=$(mktemp -t fleet-recreate) || die "mktemp failed"
  trap 'rm -f "$base"' EXIT
  # Refuse to start blind: if the fleet's own monitoring is unreachable there is nothing
  # to gate on, and an ungated sweep across six hosts is how one bad host becomes six.
  "$HEALTH" snapshot "$base" || die "no health baseline, refusing to bounce anything unobserved"
fi

for i in "${!hosts[@]}"; do
  h=${hosts[$i]} dest=${dests[$i]}
  printf '\n── %s ──\n' "$h"
  case "$skip" in *" $h "*) echo "  skipped (see the script header for why)"; continue ;; esac

  if ! on_host_t 15 "$dest" "$CARRIES"' carries /etc/resolv.conf'; then
    echo "  /etc/resolv.conf does not name all of ${RESOLVERS[*]}, not migrated yet, skipping"
    continue
  fi

  rows=$(list_containers "$dest") || die "  enumeration failed on $h, HALT"
  total=$(printf '%s' "$rows" | grep -c . || true)
  stale=$(printf '%s\n' "$rows" | awk -F'\t' '$1=="stale"{print $3}')
  nstale=$(printf '%s' "$stale" | grep -c . || true)
  echo "  $total running, $nstale stale"
  [ "$total" -gt 0 ] || { echo "  no containers, nothing to do"; continue; }

  if [ "$nstale" -eq 0 ]; then echo "  already current, nothing to do"; continue; fi
  printf '%s\n' "$rows" | awk -F'\t' '$1=="stale"{printf "    %-22s %s\n", $2, $3}'
  [ "$cmd" = plan ] && continue

  bounce "$dest" "$(printf '%s' "$stale" | paste -sd' ' -)" \
    || die "  a restart failed on $h, HALT, remaining hosts untouched"

  # The resolv.conf write lands a fraction of a second after `docker restart` returns,
  # so re-reading immediately would report a container it just fixed as still stale.
  sleep 5
  rows=$(list_containers "$dest") || die "  re-enumeration failed on $h, HALT"
  left=$(printf '%s\n' "$rows" | awk -F'\t' '$1=="stale"' | grep -c . || true)
  now=$(printf '%s' "$rows" | grep -c . || true)
  [ "$left" -eq 0 ] || die "  $left container(s) still stale on $h, HALT"
  # A container that died during its restart drops out of `docker ps` and so also drops
  # out of the stale count above; only the total catches it.
  [ "$now" -ge "$total" ] || die "  $((total - now)) container(s) did not come back on $h, HALT"
  echo "  $now running, 0 stale"

  bad=$(unanswered "$dest")
  [ -z "$bad" ] || die "  $h cannot resolve '$PROBE' via:$(printf ' %s' "$bad"), HALT"
  echo "  every resolver answers '$PROBE'"

  "$HEALTH" verify "$base" "$SETTLE" || die "  fleet health gate failed after $h, HALT, remaining hosts untouched"
done

echo
echo "done: ${hosts[*]}"
