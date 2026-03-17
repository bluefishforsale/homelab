#!/usr/bin/env bash
# Compare DNS zone data between dns01 and dns02
# Machine-readable JSON output for automation

set -euo pipefail

DNS01="192.168.1.2"
DNS02="192.168.1.3"
ZONE="${1:-home.}"

# Colors for terminal output (optional)
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get zone transfer from server
get_zone() {
    local server="$1"
    local zone="$2"
    dig @"${server}" "${zone}" AXFR +noall +answer +onesoa 2>/dev/null | grep -v '^;' | grep -v '^$' | sort
}

# Get SOA serial
get_soa() {
    local server="$1"
    local zone="$2"
    dig @"${server}" "${zone}" SOA +short | awk '{print $3}'
}

# Get record count
count_records() {
    echo "$1" | grep -v '^;' | wc -l | tr -d ' '
}

# Main comparison
echo "=== DNS Zone Comparison: ${ZONE} ===" >&2
echo "" >&2

# Get zone data
echo "Fetching zone from dns01 (${DNS01})..." >&2
ZONE1=$(get_zone "${DNS01}" "${ZONE}" 2>/dev/null || echo "")
ZONE1_COUNT=$(count_records "${ZONE1}")
ZONE1_SOA=$(get_soa "${DNS01}" "${ZONE}" 2>/dev/null || echo "0")

echo "Fetching zone from dns02 (${DNS02})..." >&2
ZONE2=$(get_zone "${DNS02}" "${ZONE}" 2>/dev/null || echo "")
ZONE2_COUNT=$(count_records "${ZONE2}")
ZONE2_SOA=$(get_soa "${DNS02}" "${ZONE}" 2>/dev/null || echo "0")

echo "" >&2

# Compare
if [ "${ZONE1}" = "${ZONE2}" ]; then
    STATUS="identical"
    EXIT_CODE=0
    echo -e "${GREEN}✓ Zones are identical${NC}" >&2
else
    STATUS="different"
    EXIT_CODE=1
    echo -e "${RED}✗ Zones differ${NC}" >&2
fi

# Find differences
ONLY_DNS01=$(comm -23 <(echo "${ZONE1}") <(echo "${ZONE2}") | grep -v '^$' || echo "")
ONLY_DNS02=$(comm -13 <(echo "${ZONE1}") <(echo "${ZONE2}") | grep -v '^$' || echo "")
ONLY_DNS01_COUNT=$(count_records "${ONLY_DNS01}")
ONLY_DNS02_COUNT=$(count_records "${ONLY_DNS02}")

# Output summary to stderr
echo "" >&2
echo "Summary:" >&2
echo "  dns01: ${ZONE1_COUNT} records, SOA serial ${ZONE1_SOA}" >&2
echo "  dns02: ${ZONE2_COUNT} records, SOA serial ${ZONE2_SOA}" >&2
echo "  Only in dns01: ${ONLY_DNS01_COUNT} records" >&2
echo "  Only in dns02: ${ONLY_DNS02_COUNT} records" >&2
echo "" >&2

# Machine-readable JSON output to stdout
DNS01_REACHABLE="false"
[ -n "${ZONE1}" ] && DNS01_REACHABLE="true"

DNS02_REACHABLE="false"
[ -n "${ZONE2}" ] && DNS02_REACHABLE="true"

# Generate JSON arrays for differences
ONLY_DNS01_JSON="[]"
if [ -n "${ONLY_DNS01}" ]; then
    ONLY_DNS01_JSON=$(echo "${ONLY_DNS01}" | jq -R -s -c 'split("\n") | map(select(length > 0))')
fi

ONLY_DNS02_JSON="[]"
if [ -n "${ONLY_DNS02}" ]; then
    ONLY_DNS02_JSON=$(echo "${ONLY_DNS02}" | jq -R -s -c 'split("\n") | map(select(length > 0))')
fi

cat <<EOF
{
  "zone": "${ZONE}",
  "status": "${STATUS}",
  "dns01": {
    "ip": "${DNS01}",
    "record_count": ${ZONE1_COUNT},
    "soa_serial": ${ZONE1_SOA},
    "reachable": ${DNS01_REACHABLE}
  },
  "dns02": {
    "ip": "${DNS02}",
    "record_count": ${ZONE2_COUNT},
    "soa_serial": ${ZONE2_SOA},
    "reachable": ${DNS02_REACHABLE}
  },
  "differences": {
    "only_in_dns01": ${ONLY_DNS01_COUNT},
    "only_in_dns02": ${ONLY_DNS02_COUNT},
    "records_only_in_dns01": ${ONLY_DNS01_JSON},
    "records_only_in_dns02": ${ONLY_DNS02_JSON}
  }
}
EOF

exit ${EXIT_CODE}
