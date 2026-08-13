#!/usr/bin/env bash
# inventory.sh — shared fleet host resolution + bounded SSH for the homelab admin scripts.
# Source it: . "$(dirname "$0")/lib/inventory.sh"
#
# Provides:
#   fleet_hosts            -> "name<TAB>user@ip" per inventory host (deduped)
#   resolve_host <name>    -> "user@ip" for one host, empty if unknown
#   on_host <user@ip> <cmd...>  -> run cmd over ssh, hard-bounded, stdin from /dev/null
#
# Env: HOMELAB_INVENTORY (default inventories/production/hosts.ini), SSH_TIMEOUT (default 5)

_INV_DEFAULT="$(dirname "${BASH_SOURCE[0]}")/../../inventories/production/hosts.ini"
INV="${HOMELAB_INVENTORY:-$_INV_DEFAULT}"
SSH_TIMEOUT="${SSH_TIMEOUT:-5}"

fleet_hosts() {
  [ -f "$INV" ] || { printf 'inventory not found: %s\n' "$INV" >&2; return 1; }
  awk '
    /^[[:space:]]*#/ { next }                 # skip commented-out host lines
    /ansible_ssh_host=/ {
      name=$1; user=""; ip=""
      for (i=1;i<=NF;i++) {
        if ($i ~ /^ansible_user=/)     { split($i,a,"="); user=a[2] }
        if ($i ~ /^ansible_ssh_host=/) { split($i,a,"="); ip=a[2] }
      }
      if (ip != "" && !(name in seen)) { seen[name]=1; printf "%s\t%s@%s\n", name, user, ip }
    }' "$INV"
}

resolve_host() { fleet_hosts | awk -v n="$1" -F'\t' '$1==n {print $2}'; }

# -n keeps ssh from swallowing a caller's `while read` stdin; timeout bounds a wedged host.
on_host() {
  local dest=$1; shift
  timeout "$((SSH_TIMEOUT * 3))" \
    ssh -n -o ConnectTimeout="$SSH_TIMEOUT" -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
    "$dest" "$@" 2>/dev/null
}
