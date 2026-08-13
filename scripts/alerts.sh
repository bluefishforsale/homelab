#!/usr/bin/env bash
# alerts.sh — homelab Alertmanager: active alerts and silences.
#
# Usage:
#   alerts.sh [active]        active alerts (default): severity, name, instance, age
#   alerts.sh all             include silenced/inhibited
#   alerts.sh silences        active silences: matcher, who, ends
#
# Env: HOMELAB_ALERTMANAGER (default http://192.168.1.143:9093), HOMELAB_AM_TIMEOUT (default 5)
set -uo pipefail

AM="${HOMELAB_ALERTMANAGER:-http://192.168.1.143:9093}"
TIMEOUT="${HOMELAB_AM_TIMEOUT:-5}"
die() { printf '%s\n' "$*" >&2; exit 1; }
usage() { grep -E '^#' "$0" | sed -n '/Usage:/,$p' | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

get() { curl -fsS -m "$TIMEOUT" "$AM$1" 2>/dev/null || die "alertmanager unreachable: $AM"; }

case "${1:-active}" in
  active|all)
    filt="?active=true&silenced=false&inhibited=false"
    [ "${1:-active}" = "all" ] && filt="?active=true&silenced=true&inhibited=true"
    get "/api/v2/alerts$filt" | jq -r '
      sort_by(.labels.severity, .labels.alertname)[]
      | "\(.labels.severity // "-")\t\(.labels.alertname)\t\(.labels.instance // "-")\t\(.status.state)\tsince \(.startsAt)"' \
    | awk -F'\t' 'BEGIN{n=0}
        {n++; printf "%-9s %-28s %-24s %-10s %s\n", $1,$2,$3,$4,$5}
        END{if(n==0)print "no alerts"; else printf "\n%d alert(s)\n", n}' ;;
  silences)
    get "/api/v2/silences" | jq -r '
      [.[] | select(.status.state=="active")] as $a
      | if ($a|length)==0 then "no active silences"
        else ($a[] | "\(.matchers|map("\(.name)\(if .isRegex then "=~" else "=" end)\(.value)")|join(","))\tby \(.createdBy)\tends \(.endsAt)")
        end' | sed 's/\t/  /g' ;;
  -h|--help|help) usage 0 ;;
  *) die "unknown command: $1 (try: alerts.sh --help)" ;;
esac
