#!/usr/bin/env bash
# Every <name>.home that nginx serves must have its own DNS record. Without the
# *.home wildcard, a vhost added to the proxy config but not to nginx_lb_vhosts
# simply doesn't resolve, and nothing else would notice until someone tried it.
# Run from repo root: bash .github/workflows/lib/nginx-vhosts-have-dns.test.sh
set -euo pipefail

CONF=files/nginx-compose/proxy_hostname_web_proxy.conf
DEFAULTS=roles/dns_infrastructure/defaults/main.yaml

served=$(grep -oE 'server_name[^;]+;' "$CONF" | sed 's/server_name//; s/;//' \
  | tr ' ' '\n' | grep -E '\.home$' | sed 's/\.home$//' | sort -u)

recorded=$(awk '/^nginx_lb_vhosts:/{on=1; next} on && /^  - /{print $2; next} on{exit}' "$DEFAULTS" | sort -u)

[ -n "$served" ] || { echo "FAIL: parsed no .home server_name from $CONF"; exit 1; }

missing=$(comm -23 <(echo "$served") <(echo "$recorded"))
stale=$(comm -13 <(echo "$served") <(echo "$recorded"))

rc=0
if [ -n "$missing" ]; then
  echo "FAIL: nginx serves these .home names with no DNS record; add them to nginx_lb_vhosts in $DEFAULTS:"
  echo "$missing" | sed 's/^/  /'
  rc=1
fi
if [ -n "$stale" ]; then
  echo "FAIL: nginx_lb_vhosts lists names nginx no longer serves; drop them from $DEFAULTS:"
  echo "$stale" | sed 's/^/  /'
  rc=1
fi
[ "$rc" -eq 0 ] && echo "PASS: $(echo "$served" | wc -l | tr -d ' ') nginx .home vhosts all have DNS records"
exit "$rc"
