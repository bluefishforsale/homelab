#!/usr/bin/env python3
"""Apply static-site edge hardening to Cloudflare zones: TLS enforcement + security headers.

For static sites (no dynamic surface) the useful hardening is HTTPS/TLS enforcement and
security response headers, not the WordPress path blocks in cloudflare-waf.py. This sets,
per zone:

  - Always Use HTTPS, Min TLS 1.2, TLS 1.3
  - HSTS (180d, includeSubDomains, nosniff; preload OFF until you're sure)
  - a response-header Transform Rule: X-Content-Type-Options, X-Frame-Options,
    Referrer-Policy, Permissions-Policy, and a REPORT-ONLY CSP (staged: violations show in
    the browser console so you can tune per site before flipping to enforcing).

Idempotent (GET-merge-PUT, replaces only "homelab:"-tagged rules). Creds come from the vault
at runtime; nothing secret is committed. Zone IDs are public identifiers already committed in
plaintext in playbooks/individual/ocean/monitoring/cloudflare_exporter.yaml (cf_zones).

    scripts/cloudflare-harden.py [zone_key ...]   # default: all zones below

Needs ~/.ansible_vault_pass. Run from the repo root on the dev laptop.
"""
import json
import os
import subprocess
import sys
import urllib.error
import urllib.request

import yaml

API = "https://api.cloudflare.com/client/v4"
TAG = "homelab:"

# Public zone identifiers (see cf_zones in cloudflare_exporter.yaml). Not secret.
ZONES = {
    "terrac_com": "f3be74ced16c63d68b522f569ecba43b",
    "saetnere_com": "427cd2f89d3669d1704f61f3d37d4149",
    "radiantatmospheres_com": "87401274f499c81d52527080d0588e7f",
}

# Zone-level SSL/TLS settings (PATCH /zones/{id}/settings/{name}).
SETTINGS = {
    "always_use_https": "on",
    "min_tls_version": "1.2",
    "tls_1_3": "on",
    "security_header": {
        "strict_transport_security": {
            "enabled": True,
            "max_age": 15552000,  # 180 days
            "include_subdomains": True,
            "nosniff": True,
            "preload": False,  # flip on only once every subdomain is HTTPS-clean
        }
    },
}

CSP_REPORT_ONLY = (
    "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "
    "script-src 'self' 'unsafe-inline'; font-src 'self' data:; object-src 'none'; "
    "base-uri 'self'; frame-ancestors 'self'"
)

SECURITY_HEADERS = {
    "X-Content-Type-Options": "nosniff",
    "X-Frame-Options": "SAMEORIGIN",
    "Referrer-Policy": "strict-origin-when-cross-origin",
    "Permissions-Policy": "geolocation=(), microphone=(), camera=()",
    "Content-Security-Policy-Report-Only": CSP_REPORT_ONLY,
}


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
            return None  # entrypoint doesn't exist yet -> the PUT creates it
        sys.exit(f"{method} {path} -> {e.code}\n{e.read().decode()}")


def apply_settings(headers, zone):
    for name, value in SETTINGS.items():
        api("PATCH", f"/zones/{zone}/settings/{name}", headers, {"value": value})
    print(f"  settings: {', '.join(SETTINGS)}")


def apply_headers(headers, zone):
    phase = "http_response_headers_transform"
    ep = api("GET", f"/zones/{zone}/rulesets/phases/{phase}/entrypoint", headers, ok404=True)
    existing = ((ep or {}).get("result") or {}).get("rules") or []
    kept = [r for r in existing if not (r.get("description") or "").startswith(TAG)]
    rule = {
        "action": "rewrite",
        "action_parameters": {
            "headers": {
                k: {"operation": "set", "value": v} for k, v in SECURITY_HEADERS.items()
            }
        },
        "expression": "true",
        "description": TAG + "security headers",
    }
    api("PUT", f"/zones/{zone}/rulesets/phases/{phase}/entrypoint", headers, {"rules": kept + [rule]})
    print(f"  headers: {len(kept)} preserved + 1 homelab = {len(kept) + 1} rules")


def main():
    keys = sys.argv[1:] or list(ZONES)
    cf = vault()["cloudflare"]
    headers = {
        "X-Auth-Email": cf["api_email"],
        "X-Auth-Key": cf["api_key"],
        "Content-Type": "application/json",
    }
    for key in keys:
        if key not in ZONES:
            sys.exit(f"unknown zone key: {key} (known: {', '.join(ZONES)})")
        zone = ZONES[key]
        print(f"zone {key} ({zone}):")
        apply_settings(headers, zone)
        apply_headers(headers, zone)
    print("done")


if __name__ == "__main__":
    main()
