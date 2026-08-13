#!/usr/bin/env bash
# Snapshot a domain's full DNS + mail-auth state to a dated JSON-ish file, so
# future changes are diffable. The honest answer to "MXToolbox shows history":
# you can't retro-fetch it for free, but you CAN start capturing it now. Run
# this on a schedule (cron / the CI workflow) and diff successive snapshots.
#
# Usage:
#   dns-snapshot <domain> [outdir]    (default outdir: snapshots/)
#   diff two snapshots with: diff snapshots/<domain>-<dateA>.txt ...-<dateB>.txt
set -euo pipefail

case "${1:-}" in -h|--help|"") sed -n '2,10p' "$0"; exit 0;; esac

d="$1"
outdir="${2:-snapshots}"
mkdir -p "$outdir"
# Date comes from the system clock; pass-through, not inferred.
stamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
day="$(date -u +%Y-%m-%d)"
out="$outdir/$d-$day.txt"

{
  echo "# DNS snapshot $d  captured $stamp"
  echo "## SOA";   dig +short SOA "$d"
  echo "## NS";    dig +short NS "$d" | sort
  echo "## A";     dig +short A "$d" | sort
  echo "## AAAA";  dig +short AAAA "$d" | sort
  echo "## MX";    dig +short MX "$d" | sort
  echo "## TXT";   dig +short TXT "$d" | sort
  echo "## SPF-expanded"
  dig +short TXT "$d" | tr -d '"' | grep -i '^v=spf1' || true
  echo "## DMARC"; dig +short TXT "_dmarc.$d"
  echo "## DKIM-selectors"
  for s in selector1 selector2 google default k1 k2 mail dkim s1 s2 smtp; do
    r="$(dig +short CNAME "${s}._domainkey.$d")"
    [ -n "$r" ] && echo "$s -> $r"
    t="$(dig +short TXT "${s}._domainkey.$d" | head -c 80)"
    [ -n "$t" ] && echo "$s TXT: ${t}..."
  done
  true
} > "$out"

echo "wrote $out ($(wc -l < "$out" | tr -d ' ') lines)"
# If a previous snapshot exists, show the diff immediately.
prev="$(ls -1 "$outdir/$d-"*.txt 2>/dev/null | grep -v "$out" | tail -1 || true)"
if [ -n "$prev" ]; then
  echo "=== diff vs previous ($(basename "$prev")) ==="
  diff "$prev" "$out" && echo "  (no change)" || true
fi
