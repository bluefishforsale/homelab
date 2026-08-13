#!/usr/bin/env bash
# fleet-systemctl.sh — systemd status across the homelab fleet over SSH.
#
# Hosts come from the ansible inventory (inventories/production/hosts.ini), so this
# tracks the real fleet. Each host is reached as ansible_user@ansible_ssh_host (the
# inventory is authoritative; SSH host aliases/users are not).
#
# Usage:
#   fleet-systemctl.sh all                     one line per host: failed count + running count
#   fleet-systemctl.sh list [host]             list active service units (all hosts, or one)
#   fleet-systemctl.sh <service> [host]        status of a unit across the fleet (or one host)
#   fleet-systemctl.sh host <host>             detail for one host: failed units + timers
#
# <service> may omit the .service suffix. Exit 1 if any targeted host has a failed unit
# (so it is usable as a check).
#
# Env: HOMELAB_INVENTORY (default inventories/production/hosts.ini),
#      SSH_TIMEOUT (default 5), FLEET_HOSTS (space-separated override of the host list)
set -uo pipefail

INV="${HOMELAB_INVENTORY:-$(dirname "$0")/../inventories/production/hosts.ini}"
SSH_TIMEOUT="${SSH_TIMEOUT:-5}"

die() { printf '%s\n' "$*" >&2; exit 1; }
usage() { grep -E '^#' "$0" | sed -n '/Usage:/,$p' | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

# Emit "name<TAB>user@ip" per inventory host with an ansible_ssh_host, deduped.
hosts() {
  if [ -n "${FLEET_HOSTS:-}" ]; then
    # FLEET_HOSTS is names only; resolve each against the inventory.
    for h in $FLEET_HOSTS; do _inventory | awk -v n="$h" '$1==n'; done
    return
  fi
  _inventory
}
_inventory() {
  [ -f "$INV" ] || die "inventory not found: $INV"
  awk '
    /^[[:space:]]*#/ { next }               # skip commented-out host lines
    /ansible_ssh_host=/ {
      name=$1; user=""; ip=""
      for (i=1;i<=NF;i++) {
        if ($i ~ /^ansible_user=/)      { split($i,a,"="); user=a[2] }
        if ($i ~ /^ansible_ssh_host=/)  { split($i,a,"="); ip=a[2] }
      }
      if (ip != "" && !(name in seen)) { seen[name]=1; printf "%s\t%s@%s\n", name, user, ip }
    }' "$INV"
}

# Run a command on a host; prints nothing on connect failure but returns 1.
# Hard-bounded by `timeout` so a host that accepts the TCP connect but then hangs
# (auth stall, wedged sshd) can't stall the whole fleet sweep — SSH's own timeouts
# don't cover every hang.
on_host() {
  local dest=$1; shift
  # -n: read stdin from /dev/null so ssh can't swallow the caller's `while read` host list.
  timeout "$((SSH_TIMEOUT * 3))" \
    ssh -n -o ConnectTimeout="$SSH_TIMEOUT" -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
    "$dest" "$@" 2>/dev/null
}

cmd_all() {
  local rc=0
  while IFS=$'\t' read -r name dest; do
    local out failed running
    out=$(on_host "$dest" 'echo "$(systemctl list-units --type=service --state=failed --no-legend --plain 2>/dev/null | wc -l) $(systemctl list-units --type=service --state=running --no-legend --plain 2>/dev/null | wc -l)"') \
      || { printf '%-18s UNREACHABLE\n' "$name"; rc=1; continue; }
    failed=${out%% *}; running=${out##* }
    if [ "${failed:-0}" -gt 0 ]; then
      printf '%-18s %s failed, %s running\n' "$name" "$failed" "$running"
      on_host "$dest" 'systemctl list-units --type=service --state=failed --no-legend --plain' \
        | awk '{print "    ✗ "$1}'
      rc=1
    else
      printf '%-18s ok (%s running)\n' "$name" "$running"
    fi
  done < <(hosts)
  return $rc
}

cmd_list() {
  local only=${1:-}
  while IFS=$'\t' read -r name dest; do
    [ -n "$only" ] && [ "$name" != "$only" ] && continue
    printf '── %s ──\n' "$name"
    on_host "$dest" 'systemctl list-units --type=service --state=active --no-legend --plain' \
      | awk '{printf "  %-40s %s %s\n", $1, $3, $4}' || echo "  UNREACHABLE"
  done < <(hosts)
}

cmd_service() {
  local svc=$1 only=${2:-} rc=0 found=0
  case "$svc" in *.*) ;; *) svc="$svc.service" ;; esac
  while IFS=$'\t' read -r name dest; do
    [ -n "$only" ] && [ "$name" != "$only" ] && continue
    local state
    state=$(on_host "$dest" "systemctl show '$svc' -p LoadState,ActiveState,SubState,ActiveEnterTimestamp --value 2>/dev/null | paste -sd'|' -") || { continue; }
    # LoadState|ActiveState|SubState|Since
    local load active sub since
    IFS='|' read -r load active sub since <<<"$state"
    [ "$load" = "not-found" ] || [ -z "$load" ] && continue
    found=1
    printf '%-18s %-10s %-10s since %s\n' "$name" "$active" "$sub" "${since:-?}"
    [ "$active" = "failed" ] && rc=1
  done < <(hosts)
  [ "$found" -eq 0 ] && die "no host has a unit named $svc"
  return $rc
}

cmd_host() {
  local only=${1:?usage: fleet-systemctl.sh host <host>}
  local dest
  dest=$(hosts | awk -v n="$only" '$1==n {print $2}')
  [ -n "$dest" ] || die "host not in inventory: $only"
  printf '── %s (%s) ──\n' "$only" "$dest"
  echo "[failed units]"
  on_host "$dest" 'systemctl list-units --type=service --state=failed --no-legend --plain' \
    | awk 'NF{print "  ✗ "$1" "$3" "$4} END{if(NR==0)print "  (none)"}' || die "unreachable: $only"
  echo "[timers — next runs]"
  on_host "$dest" 'systemctl list-timers --no-legend --no-pager 2>/dev/null | head -12' \
    | awk 'NF{printf "  %s %s → %s\n", $1, $2, $NF}'
}

[ $# -ge 1 ] || usage 1
case "$1" in
  all)  cmd_all ;;
  list) cmd_list "${2:-}" ;;
  host) cmd_host "${2:-}" ;;
  -h|--help|help) usage 0 ;;
  *)    cmd_service "$1" "${2:-}" ;;
esac
