# Homelab Infrastructure — Agent Reference

Ansible-driven homelab managing Bare metal, VMs, Docker services, GPU passthrough, and CI/CD across a multi-host Proxmox cluster.

---

## Documentation Index

### Getting Started

- **Quick Start** → [`docs/setup/getting-started.md`](docs/setup/getting-started.md)
- **macOS Development Setup** → [`docs/setup/macos-setup.md`](docs/setup/macos-setup.md)
- **Development Guide** → [`DEVELOPMENT.md`](DEVELOPMENT.md)

### Architecture

- **System Overview** → [`docs/architecture/overview.md`](docs/architecture/overview.md)
- **Network Design** → [`docs/architecture/networking.md`](docs/architecture/networking.md)
- **Ocean Services** → [`docs/architecture/ocean-services.md`](docs/architecture/ocean-services.md)
- **Deployment Flow** → [`docs/architecture/deployment-flow.md`](docs/architecture/deployment-flow.md)

### Operations

- **Proxmox Management** → [`docs/operations/proxmox.md`](docs/operations/proxmox.md)
- **ZFS Storage** → [`docs/operations/zfs.md`](docs/operations/zfs.md)
- **GPU Management** → [`docs/operations/gpu-management.md`](docs/operations/gpu-management.md)
- **Dell Hardware** → [`docs/operations/dell-hardware.md`](docs/operations/dell-hardware.md)
- **Restoring a service database** → [`docs/operations/db-restore.md`](docs/operations/db-restore.md) — per-service runbook, dry-run by default, point-in-time from GCS generations

### Troubleshooting

- **Common Issues** → [`docs/troubleshooting/common-issues.md`](docs/troubleshooting/common-issues.md)
- **Internal DNS flapping / hostname collisions** → [`docs/troubleshooting/common-issues.md#internal-dns-intermittent--flapping-home-answers`](docs/troubleshooting/common-issues.md#internal-dns-intermittent--flapping-home-answers) · check with [`scripts/dns-drift-check.sh`](scripts/dns-drift-check.sh)

### CI/CD & Automation

- **Adding a new external-repo service (READ FIRST when onboarding a new project)** → [`docs/operations/deploy-pattern.md`](docs/operations/deploy-pattern.md)
- **Validating the deploy chain end-to-end** → [`docs/operations/deploy-tracer.md`](docs/operations/deploy-tracer.md)
- **GitHub Actions Workflows** → [`.github/workflows/`](.github/workflows/)
- **Playbook Documentation** → [`playbooks/README.md`](playbooks/README.md)

### Diagnostics & Admin Scripts

**Reach for [`scripts/`](scripts/README.md) before hand-rolling `ssh host 'docker …'` or
`curl prometheus | jq`.** Full index: [`scripts/README.md`](scripts/README.md). The ones you
want most often:

- **Metrics** → [`scripts/prom.sh`](scripts/prom.sh) `q '<promql>'` / `names` / `targets down`
- **Logs for a service on a host** → [`scripts/loki.sh`](scripts/loki.sh) `<svc> <host> [since] [limit] [match]`
- **systemd across the fleet** → [`scripts/fleet-systemctl.sh`](scripts/fleet-systemctl.sh) `all` / `<service> [host]` / `host <host>`
- **Docker across the fleet** → [`scripts/docker.sh`](scripts/docker.sh) `where <c>` / `status <c> [host]` / `logs`
- **Active alerts** → [`scripts/alerts.sh`](scripts/alerts.sh)
- **Cloudflare (DNS / cache-status / purge)** → [`scripts/cf.sh`](scripts/cf.sh) — `purge` after editing a cached static site
- **Media library (ocean Plex stack, NAS reclaim)** → [`scripts/media_clients.py`](scripts/media_clients.py) `ping` (one client for radarr/sonarr/tdarr/plex/overseerr/tautulli, keys self-discovered on ocean), [`scripts/media-reclaim-report.py`](scripts/media-reclaim-report.py) (library taste profile), [`scripts/media-cull-candidates.py`](scripts/media-cull-candidates.py) `movies`/`tv` (taste-aware cull TSV → hand-edit → feed IDs to) [`scripts/media-reclaim-delete.py`](scripts/media-reclaim-delete.py) (Overseerr-guarded delete + *arr untrack, dry-run unless `--yes`). Reclaim policy: keep watched/monitored/requested/protected-genre/pre-`--since` and the well-rated; cull the mediocre recent. ZFS unlinks async — `df` lags a delete by ~1-2 min.
- **Restart/bounce a service on one host** → [`scripts/fleet-restart.sh`](scripts/fleet-restart.sh) (guarded; the only mutating fleet tool)
- **Public DNS + mail-auth for a domain** → [`scripts/dns-records.sh`](scripts/dns-records.sh) (SPF/DKIM/DMARC), [`scripts/dns-snapshot.sh`](scripts/dns-snapshot.sh) (diffable), [`scripts/dns-timeline.py`](scripts/dns-timeline.py) — external domains, distinct from the internal PowerDNS `dns-drift-check.sh`
- **Vault secrets** → [`scripts/vault.py`](scripts/vault.py) `list [path]` (redacted) / `get <path>` / `check <path> <value>` (exit 1 on mismatch — "did that rotation land?") / `set` / `rotate`. Pass `-` as the value to read it from stdin, so the secret never lands in shell history or `ps`. **Never replace `vault/secrets.yaml` wholesale** — a merged older copy silently deletes keys and the ciphertext diff hides it; CI's "Check no vault keys were dropped" step compares decrypted key paths against the base branch and fails the PR, and clearing it takes a human. Use it instead of `ansible-vault view | grep` or hand-editing: writes are line-edits, so comments and ordering survive, and the result is diffed before re-encrypting so exactly one path can change. Vault-only changes still need a manual dispatch to deploy

---

## Deploy gotchas (READ before shipping a change)

These bite every agent that doesn't know them up front:

- **Default branch is `master`, not `main`.** Open PRs against `master`.
- **Merge to `master` auto-deploys to production.** `main-apply.yml` runs a real
  `ansible-playbook` apply (not a dry run) on push to master. Merge IS deploy —
  there is no separate deploy step to gate.
- **CI green is lint/syntax only.** `ci-validate.yml` never touches live hosts, so a
  playbook can pass CI and still fail at deploy. After merging, watch the
  `main-apply.yml` run — that is where the deploy actually succeeds or fails.
- **`main-apply`'s own post-deploy health is `continue-on-error`.** A DEGRADED deploy
  still reports the job green. For real health verification use `health-check.yml`
  (exits 1 on unhealthy), the `scripts/*-check.sh` tools, or `scripts/homelab-health.sh`
  (fleet-wide diff).
- **Squash-merge only.** `main-apply` detects impact via `git diff HEAD~1 HEAD`, which
  assumes one commit per merge.
- **Deploy path filter.** Only `playbooks/**`, `files/**`, `roles/**`,
  `vars/**/*.yaml`, `.github/workflows/**` trigger CI + `main-apply`. Changes outside
  those (e.g. `scripts/`, `docs/`, `.claude/`) silently do NOT deploy — no CI, no apply.

### How a change maps to playbooks (impact detection)

`main-apply` does NOT run the whole site. `.github/workflows/lib/detect-impacted-playbooks.sh`
maps each changed file to the fewest playbooks — ideally one — and `apply-playbooks`
runs only those. The model is **explicit ownership**, not best-effort guessing:

- **`files/<svc>/**`** → the playbook that references `files/<svc>` literally, else the
  one declaring `service: <svc>`, else a playbook named `<svc>.yaml`. So a new service
  maps cleanly if it follows any one of those conventions.
- **`vars/vars_<name>.yaml`** → the playbook(s) that load it by name.
- **`vars/vars_service_ports.yaml`** (the shared port registry) → mapped by **which
  `service_ports.<key>` changed**, not the filename: a single port bump deploys only the
  service(s) referencing that key; a comment/format-only edit deploys nothing; if the
  key-diff can't be computed it falls back to every consumer. Needs `pyyaml` on the
  runner (installed in the workflow) and the base commit (`fetch-depth: 2`).
- **`roles/<role>/**`** → playbooks importing that role.
- **`inventories/**`, `group_vars/all*`** → the ONLY inputs that replay the site-wide
  orchestrators (`01_base_system`, `02_core_infrastructure`, `03_ocean_services`).
- **Unmapped owned input → hard error (exit 3).** A `files/`/`vars/`/`roles/` change that
  resolves to no playbook FAILS the run with a message, instead of silently fanning out
  across the fleet. Wire it to an owner (or, if it is genuinely deployed by nothing, add
  it to `is_known_unowned()` — that allowlist is the dead-input cleanup ledger).
- **Enforced in CI.** `ci-validate.yml`'s **Validate Deploy Mapping** job runs
  `detect-impacted-playbooks.test.sh` (pinned mappings) and `ownership-coverage.test.sh`
  (asserts every `files/`, `vars/`, `roles/` input resolves to an owner or a declared
  no-op). A new unwired service is caught at PR time, not as a fleet-wide fan-out on push.

### ZFS / storage: NEVER automate (data-safety invariant)

`/data01` (the ZFS pool) is the most critical infrastructure in the fleet; losing data
is the worst possible outcome. So storage is exempt from "merge = deploy":

- **No committed playbook may mutate ZFS.** Enforced by CI: `check-no-zfs-mutations.sh`
  (in the Validate Deploy Mapping job) fails the build on any mutating `zfs`/`zpool` verb
  (`create/destroy/set/mount/unmount/import/export/rollback/replace/...`), the
  `community.general.zfs|zpool` modules, or a `fstype: zfs` mount. Reads (`zpool list`,
  `zfs get`) and non-ZFS mounts (tmpfs) are fine.
- **Storage playbooks are dispatch-only.** The detector treats any `playbooks/**/*zfs*.yaml`
  as never-auto-apply (like terminalbench). Name a storage play `*zfs*.yaml` to inherit the
  guard; run it deliberately via `workflow_dispatch`.
- **The only in-repo ZFS play is read-only.** `playbooks/individual/ocean/data01_zfs.yaml`
  runs `become: false` (unprivileged — cannot mutate), timeout-bounds every command (a
  suspended pool blocks `zfs`/`zpool` forever), asserts `/data01` is mounted (fails if not),
  warns on non-ONLINE health, and records the live layout. It treats the running config as
  gold: it asserts reality is functional, never imposes config.
- **Real pool changes** (replacing a degraded disk, dataset work) are coordinated, hand-run,
  snapshot-first tasks with a backout plan — a runbook, not a committed auto-runnable play.
  Beware side effects: an unmount with open file handles makes services write into the empty
  mountpoint on root (invisible after remount), and property changes (`mountpoint`,
  `canmount`) silently trigger remounts.

---

## Multi-agent coordination

Rules for when more than one agent (or a human plus agents) changes this repo.
The repo drives a production homelab, so coordination is about preventing drift,
deploy races, and resource hijacking.

1. **One source of truth.** `master` is desired state; deployed state must equal
   it. Never hand-edit files on a host or run ad-hoc `docker`/`compose` — changes
   flow repo → branch → PR → merge → playbook. Drift is a defect, not a shortcut.

2. **Branch per change; isolated trees.** Never commit to `master`. One logical
   change per PR. Stage explicit paths (`git add <path>`), never `-A` / `.`. Use a
   separate git worktree or clone per concurrent agent; if a clone is shared, run
   `git status` first and leave any foreign uncommitted changes untouched.

3. **PRs are the coordination ledger.** Open a draft PR (or claim an issue) early
   to signal intent; scan open PRs and branches before starting overlapping work.

4. **Serialize deploys; check before triggering.** Host deploys run under the
   `deploy-ocean` concurrency group (`cancel-in-progress: false`) — keep it.
   Before triggering a deploy, confirm nothing else is actively managing or using
   that service. Don't deploy an unmerged branch except for deliberate testing.

5. **Explicit ownership; no hijacking.** Each service has one owner = its playbook
   plus compose path. A task must never rewrite or restart another service to
   borrow it; if you need another instance, run a separate one (own
   dir/container/port) with CPU/RAM caps. The single GPU is allocated
   deliberately — production keeps it; experiments run on CPU or a declared VRAM
   budget. Exception: a docker-compose collection, or a playbook deploying a
   service alongside its exporters/sidecars, is one owned unit — co-deployed
   from the same playbook.

6. **CI-mediated vs manual.** Ordinary ocean services deploy via CI/dispatch.
   Self-referential infra (the GitHub Actions runners themselves) and anything
   that would kill the job mid-run are applied **manually on the homelab network,
   never via CI** — a runner cannot redeploy itself.

7. **Propose → review → merge → deploy → verify.** Agents propose via PR; merge
   per the authority rule below; the owning agent then deploys and reports
   evidence. Deploys stay manually triggered (not push-to-deploy) by design.

8. **Detect drift; fix shared CI first.** Run the apply playbooks in `--check` on
   a schedule so deployed-≠-repo surfaces on its own. When shared CI breaks,
   fixing it unblocks every agent — prioritize it and fix at the root.

**Merge authority:** humans merge by default; agents open PRs and do not merge
their own changes. Low-risk classes (CI fixes, dashboards, docs) may be delegated
to a designated reviewer once checks are green.

**Isolation:** a git worktree or clone per concurrent agent is preferred;
shared-clone work is allowed only with strict branch discipline and
explicit-path staging.

---

## Choice preferences (how terrac wants homelab work done)

Distilled from prior decisions. Defaults, not laws, but don't override without a reason.

- **Live through CI/CD.** Push, watch the pipeline, iterate. Keep responses terse and
  evidence-backed; terrac is a strong critic of his own ideas, so push back rather than agree.
- **PRD before code.** For non-trivial work, write a zero-ambiguity PRD with explicit
  goals AND anti-goals first, then implement only against that scope. Grill the
  requirements before writing code.
- **New repos inherit the CI/CD deploy pattern** ([`docs/operations/deploy-pattern.md`](docs/operations/deploy-pattern.md)) automatically.
- **Every service is monitored — no exceptions.** Part of shipping any new service:
  (1) it has at least a **health check** (compose `healthcheck:` / systemd health / a
  blackbox probe) so failure is visible, and (2) if it exposes a **Prometheus metrics
  endpoint, add a scrape job** (`files/ocean-prometheus/prometheus.yml.j2`) so the metrics
  land in Prometheus and a Grafana panel. A service that ships unmonitored is unfinished.
  Verify with [`scripts/exporter-scrape-check.sh`](scripts/exporter-scrape-check.sh) (targets down)
  and [`scripts/prom.sh`](scripts/prom.sh) `targets`.
- **Fix through code, never manual SSH.** A playbook / workflow / runner-config change
  is the fix; manual SSH to a host is the escape hatch, not the default. Manual edits
  drift from IaC and get overwritten by the next apply.
- **No `Co-Authored-By: Claude` trailers** on commits. Considered weak; never add them.
- **Ansible house style:** FQCN modules (`ansible.builtin.*`), bare `true`/`false` (not
  `yes`/`no`), quoted octal file modes, and pass package lists to `apt`'s `name` rather
  than looping per item.
- **Vault is a nested tree** (`ai_services.openai.api_key`, `network.unifi.monitoring_user`).
  Reuse existing keys; never duplicate a secret across two locations.
- **Local-first for sensitive data.** PII / FERPA / HIPAA-adjacent work defaults to
  on-prem inference; cloud LLMs are explicit opt-in phase 2, never suggested unprompted.

---

## Quick Reference

**Primary Host:** ocean (192.168.1.143)
**Environment Setup:** `source .envrc`
**Deploy All:** `ansible-playbook -i inventories/production/hosts.ini playbooks/00_site.yaml`
**Validate:** `make validate`
**Adding a new project that deploys here:** follow [`docs/operations/deploy-pattern.md`](docs/operations/deploy-pattern.md) — bootstrap checklist + every gotcha that's bitten this repo before.

---

## Hardware

- **node006** (Dell R720): 40 cores, 680GB RAM, 64TB ZFS, RTX 3090 → ocean VM
- **node005** (Dell R620): 56 cores, 128GB RAM → dns01, pihole, k8s, runners

## Grafana + MySQL Consolidated Stack

MySQL consolidated into Grafana docker-compose (MySQL only serves Grafana):

- **grafana_internal** network: Grafana ↔ MySQL (private, no host exposure)
- **web_proxy** network: nginx ↔ Grafana
- MySQL: percona/percona-server:5.7, 1 CPU, 1GB, buffer_pool=512M
- Storage: `/data01/services/grafana/{mysql-data,mysql-logs,mysql-conf,data,logs}/`
- Deploy: single playbook manages both containers
