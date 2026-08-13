#!/usr/bin/env bash
# docker.sh — Docker container status across the homelab fleet over SSH (read-only).
#
# Hosts come from the ansible inventory. Reaches each as ansible_user@ansible_ssh_host.
#
# Usage:
#   docker.sh ps [host]                containers on host(s): name, status, health, restarts
#   docker.sh where <container>        which host(s) run a container matching the name
#   docker.sh status <container> [host] detail: state, health, restarts, image, started, mounts
#   docker.sh logs <container> [host] [n]  tail n log lines (default 60); host auto-found if omitted
#
# <container> is a name substring (matches like `docker ps --filter name=`).
# Env: HOMELAB_INVENTORY, SSH_TIMEOUT (see lib/inventory.sh)
set -uo pipefail
. "$(dirname "$0")/lib/inventory.sh"

die() { printf '%s\n' "$*" >&2; exit 1; }
usage() { grep -E '^#' "$0" | sed -n '/Usage:/,$p' | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

# Find "name<TAB>user@ip" host rows that actually run a container matching $1.
_hosts_running() {
  while IFS=$'\t' read -r name dest; do
    local hit
    hit=$(on_host "$dest" "docker ps -a --filter name=$1 --format '{{.Names}}'" ) || continue
    [ -n "$hit" ] && printf '%s\t%s\t%s\n' "$name" "$dest" "$(printf '%s' "$hit" | paste -sd, -)"
  done < <(fleet_hosts)
}

cmd_ps() {
  local only=${1:-}
  while IFS=$'\t' read -r name dest; do
    [ -n "$only" ] && [ "$name" != "$only" ] && continue
    printf '── %s ──\n' "$name"
    on_host "$dest" 'docker ps --format "{{.Names}}\t{{.Status}}" 2>/dev/null | sort' \
      | awk -F'\t' 'NF{printf "  %-32s %s\n", $1, $2} END{if(NR==0)print "  (none / unreachable)"}'
  done < <(fleet_hosts)
}

cmd_where() {
  local c=${1:?usage: docker.sh where <container>} found=0
  while IFS=$'\t' read -r name dest running; do
    found=1; printf '%-18s %s\n' "$name" "$running"
  done < <(_hosts_running "$c")
  [ "$found" -eq 0 ] && die "no host runs a container matching '$c'"
}

cmd_status() {
  local c=${1:?usage: docker.sh status <container> [host]} host=${2:-} dest
  if [ -n "$host" ]; then dest=$(resolve_host "$host"); [ -n "$dest" ] || die "host not in inventory: $host"
  else dest=$(_hosts_running "$c" | head -1 | cut -f2); [ -n "$dest" ] || die "no host runs '$c'"; fi
  # Resolve the exact container name (first match) on that host.
  local cn
  cn=$(on_host "$dest" "docker ps -a --filter name=$c --format '{{.Names}}' | head -1")
  [ -n "$cn" ] || die "no container matching '$c' on that host"
  echo "container: $cn  (host $host${host:+ }$dest)"
  on_host "$dest" "docker inspect $cn --format '  state:    {{.State.Status}}{{if .State.Health}} ({{.State.Health.Status}}){{end}}
  restarts: {{.RestartCount}}
  image:    {{.Config.Image}}
  started:  {{.State.StartedAt}}
  exit:     {{.State.ExitCode}}{{if .State.Error}}  err={{.State.Error}}{{end}}'"
  echo "  mounts:"
  on_host "$dest" "docker inspect $cn --format '{{range .Mounts}}    {{.Source}} -> {{.Destination}}{{println}}{{end}}'"
}

cmd_logs() {
  local c=${1:?usage: docker.sh logs <container> [host] [n]} host=${2:-} n=${3:-60} dest
  if [ -n "$host" ]; then dest=$(resolve_host "$host"); [ -n "$dest" ] || die "host not in inventory: $host"
  else dest=$(_hosts_running "$c" | head -1 | cut -f2); [ -n "$dest" ] || die "no host runs '$c'"; fi
  local cn
  cn=$(on_host "$dest" "docker ps -a --filter name=$c --format '{{.Names}}' | head -1")
  [ -n "$cn" ] || die "no container matching '$c' on that host"
  on_host "$dest" "docker logs --tail $n $cn 2>&1"
}

[ $# -ge 1 ] || usage 1
case "$1" in
  ps)     cmd_ps "${2:-}" ;;
  where)  cmd_where "${2:-}" ;;
  status) cmd_status "${2:-}" "${3:-}" ;;
  logs)   cmd_logs "${2:-}" "${3:-}" "${4:-}" ;;
  -h|--help|help) usage 0 ;;
  *) die "unknown command: $1 (try: docker.sh --help)" ;;
esac
