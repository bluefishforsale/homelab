# Deploy Pipeline Shape-Up — Design

**Date:** 2026-05-10
**Repo:** `bluefishforsale/homelab`
**Default branch:** `master`

## Goal

Make changes to homelab-managed code — and code in external service repos — propagate to the right host (ocean, dns01, gh-runner-01, etc.) without manual edits in three places. Onboarding a new web-facing service should be a one-command operation. Validate every step with `gh` so future drift is detectable.

## Non-goals

- Image-tag polling / watchtower-style auto-redeploy on upstream image rebuild.
- Replacing the `git diff HEAD~1 HEAD` push-range detection with proper push-range traversal.
- HA / multi-host deploys for any service.
- Migrating non-GitHub CI (the legacy `.gitlab-ci.yml`).

## Current state (verified)

- Self-hosted runners on `gh-runner-01` (192.168.1.250) — 4 ephemeral Docker containers, mount host `/root/.ssh` at `/root/.ssh:ro` and a `.github_token` file (`roles/github_docker_runners/templates/docker-compose.yml.j2`).
- `.github/workflows/main-apply.yml` triggers on push to `master`/`main` with path filters on `playbooks/**`, `files/**`, `roles/**`, `vars/**/*.yaml`, `group_vars/**`. Detects changed playbooks via `git diff HEAD~1 HEAD` (`fetch-depth: 2`).
- Sledgehammer rule: any change under `roles/`, `group_vars/`, `files/`, `vars/` adds `playbooks/01_base_system.yaml`, `playbooks/02_core_infrastructure.yaml`, `playbooks/03_ocean_services.yaml` to the apply list.
- `ANSIBLE_VAULT_PASSWORD` is read from a GitHub Actions secret in every deploy workflow (`main-apply.yml`, `deploy-terrac-com.yml`, etc.).
- Routing config for web-facing services lives in three hand-maintained places:
  - service playbook: `playbooks/individual/<host>/.../<svc>.yaml`
  - internal vhost: `files/nginx-compose/proxy_hostname_web_proxy.conf` (flat file, hand-listed)
  - external ingress: `vars/vars_cloudflared.yaml` (flat list under `cloudflared.tunnels.main.ingress`)
- Two existing external-repo deploy patterns:
  - **Image-based** (paia) — external repo builds image → GHCR; homelab playbook (`playbooks/individual/ocean/ai/paia.yaml`) pulls `image: ghcr.io/.../paia:latest`. No auto-redeploy on upstream image push.
  - **Source-pull** (terrac.com) — external repo dispatches `repository_dispatch: deploy-terrac-com` → `deploy-terrac-com.yml` → `terrac_com_static.yaml` plays git-clones and rsyncs the built `dist/`.
- Homepage end-to-end works today (user-confirmed).

## Phases

Each phase gates on the previous. No phase removes existing behavior. Each phase will be turned into its own implementation plan via `writing-plans` — this spec is the umbrella strategy, not a single executable plan.

### Phase 1 — Tracer

Order within the phase: items 1 and 2 (infrastructure changes) land first, then items 3–5 (validation) prove the changes work. Tracer commits are meaningless until narrow detection is in place.

Validate the chain end-to-end *before* changing structure. Adds concurrency + narrow detection as foundation.

1. **Concurrency groups (all deploy workflows).** Add `concurrency: { group: deploy-${{ <target_host> }}, cancel-in-progress: false }` to every workflow that runs Ansible against production. Two pushes that target different hosts run in parallel; two pushes that target the same host serialize.
   - `target_host` is resolved in a small setup step that greps `hosts:` from each changed playbook. Multiple hosts in one run use the most-restrictive group (alphabetical join, e.g., `deploy-dns01-ocean`).

2. **Detection narrowing — replace the sledgehammer.** In `main-apply.yml`'s `detect-changes` job, replace the "shared resources changed → add 01/02/03" block with file-pattern → playbook rules:

   | Change pattern | Playbooks triggered |
   |----------------|---------------------|
   | `playbooks/individual/<host>/<category>/<svc>.yaml` | just that playbook |
   | `files/<service-dir>/**` | playbooks that reference `files/<service-dir>` (computed via grep) |
   | `roles/<role>/**` | playbooks containing `roles: [<role>]` or `- role: <role>` |
   | `vars/vars_web_services.yaml` *(introduced in Phase 2)* | affected service playbook(s) + `nginx_compose.yaml` + `cloudflared.yaml` |
   | `vars/vars_cloudflared.yaml`, `files/cloudflared/**` | `cloudflared.yaml` only |
   | `files/nginx-compose/**` | `nginx_compose.yaml` only |
   | `vars/vars_service_ports.yaml` | all playbooks that reference it (computed via grep) |
   | `inventories/**`, `group_vars/all*.yaml`, anything not matched above | fall back to orchestrator replay (sledgehammer remains as the safety net) |

   Implementation: the rules live in `.github/workflows/lib/detect-impacted-playbooks.sh` so they can be unit-tested with sample diffs.

3. **Tracer commit on homepage.** A no-op edit to `playbooks/individual/ocean/services/homepage.yaml` (e.g., update a comment) pushed as a **single commit** (the `HEAD~1..HEAD` diff window only sees the tip commit).

4. **`gh`-driven validation.** Captured in `docs/operations/deploy-tracer.md` as the canonical runbook. After pushing:
   ```
   RUN=$(gh run list --workflow=main-apply.yml --limit 1 --json databaseId -q '.[0].databaseId')
   gh run watch $RUN --exit-status
   curl -sI https://homepage.terrac.com
   curl -sI http://homepage.home
   gh run view $RUN --log-failed   # only if it failed
   ```
   Expected: exactly one playbook applied (`homepage.yaml`), both URLs return 200, total wall time documented.

5. **Second tracer on a non-ocean playbook** (lightest available — likely a comment edit in `playbooks/individual/infrastructure/github_docker_runners.yaml` *without* touching the role). Validates the host-extraction concurrency logic and the narrow-detection rules outside the ocean path. Same `gh run watch` pattern.

**Phase 1 done when:** both tracer commits land green, the runbook is checked in, and a deliberate "edit homepage routing" tracer (touching only `files/nginx-compose/proxy_hostname_web_proxy.conf` for the homepage block) runs *only* the nginx playbook — not the full orchestrator chain.

### Phase 2 — Service registry

Consolidate web-service routing into one source of truth.

1. **New file: `vars/vars_web_services.yaml`.** One entry per web-facing service:
   ```yaml
   web_services:
     homepage:
       host: ocean                       # ansible inventory host
       upstream_port: 8089
       host_internal: homepage.home
       host_external: homepage.terrac.com
       healthcheck_path: /
       nginx_overrides: {}               # optional: timeouts, websocket headers, custom locations
       cloudflared_overrides: {}         # optional: origin_request.no_tls_verify, origin_server_name
   ```

2. **nginx playbook generates per-service vhosts.** Replace the flat `files/nginx-compose/proxy_hostname_web_proxy.conf` with one generated `conf.d/<service>.conf` per registry entry, rendered from a single Jinja template that consults `nginx_overrides` for per-service quirks. ComfyUI's websocket upgrade, Loki's longer `proxy_read_timeout`, etc., move into override blocks.

3. **cloudflared playbook generates ingress from the registry.** `vars/vars_cloudflared.yaml`'s `tunnels.main.ingress` list is generated from the registry (merged with any non-`web_services` entries, e.g., SSH tunnels). The existing flat list is migrated entry-by-entry.

4. **Migration guardrail — `tests/render_diff.sh`.** Renders both the legacy hand-maintained nginx vhost file and the new registry-driven nginx config, and diffs them. Same for cloudflared. A new PR workflow `pr-render-check.yml` runs `render_diff.sh` on every PR; non-whitespace diffs fail CI. Each migration commit must show "diff: whitespace-only" or "diff: captured in `<service>.nginx_overrides`."

5. **Migration order.** Homepage → grafana → blog.terrac.com (simple cases) → ComfyUI (websocket override case) → Loki (timeout override case) → remaining services. One commit per service. Each commit is independently revertable.

**Phase 2 done when:** every entry in the old `proxy_hostname_web_proxy.conf` and every `ocean`-host entry in `vars_cloudflared.yaml` has been migrated; `render_diff.sh` produces zero non-whitespace deltas; the flat files are deleted; `pr-render-check.yml` is green on master.

### Phase 3 — Reusable callable workflow + auth rework

External repos trigger their own deploys through a single canonical entrypoint owned by homelab.

1. **Vault password → runner mount (prerequisite).**
   - Extend `roles/github_docker_runners` to mount a vault-password file at `/runner/.vault_pass` (sibling to the existing `.github_token` mount), sourced from the same vault-managed file on the host.
   - Update every deploy workflow to read `--vault-password-file=/runner/.vault_pass` instead of writing `$ANSIBLE_VAULT_PASSWORD` to `/tmp/.vault_pass`.
   - After all workflows are migrated, delete the `ANSIBLE_VAULT_PASSWORD` GitHub Actions secret.
   - Rotation runbook: update the file on `gh-runner-01`, restart the runner systemd unit; no GitHub UI involvement.

2. **New: `.github/workflows/deploy-service.yml`** (callable). Trigger: `on: workflow_call`.

   Inputs:
   ```yaml
   service:     { required: false, type: string }
   playbook:    { required: false, type: string }
   ref:         { required: false, type: string }
   force_clean: { required: false, type: boolean, default: false }
   ```
   Behavior:
   - Validate exactly one of `service` / `playbook` is set; fail loud otherwise.
   - If `service`: look up `web_services.<service>.host` in `vars/vars_web_services.yaml` and find the matching playbook under `playbooks/individual/<host>/`. Fail loud if not found.
   - Resolve `target_host` from the playbook's `hosts:` directive. Set `concurrency: deploy-${target_host}`.
   - Run the playbook with `--vault-password-file=/runner/.vault_pass`.
   - Pass `ref` and `force_clean` through as `--extra-vars` (playbooks that don't consume them ignore them).
   - Post a job summary listing the playbook, host, and outcome.

3. **Versioning.** Tag the callable workflow `@v1` once Phase 2 lands. External repos pin to `@v1`. Breaking changes (input renames, behavior changes) ship as `@v2` and `@v1` continues working unchanged. The tag is moved (not rebuilt) only for non-breaking fixes.

4. **External repo's `.github/workflows/deploy.yml` template:**
   ```yaml
   on: { push: { branches: [main, master] } }
   jobs:
     deploy:
       uses: bluefishforsale/homelab/.github/workflows/deploy-service.yml@v1
       with:
         service: <name>
         ref: ${{ github.sha }}
   ```
   No secrets need to be passed from the external repo. The self-hosted runner reads vault + SSH from its local mounts.

5. **Bootstrap script: `scripts/new-web-service.sh`.** Prompts for `{name, image (or git_repo), upstream_port, host_internal, host_external}`. Generates:
   - the service playbook under `playbooks/individual/ocean/services/<name>.yaml` from a Jinja template based on `paia.yaml` (image-based) or `terrac_com_static.yaml` (source-pull) depending on input.
   - the `web_services.<name>` registry entry.
   - the external repo's `.github/workflows/deploy.yml` (printed to stdout for the user to commit in the other repo, or piped to `gh api` to create directly).
   - idempotent: re-running for an existing service shows the diff and asks before overwriting.

6. **Doc:** `docs/setup/adding-a-web-service.md` describes both paths (script-driven and manual).

**Phase 3 done when:** at least two external repos are deploying through the callable workflow with no GitHub secrets exposed to those repos; vault password GitHub secret is deleted; bootstrap script generates a working pair of files in one invocation.

## Cross-cutting

- All deploy workflows: `concurrency: { group: deploy-${target_host}, cancel-in-progress: false }`.
- All deploy workflows after Phase 3.1: vault password via runner mount, not GitHub secret.
- Rollback (`rollback.yml`) unchanged in concept. A `git revert` of a registry entry triggers narrow re-deploy of just the affected service + nginx + cloudflared.
- PR tests:
  - existing `pr-test.yml` (ephemeral VM, full playbook run) — unchanged.
  - new `pr-render-check.yml` (no VM, just renders + diffs nginx and cloudflared configs) — runs on every PR after Phase 2.

## Risk register

| Risk | Mitigation |
|------|------------|
| nginx template miss for a per-service quirk (e.g., missing websocket header) | `render_diff.sh` blocks merge if non-whitespace delta; migrate one service per commit |
| Concurrency host-extraction misreads `hosts:` (e.g., group names like `ocean,dns01`) | Setup step normalizes; multi-host playbooks join sorted host names into the group key |
| Callable workflow `@v1` tag points at a broken commit | Tag is only moved after green run on master; document tag-promotion procedure in `docs/operations/deploy-tracer.md` |
| Vault-password file on runner host gets out of sync with vault contents | Single canonical source — the file on `gh-runner-01` is the vault password used to encrypt `vault/secrets.yaml`; rotation runbook is part of Phase 3.1 deliverable |
| External repo pushes faster than ocean can serialize | `concurrency: deploy-ocean, cancel-in-progress: false` queues them; runners are ephemeral so queue head-of-line blocking is bounded by playbook duration |
| `git diff HEAD~1 HEAD` misses files in non-tip commits of a multi-commit push | Pre-existing; document in tracer runbook that tracer commits MUST be single-commit pushes; out of scope to fix in this design |

## Open questions

None at design time. Decisions captured:

- (A) Vault password → runner mount. External repos pass no homelab secrets.
- (B) Homepage works today. Phase 0 verification skipped per user confirmation.
- (C) Detection narrows per-host. Sledgehammer remains only as fallback for unmatched changes.
