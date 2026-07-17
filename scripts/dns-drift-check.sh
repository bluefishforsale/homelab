#!/usr/bin/env bash
# DNS Drift Check - detect divergence between the two authoritative PowerDNS
# servers (dns01/dns02) and the DDNS hostname collisions that cause it.
#
# The dns-stack is dual-primary with NO replication: each host runs its own
# PowerDNS + Kea DHCP + kea-dhcp-ddns, and Kea DDNS-updates only its LOCAL
# PowerDNS (127.0.0.1:53). So the two servers drift whenever DDNS resolves a
# conflict differently. pihole forwards .home to BOTH (server=/home/.2 and
# /home/.3 in dnsmasq), so clients get whichever answers first -> flapping.
#
# The usual cause of drift is a multi-interface device announcing ONE DHCP
# hostname on TWO MACs: each NIC gets its own pool IP, each DDNS-registers the
# same name, DHCID conflict-checking then locks a different winner per server.
# This script surfaces both the symptom (zone diff) and the cause (lease
# collisions). Static/reserved records are checked by dns-parity-check.sh.
#
# Usage: ./scripts/dns-drift-check.sh [--a 192.168.1.2] [--b 192.168.1.3] [--no-leases]
# Exit:  0 clean, 1 drift or collisions found, 2 a server was unreachable.

set -euo pipefail

A="192.168.1.2"; B="192.168.1.3"
SSH_USER="debian"
LEASE_CSV="/opt/dns-stack/data/kea/kea-leases4.csv"
FWD_ZONE="home."
REV_ZONE="1.168.192.in-addr.arpa."
CHECK_LEASES=true

RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[0;33m'; BOLD=$'\033[1m'; NC=$'\033[0m'

while [[ $# -gt 0 ]]; do
  case $1 in
    --a) A="$2"; shift 2 ;;
    --b) B="$2"; shift 2 ;;
    --no-leases) CHECK_LEASES=false; shift ;;
    -h|--help) grep -E '^# ' "$0" | sed 's/^# //'; exit 0 ;;
    *) echo "unknown option: $1"; exit 1 ;;
  esac
done

fail=0; unreachable=0
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# --- zone divergence: AXFR each zone from both servers and diff (drop SOA) ---
diff_zone() {
  local zone="$1"
  dig +time=5 +tries=1 "@${A}" "$zone" AXFR 2>/dev/null | grep -vE '^;|^$' | grep -v 'IN[[:space:]]*SOA' | sort >"$tmp/a"
  dig +time=5 +tries=1 "@${B}" "$zone" AXFR 2>/dev/null | grep -vE '^;|^$' | grep -v 'IN[[:space:]]*SOA' | sort >"$tmp/b"
  if [[ ! -s "$tmp/a" || ! -s "$tmp/b" ]]; then
    printf "  ${RED}AXFR failed${NC} for %s (a=%s lines, b=%s lines)\n" "$zone" "$(wc -l <"$tmp/a")" "$(wc -l <"$tmp/b")"
    unreachable=1; return
  fi
  if diff -q "$tmp/a" "$tmp/b" >/dev/null; then
    printf "  ${GREEN}in sync${NC}   %-28s (%s records)\n" "$zone" "$(wc -l <"$tmp/a")"
  else
    printf "  ${RED}DIVERGED${NC}  %s\n" "$zone"
    diff "$tmp/a" "$tmp/b" | grep -E '^[<>]' | sed "s/^</    ${A} only:/;s/^>/    ${B} only:/"
    fail=1
  fi
}

echo
echo "${BOLD}DNS drift check — ${A} vs ${B}${NC}"
echo "${BOLD}── zone divergence (AXFR diff, SOA ignored) ──${NC}"
diff_zone "$FWD_ZONE"
diff_zone "$REV_ZONE"

# --- DDNS hostname collisions: same hostname, >1 MAC, in the Kea lease DB ---
# state col 10 == 0 is active; Kea memfile appends so last row per IP wins.
if $CHECK_LEASES; then
  echo
  echo "${BOLD}── DDNS hostname collisions (Kea leases) ──${NC}"
  for host in "$A" "$B"; do
    if ! csv="$(timeout 15 ssh -o BatchMode=yes -o ConnectTimeout=8 "${SSH_USER}@${host}" "sudo cat ${LEASE_CSV}" 2>/dev/null)"; then
      printf "  ${YELLOW}skip${NC} %s (ssh/sudo cat failed)\n" "$host"; unreachable=1; continue
    fi
    report="$(printf '%s\n' "$csv" | awk -F, '
      NR>1 && $1!="address" { mac[$1]=$2; name[$1]=$9; st[$1]=$10 }   # last row per IP wins
      END {
        for (a in mac) if (st[a]=="0" && name[a]!="") {
          key=name[a]
          if (!(key SUBSEP mac[a] in seenmac)) { seenmac[key SUBSEP mac[a]]=1; distinct[key]++ }
          detail[key]=detail[key] sprintf("      %-16s %s\n",a,mac[a])
        }
        for (k in distinct) if (distinct[k]>1) { print "  COLLISION " k; printf "%s", detail[k] }
      }')"
    if [[ -n "$report" ]]; then
      printf "  ${RED}%s${NC}\n%s\n" "$host" "$report"; fail=1
    else
      printf "  ${GREEN}no collisions${NC} on %s\n" "$host"
    fi
  done
fi

echo
if [[ $unreachable -eq 1 && $fail -eq 0 ]]; then
  echo "${YELLOW}${BOLD}incomplete — a server/lease source was unreachable${NC}"; exit 2
elif [[ $fail -gt 0 ]]; then
  echo "${RED}${BOLD}DNS drift detected — see divergences/collisions above${NC}"
  echo "Remediation: give each colliding device's interfaces a Kea reservation"
  echo "with a stable IP + distinct hostname, then delete the stale A/PTR records."
  exit 1
else
  echo "${GREEN}${BOLD}no drift — authoritative servers agree, no hostname collisions${NC}"; exit 0
fi
