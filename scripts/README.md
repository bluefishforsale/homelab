# scripts/

Admin, diagnostic, and action helpers for the homelab. **Reach for these before
hand-rolling `ssh host 'docker ...'` or `curl prometheus | jq`** — they exist so those
one-offs stop getting rewritten every session.

Everything here runs from the dev laptop (or a checkout on the LAN). The query tools need
the homelab LAN reachable; the Cloudflare tools need the ansible vault
(`~/.ansible_vault_pass`). **`scripts/` is outside the `main-apply` push filter — changes
here never trigger CI or a deploy**, so a scripts-only PR merges with no checks.

## Diagnostics & query (read-only)

| Script | What | Example |
|---|---|---|
| `prom.sh` | Prometheus: `q '<promql>'`, `raw`, `names [regex]`, `labels <metric>`, `targets [up\|down]` | `prom.sh q 'up==0'` |
| `loki.sh` | logs for a service on a host (auto container→unit; `-c`/`-u` to force) | `loki.sh blog_saetnere_com_wp ocean 2h 200 error` |
| `fleet-systemctl.sh` | systemd across the fleet: `all` / `list [host]` / `<service> [host]` / `host <host>` | `fleet-systemctl.sh all` |
| `docker.sh` | containers across the fleet: `ps [host]` / `where <c>` / `status <c> [host]` / `logs <c> [host] [n]` | `docker.sh where plex` |
| `alerts.sh` | Alertmanager: active alerts (default), `all`, `silences` | `alerts.sh` |
| `exporter-scrape-check.sh` | Prometheus targets that are down + their exporter | `exporter-scrape-check.sh` |
| `dns-drift-check.sh` | divergence between the two authoritative PowerDNS nodes (internal) | `dns-drift-check.sh` |
| `dns-parity-check.sh` | compare dns01 vs dns02 HA nodes record-by-record (internal) | `dns-parity-check.sh` |
| `homelab-health.sh` | fleet health gate by diff: `snapshot <f>` / `verify <f>` (used by the ship flow) | `homelab-health.sh verify /tmp/base.json` |

## DNS & mail-auth (external / public domains)

These inspect a domain's *public* DNS + mail-auth posture (any domain, e.g. the Cloudflare
zones) — distinct from the internal PowerDNS HA checks above. Copied from the
artesiannetwork.com deliverability tooling. `dig`/`whois`, stdlib only.

| Script | What | Example |
|---|---|---|
| `dns-records.sh` | dump mail-relevant records: NS/A/MX/TXT/SPF/DMARC + probe common DKIM selectors | `dns-records.sh saetnere.com` |
| `dns-snapshot.sh` | snapshot full DNS + mail-auth state to a dated file, so changes are diffable | `dns-snapshot.sh terrac.com snapshots/` |
| `dns-timeline.py` | merge dated signals (whois, SOA serial, crt.sh, DMARC reports via `--reports`) into one timeline | `dns-timeline.py terrac.com` |

## Actions (mutating — used deliberately)

| Script | What | Notes |
|---|---|---|
| `fleet-restart.sh` | `restart <unit> <host>` (recreates via compose down/pull/up) or `container <name> <host>` (in-place bounce) | one host, existence-checked, confirm unless `-y`, passwordless `sudo -n` only |
| `cf.sh` | Cloudflare: `zones` / `cache-status <url>` / `dns <zone> [name]` / `dns-add …` / `purge <zone> [url…]` | vault creds; `purge` pairs with the 2h static edge cache |
| `cloudflare-harden.py` | per-zone TLS/HSTS + security headers + host-scoped static cache rule | idempotent; classifier may block the live run → run by hand |
| `cloudflare-waf.py` | per-zone WAF block + rate-limit rules (WordPress zones) | idempotent; run by hand |
| `media-reclaim-report.py` / `media-reclaim-delete.py` | profile the library / delete via Radarr+Sonarr with unmonitor + import-exclusion | delete is destructive |
| `media_clients.py` | one client wrapping radarr/sonarr/tdarr/plex/overseerr/tautulli | library used by the media scripts |
| `vault.py` | `vault/secrets.yaml`: `list [path]` (redacted) / `get <path>` / `check <path> <value>` / `set <path> <value>` / `rotate <path> <value>` | writes are line-edits (comments + ordering survive), diffed before re-encrypting so exactly one path can change, then encrypted beside the vault and decrypted back to prove it round-trips before `os.replace` — the vault is never overwritten by an unverified write. Exit codes: `0` ok, `1` `check` mismatch, `2` error, which is how you tell "rotation didn't land" from "typo'd the path". Self-check: `python3 scripts/test_vault.py` |

## Environment overrides

| Var | Default | Used by |
|---|---|---|
| `HOMELAB_PROM` | `http://192.168.1.143:9090` | prom.sh, homelab-health.sh |
| `HOMELAB_ALERTMANAGER` | `http://192.168.1.143:9093` | alerts.sh, homelab-health.sh |
| `HOMELAB_LOKI` | `http://192.168.1.143:3100` | loki.sh |
| `HOMELAB_INVENTORY` | `inventories/production/hosts.ini` | fleet-systemctl.sh, docker.sh, fleet-restart.sh |
| `SSH_TIMEOUT` | `5` | the ssh-based fleet tools |
| `HOMELAB_VAULT` | `vault/secrets.yaml` | vault.py (point it at a copy to rehearse a rotation) |
| `ANSIBLE_VAULT_PASSWORD_FILE` | `~/.ansible_vault_pass` | vault.py |
| `~/.ansible_vault_pass` | — | cf.sh, cloudflare-*.py |

The ssh-based tools resolve hosts from the ansible inventory (reached as
`ansible_user@ansible_ssh_host` — SSH aliases/users are not authoritative) via the shared
`scripts/lib/inventory.sh` (`fleet_hosts` / `resolve_host` / `on_host`).
