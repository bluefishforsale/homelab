#!/usr/bin/env bash
# fleet-restart.sh — restart a systemd unit or docker container on ONE host.
#
# The only state-mutating helper in this set, so it is deliberately narrow: an explicit
# target and host, no wildcards, existence-checked, and a confirmation prompt unless -y.
#
# Usage:
#   fleet-restart.sh restart <unit> <host> [-y]      sudo systemctl restart <unit>
#   fleet-restart.sh container <name> <host> [-y]    docker restart <container> (in-place bounce)
#
# NOTE: homelab systemd units run `compose down -> pull -> up` on start, so
# `restart <unit>` RECREATES the container (picks up a new image / compose change). Use
# `container` for a fast in-place bounce that does NOT recreate.
#
# Env: HOMELAB_INVENTORY, SSH_TIMEOUT (see lib/inventory.sh)
set -uo pipefail
. "$(dirname "$0")/lib/inventory.sh"

die() { printf '%s\n' "$*" >&2; exit 1; }
usage() { grep -E '^#' "$0" | sed -n '/Usage:/,$p' | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

confirm() {  # $1 = prompt; auto-yes if $YES
  [ "${YES:-0}" = "1" ] && return 0
  local ans
  printf '%s [y/N] ' "$1" >&2
  read -r ans </dev/tty || return 1
  [ "$ans" = "y" ] || [ "$ans" = "Y" ]
}

[ $# -ge 3 ] || usage 1
mode=$1 target=$2 host=$3
YES=0; [ "${4:-}" = "-y" ] && YES=1
dest=$(resolve_host "$host"); [ -n "$dest" ] || die "host not in inventory: $host"

case "$mode" in
  restart)
    # Verify the unit exists (LoadState != not-found) before touching it.
    load=$(on_host "$dest" "systemctl show '$target' -p LoadState --value 2>/dev/null") \
      || die "unreachable: $host"
    [ "$load" = "not-found" ] || [ -z "$load" ] && die "no systemd unit '$target' on $host"
    echo "before: $(on_host "$dest" "systemctl is-active '$target'; systemctl show '$target' -p SubState --value" | paste -sd' ' -)"
    confirm "restart unit '$target' on $host (recreates the container)?" || die "aborted"
    on_host "$dest" "sudo -n systemctl restart '$target'" || die "restart failed (passwordless sudo? unit ok?)"
    sleep 3
    echo "after:  $(on_host "$dest" "systemctl is-active '$target'; systemctl show '$target' -p SubState --value" | paste -sd' ' -)" ;;
  container)
    cn=$(on_host "$dest" "docker ps -a --filter name=$target --format '{{.Names}}' | head -1") \
      || die "unreachable: $host"
    [ -n "$cn" ] || die "no container matching '$target' on $host"
    echo "before: $(on_host "$dest" "docker inspect $cn --format '{{.State.Status}} restarts={{.RestartCount}}'")"
    confirm "docker restart '$cn' on $host?" || die "aborted"
    on_host "$dest" "docker restart $cn" >/dev/null || die "docker restart failed"
    sleep 3
    echo "after:  $(on_host "$dest" "docker inspect $cn --format '{{.State.Status}}{{if .State.Health}} ({{.State.Health.Status}}){{end}}'")" ;;
  -h|--help|help) usage 0 ;;
  *) die "unknown mode: $mode (try: fleet-restart.sh --help)" ;;
esac
