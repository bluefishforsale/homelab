# Homelab Infrastructure — Agent Reference

Ansible-driven homelab managing Docker services, GPU passthrough, and CI/CD across a multi-host Proxmox cluster.

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

### Troubleshooting

- **Common Issues** → [`docs/troubleshooting/common-issues.md`](docs/troubleshooting/common-issues.md)

### CI/CD & Automation

- **Adding a new external-repo service (READ FIRST when onboarding a new project)** → [`docs/operations/deploy-pattern.md`](docs/operations/deploy-pattern.md)
- **Validating the deploy chain end-to-end** → [`docs/operations/deploy-tracer.md`](docs/operations/deploy-tracer.md)
- **GitHub Actions Workflows** → [`.github/workflows/`](.github/workflows/)
- **Playbook Documentation** → [`playbooks/README.md`](playbooks/README.md)

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
   budget.

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
