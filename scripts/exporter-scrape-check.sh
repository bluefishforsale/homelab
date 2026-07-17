#!/usr/bin/env bash
# Exporter scrape health check — find Prometheus targets that are down and show
# WHY, by probing each failing endpoint directly. Distinguishes "port closed"
# (service gone) from "port open but scrape 500s" (auth/config), which are the
# two failure modes that hide behind a generic ServiceDown alert.
#
# Usage: ./scripts/exporter-scrape-check.sh [--prom http://192.168.1.143:9090]
# Exit:  0 all scrapes up, 1 one or more down, 2 Prometheus unreachable.

set -euo pipefail

PROM="http://192.168.1.143:9090"
while [[ $# -gt 0 ]]; do
  case $1 in
    --prom) PROM="$2"; shift 2 ;;
    -h|--help) grep -E '^# ' "$0" | sed 's/^# //'; exit 0 ;;
    *) echo "unknown option: $1"; exit 1 ;;
  esac
done

RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[0;33m'; BOLD=$'\033[1m'; NC=$'\033[0m'

# up==0 for real scrape jobs (blackbox probes are excluded from ServiceDown too).
resp="$(curl -s --max-time 10 --data-urlencode 'query=up{job!~"blackbox-.*"}==0' "${PROM}/api/v1/query" 2>/dev/null || true)"
if [[ -z "$resp" || "$resp" != *'"status":"success"'* ]]; then
  echo "${YELLOW}${BOLD}Prometheus unreachable at ${PROM}${NC}"; exit 2
fi

# each down target as "job<TAB>instance". Parse the JSON in python, not sed.
mapfile -t down < <(printf '%s' "$resp" | python3 -c '
import sys, json
for r in json.load(sys.stdin)["data"]["result"]:
    m = r["metric"]
    print(m.get("job","?") + "\t" + m.get("instance","?"))
')

echo
echo "${BOLD}Exporter scrape check — ${PROM}${NC}"
if [[ ${#down[@]} -eq 0 ]]; then
  echo "${GREEN}${BOLD}all scrape targets up${NC}"; exit 0
fi

for row in "${down[@]}"; do
  job="${row%%$'\t'*}"; inst="${row##*$'\t'}"
  # instance had its port dropped by relabel on most jobs; probe the raw address
  # only when it carries one, else just report the target is down.
  if [[ "$inst" == *:* ]]; then
    code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 8 "http://${inst}/metrics" 2>/dev/null || echo 000)"
    case "$code" in
      000) why="${RED}port closed / no route${NC} (service not running)";;
      200) why="${YELLOW}scrape 200 but Prometheus sees it down${NC} (timeout? relabel?)";;
      *)   why="${RED}HTTP ${code}${NC} (up but erroring — auth/config)";;
    esac
  else
    why="down (instance carries no port; check the exporter host)"
  fi
  printf "  ${RED}DOWN${NC}  %-26s %-22s %b\n" "$job" "$inst" "$why"
done
exit 1
