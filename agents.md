# Homelab Infrastructure — Agent Reference

Ansible-driven homelab managing Bare metal, VMs, Docker services, GPU passthrough, and CI/CD across a multi-host Proxmox cluster.

---

## Where to look

- **Docs index** (setup, architecture, operations, troubleshooting, CI/CD) -> [`docs/README.md`](docs/README.md)
- **Diagnostic + admin tooling** -> [`scripts/README.md`](scripts/README.md). Reach for
  `scripts/` before hand-rolling `ssh host 'docker ...'` or `curl prometheus | jq`:
  `prom.sh` (metrics), `loki.sh` (logs), `fleet-systemctl.sh` / `docker.sh` (fleet state),
  `alerts.sh`, `cf.sh` (Cloudflare), `vault.py` (secrets), `fleet-restart.sh` (the only
  mutating one).
- **Onboarding a new external-repo service** -> [`docs/operations/deploy-pattern.md`](docs/operations/deploy-pattern.md)
- **Validate:** `make validate`

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

### GitHub API / CLI traps (each of these cost hours)

- **The Checks API is restricted to GitHub Apps.** No PAT can read check runs, and
  the `Checks` permission is not offered in the fine-grained token UI at all — on a
  new token or an old one. Do not go looking for the checkbox, and do not "fix" it
  with a classic `repo`-scoped token. `gh pr list --json statusCheckRollup` expands
  to the per-check `contexts` and 403s forever; the GraphQL rollup **aggregate**
  (`commits(last:1){nodes{commit{statusCheckRollup{state}}}}`) needs no extra
  permission and is what `files/agentbox/issue-watcher.sh` uses.
- **`gh`'s `--jq` is not jq.** One expression, no `--arg`. Passing one makes `gh`
  reject the whole command. Pipe to real jq: `gh ... --json ... | jq -r --arg ...`.
- **`x-accepted-github-permissions`** on any response names the exact permission an
  endpoint wants. Read it instead of guessing at a 403.
- **Listings lie.** Google's models endpoint advertises models that error on every
  call (`gemini-2.5-pro`, 2026-08-31). Round-trip a model before wiring it in, and
  remember reachability is not capability — make it do the real task.
- **Two similarly-named checks are not the same check.** This repo has both
  `Validate Ansible Playbooks` (`ci-validate.yml`) and `Validate Playbooks`
  (`pr-deploy.yml`). Match job names exactly via `gh run view <id> --json jobs`.

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
  land in Prometheus and a Grafana panel, and (3) it runs as a **compose project**, which
  is what enables its `docker-refresh@<project>` timer and keeps a floating tag current
  (a bare `docker run` service never gets a new image again — see
  [`docs/operations/deploy-pattern.md`](docs/operations/deploy-pattern.md)).
  A service that ships unmonitored is unfinished.
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

