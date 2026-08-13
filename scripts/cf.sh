#!/usr/bin/env bash
# cf.sh — Cloudflare zone helper for the homelab (analytics/DNS/cache one-offs).
#
# Creds come from the ansible vault at runtime (nothing secret committed). Zone may be a
# known domain, or a raw 32-hex zone id.
#
# Usage:
#   cf.sh zones                          list known zones + ids
#   cf.sh cache-status <url>             show cf-cache-status / content-type / HTTP for a URL
#   cf.sh dns <zone> [name]              list DNS records (optional name substring filter)
#   cf.sh dns-add <zone> <type> <name> <content> [ttl]   add a DNS record (ttl default 300)
#   cf.sh purge <zone> [url ...]         purge cache: everything, or just the given URLs
#
# Examples:
#   cf.sh cache-status https://blog.terrac.com/
#   cf.sh purge terrac.com https://blog.terrac.com/ https://blog.terrac.com/posts/
#
# Env: ANSIBLE_VAULT_PASS (default ~/.ansible_vault_pass), CF_TIMEOUT (default 10)
set -uo pipefail

TIMEOUT="${CF_TIMEOUT:-10}"
die() { printf '%s\n' "$*" >&2; exit 1; }
usage() { grep -E '^#' "$0" | sed -n '/Usage:/,$p' | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

# Known zones (public ids, mirror cf_zones in cloudflare_exporter.yaml).
declare -A ZONES=(
  [terrac.com]=f3be74ced16c63d68b522f569ecba43b
  [saetnere.com]=427cd2f89d3669d1704f61f3d37d4149
  [radiantatmospheres.com]=87401274f499c81d52527080d0588e7f
)

zone_id() {
  local z=$1
  [ -n "${ZONES[$z]:-}" ] && { echo "${ZONES[$z]}"; return; }
  [[ "$z" =~ ^[0-9a-f]{32}$ ]] && { echo "$z"; return; }
  die "unknown zone: $z (known: ${!ZONES[*]}, or a 32-hex id)"
}

# Emit "email\nkey" from the vault; cached in globals after first call.
_EMAIL=""; _KEY=""
creds() {
  [ -n "$_KEY" ] && return
  local pass="${ANSIBLE_VAULT_PASS:-$HOME/.ansible_vault_pass}"
  local secrets; secrets="$(dirname "$0")/../vault/secrets.yaml"
  local out
  out=$(python3 - "$pass" "$secrets" <<'PY'
import sys, subprocess, yaml
pw, path = sys.argv[1], sys.argv[2]
raw = subprocess.check_output(["ansible-vault","view","--vault-password-file",pw,path])
cf = yaml.safe_load(raw)["cloudflare"]
print(cf["api_email"]); print(cf["api_key"])
PY
) || die "could not read cloudflare creds from vault"
  _EMAIL=$(printf '%s\n' "$out" | sed -n 1p)
  _KEY=$(printf '%s\n' "$out" | sed -n 2p)
}

# cf_api METHOD PATH [json-body] -> response JSON; dies on API error.
cf_api() {
  creds
  local method=$1 path=$2 body=${3:-}
  local args=(-fsS -m "$TIMEOUT" -X "$method"
    -H "X-Auth-Email: $_EMAIL" -H "X-Auth-Key: $_KEY" -H "Content-Type: application/json")
  [ -n "$body" ] && args+=(--data "$body")
  local resp
  resp=$(curl "${args[@]}" "https://api.cloudflare.com/client/v4$path" 2>/dev/null) \
    || die "cf api $method $path failed"
  printf '%s' "$resp" | jq -e '.success' >/dev/null 2>&1 \
    || die "cf api error: $(printf '%s' "$resp" | jq -c '.errors')"
  printf '%s' "$resp"
}

case "${1:-}" in
  zones)
    for z in "${!ZONES[@]}"; do printf '%-28s %s\n' "$z" "${ZONES[$z]}"; done | sort ;;
  cache-status)
    url=${2:?usage: cf.sh cache-status <url>}
    curl -s -o /dev/null -D - "$url" \
      | grep -iE "^HTTP|^cf-cache-status|^content-type|^age|^cache-control" | tr -d '\r' ;;
  dns)
    z=$(zone_id "${2:?usage: cf.sh dns <zone> [name]}"); filt=${3:-}
    cf_api GET "/zones/$z/dns_records?per_page=100" \
      | jq -r '.result[] | "\(.type)\t\(.name)\t\(.content)\tproxied=\(.proxied)"' \
      | { [ -n "$filt" ] && grep -i "$filt" || cat; } \
      | awk -F'\t' '{printf "%-6s %-38s %-40s %s\n", $1,$2,$3,$4}' ;;
  dns-add)
    z=$(zone_id "${2:?usage: cf.sh dns-add <zone> <type> <name> <content> [ttl]}")
    typ=${3:?type}; name=${4:?name}; content=${5:?content}; ttl=${6:-300}
    body=$(jq -nc --arg t "$typ" --arg n "$name" --arg c "$content" --argjson ttl "$ttl" \
      '{type:$t,name:$n,content:$c,ttl:$ttl}')
    cf_api POST "/zones/$z/dns_records" "$body" \
      | jq -r '"added \(.result.type) \(.result.name) -> \(.result.content) (id \(.result.id))"' ;;
  purge)
    z=$(zone_id "${2:?usage: cf.sh purge <zone> [url ...]}"); shift 2
    if [ $# -eq 0 ]; then
      echo "purging EVERYTHING for zone $z ..."
      cf_api POST "/zones/$z/purge_cache" '{"purge_everything":true}' | jq -r '"purged: success=\(.success)"'
    else
      body=$(printf '%s\n' "$@" | jq -R . | jq -sc '{files:.}')
      cf_api POST "/zones/$z/purge_cache" "$body" | jq -r "\"purged $# url(s): success=\(.success)\""
    fi ;;
  -h|--help|help|"") usage 0 ;;
  *) die "unknown command: $1 (try: cf.sh --help)" ;;
esac
