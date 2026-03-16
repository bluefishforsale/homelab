#!/usr/bin/env bash
# DNS Parity Check - Compare BIND (dns01) vs PowerDNS (dns02)
#
# Queries every known record against both DNS servers using dig,
# compares responses, and reports pass/fail with color-coded output.
#
# Usage:
#   ./scripts/dns-parity-check.sh [--old 192.168.1.2] [--new 192.168.1.3] [--verbose]
#
# Exit codes:
#   0 - All records match
#   1 - One or more records differ

set -euo pipefail

# Defaults
OLD_SERVER="192.168.1.2"
NEW_SERVER="192.168.1.3"
VERBOSE=false
PASS=0
FAIL=0
SKIP=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

usage() {
  echo "Usage: $0 [--old OLD_DNS_IP] [--new NEW_DNS_IP] [--verbose]"
  echo ""
  echo "  --old      Old DNS server IP (default: 192.168.1.2)"
  echo "  --new      New DNS server IP (default: 192.168.1.3)"
  echo "  --verbose  Show dig responses for each query"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case $1 in
    --old) OLD_SERVER="$2"; shift 2 ;;
    --new) NEW_SERVER="$2"; shift 2 ;;
    --verbose) VERBOSE=true; shift ;;
    -h|--help) usage ;;
    *) echo "Unknown option: $1"; usage ;;
  esac
done

# Verify dig is available
if ! command -v dig &>/dev/null; then
  echo "Error: dig not found. Install dnsutils/bind-utils."
  exit 1
fi

check_record() {
  local name="$1"
  local type="$2"
  local zone="${3:-}"
  local label="${name} ${type}"

  local old_result new_result
  old_result=$(dig +short +time=5 +tries=2 "@${OLD_SERVER}" "${name}" "${type}" 2>/dev/null | sort)
  new_result=$(dig +short +time=5 +tries=2 "@${NEW_SERVER}" "${name}" "${type}" 2>/dev/null | sort)

  if [[ -z "$old_result" && -z "$new_result" ]]; then
    printf "  ${YELLOW}⊘ SKIP${NC}  %-45s (both empty)\n" "$label"
    ((SKIP++))
    return
  fi

  if [[ "$old_result" == "$new_result" ]]; then
    printf "  ${GREEN}✅ PASS${NC}  %-45s %s\n" "$label" "$old_result"
    ((PASS++))
  else
    printf "  ${RED}❌ FAIL${NC}  %-45s\n" "$label"
    if [[ "$VERBOSE" == true ]] || true; then
      printf "           ${CYAN}old:${NC} %s\n" "${old_result:-<empty>}"
      printf "           ${CYAN}new:${NC} %s\n" "${new_result:-<empty>}"
    fi
    ((FAIL++))
  fi

  if [[ "$VERBOSE" == true ]]; then
    echo "           old raw: $(dig +short +time=5 +tries=2 "@${OLD_SERVER}" "${name}" "${type}" 2>/dev/null)"
    echo "           new raw: $(dig +short +time=5 +tries=2 "@${NEW_SERVER}" "${name}" "${type}" 2>/dev/null)"
  fi
}

check_soa() {
  local zone="$1"
  local label="SOA ${zone}"

  # Compare SOA fields except serial (field 3 in dig +short output)
  local old_result new_result
  old_result=$(dig +short +time=5 +tries=2 "@${OLD_SERVER}" "${zone}" SOA 2>/dev/null | awk '{$3="SERIAL"; print}')
  new_result=$(dig +short +time=5 +tries=2 "@${NEW_SERVER}" "${zone}" SOA 2>/dev/null | awk '{$3="SERIAL"; print}')

  if [[ -z "$old_result" && -z "$new_result" ]]; then
    printf "  ${YELLOW}⊘ SKIP${NC}  %-45s (both empty)\n" "$label"
    ((SKIP++))
    return
  fi

  if [[ "$old_result" == "$new_result" ]]; then
    printf "  ${GREEN}✅ PASS${NC}  %-45s (serial ignored)\n" "$label"
    ((PASS++))
  else
    printf "  ${RED}❌ FAIL${NC}  %-45s\n" "$label"
    printf "           ${CYAN}old:${NC} %s\n" "${old_result:-<empty>}"
    printf "           ${CYAN}new:${NC} %s\n" "${new_result:-<empty>}"
    ((FAIL++))
  fi
}

echo ""
echo -e "${BOLD}DNS Parity Check${NC}"
echo -e "  Old server (BIND):     ${OLD_SERVER}"
echo -e "  New server (PowerDNS): ${NEW_SERVER}"
echo ""

# ---------------------------------------------------------------------------
# Zone: home. — SOA and NS
# ---------------------------------------------------------------------------
echo -e "${BOLD}── Zone: home. (SOA/NS) ──${NC}"
check_soa "home."
check_record "home." NS

# ---------------------------------------------------------------------------
# Zone: home. — Forward A records
# ---------------------------------------------------------------------------
echo ""
echo -e "${BOLD}── Zone: home. (A records) ──${NC}"
check_record "gw.home." A
check_record "dns01.home." A
check_record "gitlab.home." A
check_record "pihole.home." A
check_record "unifi.home." A
check_record "apiserver.home." A
check_record "node005-idrac.home." A
check_record "ocean-idrac.home." A
check_record "gh-runner-01.home." A
check_record "gh-test-vm.home." A
check_record "openclaw.home." A
check_record "kube501.home." A
check_record "kube502.home." A
check_record "kube503.home." A
check_record "kube511.home." A
check_record "kube512.home." A
check_record "kube513.home." A
check_record "kube611.home." A
check_record "kube612.home." A
check_record "kube613.home." A
check_record "node005.home." A
check_record "node006.home." A
check_record "ocean-bond0.home." A
check_record "ocean-eth0.home." A
check_record "usg-3p.home." A
check_record "us-48-ac-pro.home." A
check_record "us-16-xg-10g.home." A
check_record "us-8-150w-poe.home." A
check_record "uap-ac-pro-home-1.home." A
check_record "uap-ac-pro-home-2.home." A
check_record "uap-ac-pro-garage-1.home." A

# ---------------------------------------------------------------------------
# Zone: home. — CNAME records
# ---------------------------------------------------------------------------
echo ""
echo -e "${BOLD}── Zone: home. (CNAME) ──${NC}"
check_record "ocean.home." CNAME

# ---------------------------------------------------------------------------
# Zone: home. — Wildcard
# ---------------------------------------------------------------------------
echo ""
echo -e "${BOLD}── Zone: home. (Wildcard) ──${NC}"
check_record "randomtest12345.home." A
check_record "anything.home." A

# ---------------------------------------------------------------------------
# Zone: cluster.local. — SOA/NS and wildcard
# ---------------------------------------------------------------------------
echo ""
echo -e "${BOLD}── Zone: cluster.local. ──${NC}"
check_soa "cluster.local."
check_record "cluster.local." NS
check_record "randomtest12345.cluster.local." A
check_record "any-service.cluster.local." A

# ---------------------------------------------------------------------------
# Zone: 1.168.192.in-addr.arpa — SOA/NS
# ---------------------------------------------------------------------------
echo ""
echo -e "${BOLD}── Zone: 1.168.192.in-addr.arpa (SOA/NS) ──${NC}"
check_soa "1.168.192.in-addr.arpa."
check_record "1.168.192.in-addr.arpa." NS

# ---------------------------------------------------------------------------
# Zone: 1.168.192.in-addr.arpa — PTR records
# ---------------------------------------------------------------------------
echo ""
echo -e "${BOLD}── Zone: 1.168.192.in-addr.arpa (PTR records) ──${NC}"
check_record "1.1.168.192.in-addr.arpa." PTR
check_record "2.1.168.192.in-addr.arpa." PTR
check_record "5.1.168.192.in-addr.arpa." PTR
check_record "9.1.168.192.in-addr.arpa." PTR
check_record "10.1.168.192.in-addr.arpa." PTR
check_record "15.1.168.192.in-addr.arpa." PTR
check_record "16.1.168.192.in-addr.arpa." PTR
check_record "20.1.168.192.in-addr.arpa." PTR
check_record "25.1.168.192.in-addr.arpa." PTR
check_record "31.1.168.192.in-addr.arpa." PTR
check_record "51.1.168.192.in-addr.arpa." PTR
check_record "52.1.168.192.in-addr.arpa." PTR
check_record "53.1.168.192.in-addr.arpa." PTR
check_record "54.1.168.192.in-addr.arpa." PTR
check_record "55.1.168.192.in-addr.arpa." PTR
check_record "56.1.168.192.in-addr.arpa." PTR
check_record "61.1.168.192.in-addr.arpa." PTR
check_record "62.1.168.192.in-addr.arpa." PTR
check_record "63.1.168.192.in-addr.arpa." PTR
check_record "99.1.168.192.in-addr.arpa." PTR
check_record "105.1.168.192.in-addr.arpa." PTR
check_record "106.1.168.192.in-addr.arpa." PTR
check_record "143.1.168.192.in-addr.arpa." PTR
check_record "144.1.168.192.in-addr.arpa." PTR
check_record "240.1.168.192.in-addr.arpa." PTR
check_record "241.1.168.192.in-addr.arpa." PTR
check_record "242.1.168.192.in-addr.arpa." PTR
check_record "243.1.168.192.in-addr.arpa." PTR
check_record "244.1.168.192.in-addr.arpa." PTR
check_record "245.1.168.192.in-addr.arpa." PTR

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo -e "${BOLD}── Summary ──${NC}"
TOTAL=$((PASS + FAIL + SKIP))
echo -e "  Total:   ${TOTAL}"
echo -e "  ${GREEN}Passed:  ${PASS}${NC}"
echo -e "  ${RED}Failed:  ${FAIL}${NC}"
echo -e "  ${YELLOW}Skipped: ${SKIP}${NC}"
echo ""

if [[ $FAIL -gt 0 ]]; then
  echo -e "${RED}${BOLD}DNS parity check FAILED — ${FAIL} record(s) differ${NC}"
  exit 1
else
  echo -e "${GREEN}${BOLD}DNS parity check PASSED — all records match${NC}"
  exit 0
fi
