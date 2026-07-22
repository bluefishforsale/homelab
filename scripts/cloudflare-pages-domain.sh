#!/usr/bin/env bash
# Cloudflare Pages custom-domain setup.
#
# Attaches a custom domain to a Cloudflare Pages project via the API. This is the
# "register the domain in Pages FIRST" step: doing it before the DNS record exists
# means Pages already knows the hostname, so when the CNAME resolves the cert
# validates instead of the domain returning a 522.
#
# It does NOT touch DNS. DNS records live at the domain's registrar / DNS host
# (for radiantatmospheres.com that is LMI, and the domain deliberately does NOT
# move to Cloudflare nameservers). Per radiantatmospheres.com ADR-0005 the records
# there are: `www` CNAME -> <project>.pages.dev, and apex 301-redirect -> www.
# This script prints that reminder with the correct target after attaching.
#
# Idempotent: re-running for an already-attached domain is a no-op.
#
# Usage:
#   cloudflare-pages-domain.sh <project> <domain>
#   cloudflare-pages-domain.sh radiantatmospheres www.radiantatmospheres.com
#
# Auth + account, from env (source these from the vault, e.g. vault/secrets.yaml
# key `cloudflare`; see .envrc):
#   CLOUDFLARE_ACCOUNT_ID                        required
#   CLOUDFLARE_API_TOKEN                         scoped token, Bearer (preferred)
#   CLOUDFLARE_API_KEY + CLOUDFLARE_API_EMAIL    global key (fallback)
set -euo pipefail

API=https://api.cloudflare.com/client/v4
proj="${1:?usage: cloudflare-pages-domain.sh <project> <domain>}"
domain="${2:?usage: cloudflare-pages-domain.sh <project> <domain>}"
: "${CLOUDFLARE_ACCOUNT_ID:?set CLOUDFLARE_ACCOUNT_ID (from vault:cloudflare.account_id)}"

# Auth: prefer a scoped token; fall back to the global key.
if [[ -n "${CLOUDFLARE_API_TOKEN:-}" ]]; then
  auth=(-H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}")
elif [[ -n "${CLOUDFLARE_API_KEY:-}" && -n "${CLOUDFLARE_API_EMAIL:-}" ]]; then
  auth=(-H "X-Auth-Email: ${CLOUDFLARE_API_EMAIL}" -H "X-Auth-Key: ${CLOUDFLARE_API_KEY}")
else
  echo "no Cloudflare auth: set CLOUDFLARE_API_TOKEN, or CLOUDFLARE_API_KEY + CLOUDFLARE_API_EMAIL" >&2
  exit 1
fi

base="${API}/accounts/${CLOUDFLARE_ACCOUNT_ID}/pages/projects/${proj}/domains"

# Already attached? (idempotent)
if curl -fsS "${auth[@]}" "$base" \
   | python3 -c "import sys,json;print('\n'.join(d['name'] for d in (json.load(sys.stdin).get('result') or [])))" \
   | grep -qxF "$domain"; then
  echo "already attached: ${domain} (project ${proj})"
else
  echo "attaching ${domain} to Pages project ${proj} ..."
  resp=$(curl -fsS -X POST "${auth[@]}" -H "Content-Type: application/json" \
         --data "{\"name\":\"${domain}\"}" "$base")
  if [[ "$(python3 -c "import sys,json;print(json.load(sys.stdin)['success'])" <<<"$resp")" != "True" ]]; then
    echo "FAILED to attach ${domain}:" >&2
    python3 -m json.tool <<<"$resp" >&2
    exit 1
  fi
  echo "attached."
fi

# Report status of the target domain.
echo "--- ${domain} ---"
curl -fsS "${auth[@]}" "$base" | python3 -c "
import sys, json
tgt = '${domain}'
for d in (json.load(sys.stdin).get('result') or []):
    if d['name'] == tgt:
        print('status     :', d.get('status'))
        print('validation :', (d.get('validation_data') or {}).get('status', 'n/a'))
"

# DNS reminder with the correct CNAME target.
sub=$(curl -fsS "${auth[@]}" "${API}/accounts/${CLOUDFLARE_ACCOUNT_ID}/pages/projects/${proj}" \
      | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['subdomain'])")
cat <<EOF

NEXT (DNS records, at the domain's registrar / DNS host - NOT set by this script):
  CNAME   ${domain}   ->   ${sub}
  apex    (if used)   ->   301 redirect to https://${domain}
Cloudflare Pages will validate and issue the TLS cert once that CNAME resolves.
EOF
