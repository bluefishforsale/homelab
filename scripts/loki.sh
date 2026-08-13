#!/usr/bin/env bash
# loki.sh — pull logs for a service on a host from the homelab Loki.
#
# A "service" is matched as a docker container name first, then falls back to a
# systemd unit — so it works whether the thing runs in compose (container=) or as a
# unit (unit=). Force one with -c/--container or -u/--unit.
#
# Usage:
#   loki.sh <service> <host> [since] [limit] [match]
#     since   time window back from now (30m, 2h, 1d, or seconds). default 1h
#     limit   max lines. default 300
#     match   optional substring line-filter (LogQL |= "…")
#   loki.sh -u <unit> <host> [since] [limit] [match]      force systemd unit
#   loki.sh -c <container> <host> [since] [limit] [match]  force docker container
#
# Examples:
#   loki.sh blog_saetnere_com_wp ocean 2h
#   loki.sh -u wordpress ocean 1d 500 error
#
# Env: HOMELAB_LOKI (default http://192.168.1.143:3100), HOMELAB_LOKI_TIMEOUT (default 8)
set -uo pipefail

LOKI="${HOMELAB_LOKI:-http://192.168.1.143:3100}"
TIMEOUT="${HOMELAB_LOKI_TIMEOUT:-8}"

die() { printf '%s\n' "$*" >&2; exit 1; }
usage() { grep -E '^#' "$0" | sed -n '/Usage:/,$p' | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

label=""            # forced label (container|unit), empty = auto
case "${1:-}" in
  -h|--help|help) usage 0 ;;
  -c|--container) label="container"; shift ;;
  -u|--unit)      label="unit";      shift ;;
esac

svc="${1:-}"; host="${2:-}"
[ -n "$svc" ] && [ -n "$host" ] || usage 1
since="${3:-1h}"; limit="${4:-300}"; match="${5:-}"

# duration -> seconds (s/m/h/d suffix, or bare seconds)
dur_s() {
  case "$1" in
    *s) echo "${1%s}" ;;
    *m) echo "$(( ${1%m} * 60 ))" ;;
    *h) echo "$(( ${1%h} * 3600 ))" ;;
    *d) echo "$(( ${1%d} * 86400 ))" ;;
    *)  echo "$1" ;;
  esac
}
now=$(date +%s)
start_ns=$(( (now - $(dur_s "$since")) * 1000000000 ))
end_ns=$(( now * 1000000000 ))

# Fetch for a given label; echoes raw JSON. Returns nonzero on transport error.
fetch() {
  local sel="{host=\"$host\", $1=~\"^$svc.*\"}"
  [ -n "$match" ] && sel="$sel |= \"$match\""
  curl -fsS -m "$TIMEOUT" -G "$LOKI/loki/api/v1/query_range" \
    --data-urlencode "query=$sel" \
    --data-urlencode "start=$start_ns" --data-urlencode "end=$end_ns" \
    --data-urlencode "limit=$limit" --data-urlencode "direction=backward" 2>/dev/null
}

# Count streams in a Loki response.
streams() { printf '%s' "$1" | jq -r '.data.result | length' 2>/dev/null; }

resp=""
if [ -n "$label" ]; then
  resp=$(fetch "$label") || die "loki: query failed (is $LOKI reachable?)"
else
  resp=$(fetch container) || die "loki: query failed (is $LOKI reachable?)"
  if [ "$(streams "$resp")" = "0" ]; then
    resp=$(fetch unit) || die "loki: query failed"
    label="unit"
  else
    label="container"
  fi
fi

[ "$(streams "$resp")" = "0" ] && die "no logs: no $([ -n "$label" ] && echo "$label" || echo container/unit) matching '$svc' on $host in the last $since"

# Flatten all streams' [ts_ns, line] pairs, sort ascending by time, print "MM-DD HH:MM:SS  line".
printf '%s' "$resp" | jq -r '
  [ .data.result[] | .values[] ] | sort_by(.[0])
  | .[] | "\((.[0]|tonumber/1e9|strflocaltime("%m-%d %H:%M:%S")))  \(.[1])"'
