#!/usr/bin/env bash
# Dump mail-relevant DNS records for a domain.
# Usage: dns-records [domain]   (default: terrac.com)
set -euo pipefail

case "${1:-}" in -h|--help) sed -n '2,3p' "$0"; exit 0;; esac
d=${1:-terrac.com}

echo "=== NS ==="
dig +short NS "$d"

echo "=== A ==="
dig +short A "$d"

echo "=== MX ==="
dig +short MX "$d"

echo "=== TXT (SPF + verification) ==="
dig +short TXT "$d"

echo "=== DMARC ==="
dig +short TXT "_dmarc.$d"

echo "=== DKIM (probing common selectors) ==="
for s in google default selector1 selector2 k1 k2 mail dkim s1 s2 smtp; do
  r=$(dig +short TXT "${s}._domainkey.$d" || true)
  [ -n "$r" ] && echo "[$s] $r"
  c=$(dig +short CNAME "${s}._domainkey.$d" || true)
  [ -n "$c" ] && echo "[$s -> CNAME] $c"
done
true
