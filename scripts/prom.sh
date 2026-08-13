#!/usr/bin/env bash
# prom.sh — thin Prometheus query helper for the homelab.
#
# Stops every ad-hoc `curl .../api/v1/query --data-urlencode ... | jq` from being
# rewritten by hand. Talks to the ocean Prometheus by default; override with
# HOMELAB_PROM.
#
# Usage:
#   prom.sh q '<promql>'                 instant query -> "value  {labels}" per series, desc
#   prom.sh raw '<promql>'               instant query -> raw JSON (pipe to jq)
#   prom.sh names [regex]                list metric names (__name__), optional grep filter
#   prom.sh labels <metric>              show the distinct label sets for a metric
#   prom.sh targets [up|down]            scrape targets, optionally filtered by health
#
# For time ranges, use a range-vector inside an instant query (no range subcommand):
#   prom.sh q 'max_over_time((rate(foo[5m]))[24h:10m])'
#
# Env: HOMELAB_PROM (default http://192.168.1.143:9090), HOMELAB_PROM_TIMEOUT (default 5)
set -uo pipefail

PROM="${HOMELAB_PROM:-http://192.168.1.143:9090}"
TIMEOUT="${HOMELAB_PROM_TIMEOUT:-5}"

die() { printf '%s\n' "$*" >&2; exit 1; }
usage() { grep -E '^#' "$0" | sed -n '/Usage:/,$p' | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

# raw instant query -> JSON on stdout; nonzero + message on API/transport failure.
_query() {
  local out
  out=$(curl -fsS -m "$TIMEOUT" -G "$PROM/api/v1/query" --data-urlencode "query=$1" 2>/dev/null) \
    || die "prom: query failed (is $PROM reachable?)"
  printf '%s' "$out" | jq -e '.status=="success"' >/dev/null 2>&1 \
    || die "prom: query error: $(printf '%s' "$out" | jq -r '.error // "unknown"')"
  printf '%s' "$out"
}

[ $# -ge 1 ] || usage 1
cmd=$1; shift

case "$cmd" in
  q)
    [ $# -ge 1 ] || die "usage: prom.sh q '<promql>'"
    # "value  {label=val,...}" per series, numeric-desc; __name__ dropped from labels.
    _query "$1" | jq -r '
      .data.result[]
      | (.value[1]) as $v
      | (.metric | del(.__name__) | to_entries | map("\(.key)=\(.value)") | join(",")) as $l
      | "\($v)\t{\($l)}"' \
    | sort -t$'\t' -k1 -gr ;;
  raw)
    [ $# -ge 1 ] || die "usage: prom.sh raw '<promql>'"
    _query "$1" ;;
  names)
    out=$(curl -fsS -m "$TIMEOUT" "$PROM/api/v1/label/__name__/values" 2>/dev/null) \
      || die "prom: names failed"
    printf '%s' "$out" | jq -r '.data[]' | { [ $# -ge 1 ] && grep -iE "$1" || cat; } ;;
  labels)
    [ $# -ge 1 ] || die "usage: prom.sh labels <metric>"
    _query "$1" | jq -r '.data.result[].metric | del(.__name__) | to_entries
      | map("\(.key)=\(.value)") | join(",")' | sort -u ;;
  targets)
    out=$(curl -fsS -m "$TIMEOUT" "$PROM/api/v1/targets?state=active" 2>/dev/null) \
      || die "prom: targets failed"
    printf '%s' "$out" | jq -r --arg f "${1:-}" '
      .data.activeTargets[]
      | select($f=="" or .health==$f)
      | "\(.health)\t\(.labels.job)\t\(.scrapeUrl)"' | sort ;;
  -h|--help|help) usage 0 ;;
  *) die "unknown command: $cmd (try: prom.sh --help)" ;;
esac
