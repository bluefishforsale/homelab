#!/usr/bin/env python3
"""Apply Cloudflare edge hardening (WAF custom rules + rate limiting) to a zone.

Idempotent: each phase entrypoint is fetched, our rules (tagged "homelab:") are
replaced, any other rules are preserved, then the ruleset is PUT back. Safe to
re-run.

Creds come from the ansible vault at runtime (nothing secret is committed).
Run from the repo root on the dev laptop (needs ~/.ansible_vault_pass):

    scripts/cloudflare-waf.py [zone_key]   # zone_key defaults to saetnere_com

Free plan allows 5 custom WAF rules and 1 rate-limiting rule per zone, which is
what this stays within.
"""
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request

import yaml

TAG = "homelab:"  # marks rules this script owns, so re-runs replace instead of duplicate
API = "https://api.cloudflare.com/client/v4"

# WAF custom rules (phase http_request_firewall_custom). Blocks the whole XML-RPC
# surface at the edge; the mu-plugin is the origin-side belt-and-suspenders.
CUSTOM_RULES = [
    {
        "action": "block",
        "expression": '(http.request.uri.path eq "/xmlrpc.php")',
        "description": TAG + "block xmlrpc",
    },
]

# One rate-limiting rule (phase http_ratelimit): throttle wp-login brute force.
# 5 requests / 10s per IP -> block 60s. A human login is 1-2 requests.
RATELIMIT_RULES = [
    {
        "action": "block",
        "expression": '(http.request.uri.path eq "/wp-login.php")',
        "description": TAG + "rate-limit wp-login",
        "ratelimit": {
            "characteristics": ["ip.src", "cf.colo.id"],
            "period": 10,
            "requests_per_period": 5,
            "mitigation_timeout": 60,
        },
    },
]


def vault():
    passfile = os.path.expanduser("~/.ansible_vault_pass")
    here = os.path.dirname(os.path.abspath(__file__))
    secrets = os.path.join(here, "..", "vault", "secrets.yaml")
    out = subprocess.check_output(
        ["ansible-vault", "view", "--vault-password-file", passfile, secrets]
    )
    return yaml.safe_load(out)


def api(method, path, headers, body=None, ok404=False):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(API + path, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        if e.code == 404 and ok404:
            return None  # phase entrypoint doesn't exist yet -> the PUT below creates it
        sys.exit(f"{method} {path} -> {e.code}\n{e.read().decode()}")


def apply_phase(headers, zone, phase, rules):
    # A zone with no rules in this phase has no entrypoint ruleset yet -> GET 404s; PUT creates it.
    ep = api("GET", f"/zones/{zone}/rulesets/phases/{phase}/entrypoint", headers, ok404=True)
    existing = ((ep or {}).get("result") or {}).get("rules") or []
    kept = [r for r in existing if not (r.get("description") or "").startswith(TAG)]
    merged = kept + rules
    api(
        "PUT",
        f"/zones/{zone}/rulesets/phases/{phase}/entrypoint",
        headers,
        {"rules": merged},
    )
    print(f"  {phase}: {len(kept)} preserved + {len(rules)} homelab = {len(merged)} rules")


def main():
    zone_key = sys.argv[1] if len(sys.argv) > 1 else "saetnere_com"
    cf = vault()["cloudflare"]
    zone = cf[zone_key]
    headers = {
        "X-Auth-Email": cf["api_email"],
        "X-Auth-Key": cf["api_key"],
        "Content-Type": "application/json",
    }
    print(f"zone {zone_key} ({zone}):")
    apply_phase(headers, zone, "http_request_firewall_custom", CUSTOM_RULES)
    apply_phase(headers, zone, "http_ratelimit", RATELIMIT_RULES)
    print("done")


if __name__ == "__main__":
    main()
