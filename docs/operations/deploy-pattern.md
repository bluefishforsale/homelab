# Homelab CI/CD pattern — external repo → deploy on ocean

Reference for adding a new service that ships from its own GitHub repo and
deploys onto ocean (or another homelab host) automatically on push.

## Data flow

```
bluefishforsale/<service>             bluefishforsale/homelab          ocean (192.168.1.143)
─────────────────────────             ───────────────────────          ─────────────────────
push main                             repository_dispatch              docker pull + systemd restart
   │                                  ↓                                   │
   ▼                                  deploy-<service>.yml                ▼
.github/workflows/build.yml           (concurrency: deploy-ocean)      ghcr.io/.../<service>:latest
   │                                  ↓                                   │
   ├─ build (image OR static)         ansible-playbook                    ▼
   ├─ push artifact (GHCR / git)      individual/ocean/services/...    nginx vhost
   └─ curl POST /homelab/dispatches      │                                │
      (jq -cn --arg payload)            ▼                                ▼
                                    self-hosted runner               cloudflared tunnel
                                    (/root/.ssh + cloudflared           │
                                     binary mounted)                    ▼
                                                                     https://<service>.terrac.com
```

## Two deploy archetypes

| | Image-based (paia, my-ta-jose) | Source-pull (terrac.com, blog.terrac.com) |
|---|---|---|
| External repo builds | Docker image → GHCR | static `dist/` or `public/` → committed back to main |
| Homelab playbook | `docker login ghcr.io`, compose pull, systemd restart | git clone, rsync `public/` → web_root |
| Runtime on ocean | container on private port (8090–8095) | files under `/data01/services/<svc>/` served by nginx |
| Refresh trigger | systemd restart pulls `:latest` | git clone replaces files |

## External repo side (`bluefishforsale/<service>`)

**Files:**
- `Dockerfile` *(image-based)* OR build script + `dist/` *(source-pull)*
- `.github/workflows/build.yml` — push trigger, paths-ignore docs, optionally
  Dockerfile-gated so the workflow is a no-op until the build artifact exists

**Secret:** `HOMELAB_DISPATCH_TOKEN` set via `gh secret set --repo bluefishforsale/<service>`,
value pulled from the homelab vault key `development.github.deploy_dispatch_token`
(fine-grained PAT with `Contents: write` on `bluefishforsale/homelab`).

**Dispatch payload** — always built with `jq -cn --arg`, never string-concatenated:

```bash
PAYLOAD=$(jq -cn --arg sha "$COMMIT_SHA" --arg msg "$COMMIT_MESSAGE" \
  '{event_type:"deploy-<service>",client_payload:{branch:"main",sha:$sha,commit_message:$msg}}')
curl -f -X POST -H "Authorization: Bearer $HOMELAB_DISPATCH_TOKEN" \
  -H "Accept: application/vnd.github+json" \
  https://api.github.com/repos/bluefishforsale/homelab/dispatches -d "$PAYLOAD"
```

## Homelab repo side (`bluefishforsale/homelab`)

Five files per new service:

1. `playbooks/individual/ocean/services/<service>.yaml` — ansible playbook.
   For image-based: templates compose + systemd, ends with an **unconditional**
   `systemd: state: restarted` (so floating `:latest` tags get pulled on every run).
2. `files/<service>/{docker-compose.yml.j2, .env.j2, <service>.service.j2}` — templates.
3. `.github/workflows/deploy-<service>.yml` — listens for
   `repository_dispatch: deploy-<service>`, runs the playbook.
4. Routing — three coordinated entries:
   - `vars/vars_service_ports.yaml`: `<service>.port: 8xxx`
   - `files/nginx-compose/proxy_hostname_web_proxy.conf`: vhost
     `<service>.terrac.com` + `<service>.home` → `proxy_pass http://172.17.0.1:8xxx`
   - `vars/vars_cloudflared.yaml`: ingress
     `<service>.terrac.com` → `http://192.168.1.143:80`
5. `playbooks/individual/ocean/network/cloudflared.yaml`: add the first label
   of the hostname to `fully_public_services` (Access bypass) for public
   services. `public_services` = admin + plex-users gate. Else admin-only.

**Workflow conventions for `.github/workflows/deploy-<service>.yml`:**

- `concurrency: deploy-ocean` (or per-host) with `cancel-in-progress: false` —
  same-host deploys serialize, different-host deploys run in parallel.
- `environment: Github Actions CI` — gates on the GH environment for secret access.
- `runs-on: [self-hosted, homelab, ansible]` — runs on gh-runner-01's ephemeral
  containers, which mount `/root/.ssh` and `/usr/local/bin/cloudflared` from the host.
- **Step-level `env:`** to receive `${{ github.event.client_payload.* }}` — never
  inline expansion in `run: |` heredocs (see Gotcha 2).
- **`${PIPESTATUS[0]}`** to capture ansible's exit code through `| tee` (Gotcha 1).
- Truncate `commit_message` to first line with `head -1` before
  `>> $GITHUB_OUTPUT` (Gotcha 3).

## Hostname / port conventions

| | Convention |
|---|---|
| External hostname | `<service>.terrac.com` (Cloudflare zone, routed via tunnel) |
| Internal hostname | `<service>.home` (BIND on dns01, no tunnel) |
| Container | `container_name: <service>` |
| On-disk state | `/data01/services/<service>/` |
| systemd unit | `<service>.service` |
| Host port | Free port in 8090–8095 range; reserved in `vars/vars_service_ports.yaml` |

## Auth model

No GitHub secret crosses a repo boundary — every external repo has only the
dispatch PAT, never the vault password.

| Secret | Where it lives | Used by |
|---|---|---|
| `ANSIBLE_VAULT_PASSWORD` | GH secret on `bluefishforsale/homelab` only | self-hosted runner → ansible-playbook |
| `HOMELAB_DISPATCH_TOKEN` (= vault `deploy_dispatch_token`) | GH secret on each external repo; canonical value in vault | external CI → POST `/homelab/dispatches` |
| Runner's SSH ed25519 key | `/home/github-runner/.ssh/` on gh-runner-01; pubkey published as `bluefishforsale.keys` | runner container → ssh to ocean/dns01/etc. as `ansible_user` |
| `packages_read_token` (vault) | vault `development.github.packages_read_token` | ocean → docker login ghcr.io |

When the runner's SSH key rotates: replace the `gh-runner-01` entry on
bluefishforsale's GitHub keys, then run `playbooks/individual/base/authorized_keys.yaml`
from laptop via ProxyJump to sync every target's `authorized_keys`. See
`reference_runner_ssh_key_sync.md` in the auto-memory dir for the exact commands.

## Gotchas (each one was a real bug)

1. **`ansible-playbook | tee` masks the exit code** — the `if` sees `tee`'s
   exit (always 0), not ansible's. Use `${PIPESTATUS[0]}` immediately after the
   pipeline, then `if [ "$ANSIBLE_EXIT" -eq 0 ]; then`. Affects `main-apply.yml`
   and `pr-test.yml`.
2. **`${{ github.event.client_payload.commit_message }}` inlined into bash is
   shell injection.** A commit body with `{"detail":"Not Found"}` was executed
   as `Found} JSON. Replace ...` Use step-level `env:` to pass payload, then
   reference as `$CLIENT_COMMIT_MESSAGE` in bash — special chars become data.
3. **`echo "name=$MSG" >> $GITHUB_OUTPUT` breaks on newlines.** GITHUB_OUTPUT
   expects `name=value\n`; a multi-line commit message produces
   `name=line1\nline2\n` which is invalid format. Truncate with
   `$(printf '%s' "$MSG" | head -1)` before writing.
4. **`jq -cn --arg`** for dispatch JSON, never `-d "{...${{ ... }}...}"`
   string concatenation — same newline/quote problem as Gotcha 2.
5. **Image-based playbooks must unconditionally restart the systemd unit**;
   otherwise an unchanged compose template → no handler fires → systemd unit
   keeps the old image. The unit's `ExecStartPre=docker compose pull` only runs
   on restart.
6. **The cloudflared playbook needs the CLI on the controller.** The
   `github_docker_runners` role installs `cloudflared` at
   `/usr/local/bin/cloudflared` on gh-runner-01 and bind-mounts it into each
   runner container; without it, all `delegate_to: localhost` cloudflared tasks
   fail in CI.
7. **First label of hostname drives Access policy.** The classifier in
   `cloudflared.yaml` splits `host.zone.tld` on `.` and matches the first
   segment against `fully_public_services` / `public_services`. So `blog.X` and
   `blog.Y` are both public. Adding a new public service = add its first label
   to `fully_public_services`.
8. **`run_once: true` on tasks that register a per-host fact leaks across
   hosts.** The cloudflared SSH-tunnel-check task had it; one host's positive
   result made all non-ocean hosts try `cloudflared tunnel info ssh-<host>` for
   tunnels that didn't exist. Drop `run_once` for register-into-fact tasks.

## Bootstrap checklist for a new service

```text
[ ] Decide hostname (<name>.terrac.com) and host port (next free 80xx)

homelab:
[ ] playbooks/individual/ocean/services/<name>.yaml      (clone paia.yaml or terrac_com.yaml)
[ ] files/<name>/{docker-compose.yml.j2, .env.j2, .service.j2}
[ ] vars/vars_service_ports.yaml — add <name>.port
[ ] files/nginx-compose/proxy_hostname_web_proxy.conf — add vhost block
[ ] vars/vars_cloudflared.yaml — add ingress entry
[ ] playbooks/individual/ocean/network/cloudflared.yaml — add to fully_public_services if public
[ ] .github/workflows/deploy-<name>.yml (clone deploy-my-ta-jose.yml)
[ ] .github/workflows/deploy-ocean-service.yml — add to manual-dispatch enum
[ ] Push homelab — main-apply runs the affected narrow set.

external repo:
[ ] .github/workflows/build.yml (clone bluefishforsale/my-ta-jose's)
[ ] gh secret set HOMELAB_DISPATCH_TOKEN (value from vault deploy_dispatch_token)
[ ] Push initial code → CI builds → dispatches → ocean deploys.

[ ] Visit https://<name>.terrac.com
```
