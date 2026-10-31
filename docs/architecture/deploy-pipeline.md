# Deploy pipeline

How a change in this repo, or a dispatch from an external repo, becomes an
`ansible-playbook` run against a production host. It exists so a merge touches
the fewest playbooks and two deploys to one host never overlap. It is phase 1 of
the 2026-05-10 shape-up design (in git history); phases 2 and 3 are not built.

Every workflow runs on the self-hosted pool, labels `[self-hosted, homelab,
ansible]`: four ephemeral containers on gh-runner-01 (192.168.1.20)
(`roles/github_docker_runners/defaults/main.yml:32`,
`inventories/production/hosts.ini:33-34`). Ansible reaches hosts with the
runner host's `/root/.ssh`, mounted read-only
(`roles/github_docker_runners/templates/docker-compose.yml.j2:102-105`).

Which files map to which playbooks, the path filters, and the GitHub API traps
are in [`agents.md`](../../agents.md#how-a-change-maps-to-playbooks-impact-detection).
Onboarding an external repo is [`deploy-pattern.md`](../operations/deploy-pattern.md).

## Workflows

| Workflow | Trigger | What it owns | Concurrency group |
|---|---|---|---|
| `main-apply.yml` | push to master (path filter), dispatch with a playbook list | The only push-time apply. `detect-changes` runs the detector (`:89`) and the resolver (`:95`); `apply-playbooks` runs each playbook and fails if any failed; `verify-deployment` probes after. Dispatch input bypasses the detector and its exclusions (`:70-82`). | resolved per run, job level (`:127-130`) |
| `deploy-paia`, `-mem0`, `-my-ta-jose`, `-etrading-boy`, `-photonic-inventory`, `-terrac-com` | `repository_dispatch` `deploy-<name>`, or dispatch | Shape A/B entry points for external repos: one fixed playbook each. | `deploy-ocean` |
| `deploy-blog-terrac-com.yml` | `repository_dispatch` `deploy-blog-terrac-com` | `ocean/services/blog_terrac_com_static.yaml` (`:83`), an ocean play. | none |
| `deploy-netboot-images.yml` | `repository_dispatch` `deploy-netboot-images` | `infrastructure/netboot_image.yaml` with image and sha. Excluded from push detection. | `deploy-netboot` |
| `deploy-registry-cache.yml` | `repository_dispatch` `deploy-registry-cache` | `infrastructure/registry_cache.yaml`. | `deploy-registry-cache` |
| `deploy-ocean-service.yml` | dispatch, service choice | Validate, `--check`, then apply one mapped playbook. | `deploy-ocean` |
| `deploy-critical-service.yml` | dispatch, `dns` or `plex` | Same with a `critical-services` environment approval (`:151-152`). | `deploy-<service>` |
| `deploy-services.yml` | dispatch, orchestrator choice | Runs `00_site` or `01`/`02`/`03` whole. | none |
| `deploy-changed-services.yml` | dispatch, `base_ref` | Diffs `origin/<base_ref>...HEAD` (`:54`) and applies ocean playbooks. Older parallel detector. | none |
| `rollback.yml` | dispatch | See Rollback. | none |
| `decommission.yml` | dispatch | Report or apply a `playbooks/operations/decommission/<svc>.yaml`, then archive it with a `[skip ci]` push to master. Needs `contents: write`. | `deploy-ocean` |
| `health-check.yml` | dispatch, `workflow_call` | Ocean, DNS and runner checks; exits 1 when unhealthy (`:291`). Nothing calls it today. | none |
| `ci-validate.yml`, `pr-deploy.yml`, `pr-hygiene.yml` | push, PR | Syntax and lint only, plus the detector and resolver tests (`ci-validate.yml:288-294`). No apply. | none |

## Concurrency

GitHub matches concurrency groups as exact strings. main-apply derives its group
with `lib/resolve-concurrency-group.sh`: the first `hosts:` line of each playbook
(`:31-36`), `all` if there is none (`:38`), sorted and joined with `-` (`:48-49`).
So a homepage edit gets `deploy-ocean` and queues behind a `deploy-paia` run,
which is the case phase 1 was built for.

NOTE: anything that is not exactly `deploy-ocean` does not serialize with the
ocean dispatch workflows, even when it lands on ocean:

- a push spanning hosts (`deploy-dns-ocean`, `deploy-all-ocean`);
- group names instead of hosts (`deploy-harbor` and `deploy-data01_workers`
  both mean harbor);
- multi-play playbooks keyed by their first play only (`workers/nzbget.yaml` is
  ocean, then harbor, and resolves to `deploy-ocean`);
- `deploy-critical-service.yml` plex runs (`deploy-plex`);
- the workflows with no group at all: `deploy-blog-terrac-com`, `deploy-services`,
  `deploy-changed-services`, `rollback`. The phase 1 plan deferred the last
  three; blog came later and never got one.

`cancel-in-progress: false` everywhere. A GitHub concurrency group
holds one running and one pending run, and a newer pending run cancels the older
pending one, so three quick dispatches to ocean drop the middle one.

## Vault password

Every workflow that runs Ansible writes `secrets.ANSIBLE_VAULT_PASSWORD` to
`/tmp/.vault_pass` (some use `/tmp/vault_pass`) and deletes it in an `always()`
step (`main-apply.yml:150-156,266-269`). The runner compose mounts the GitHub
token, work dir, action cache, docker socket, `/root/.ssh` and cloudflared, and
no vault file (`docker-compose.yml.j2:80-111`). Rotating the vault password
means updating the GitHub secret.

## Health after a deploy

`verify-deployment` waits 30s, then TCP-probes 192.168.1.143 on 80, 32400, 9090
and 3000 and runs two `dig`s against 192.168.1.2. Targets come from a regex on
playbook paths (`main-apply.yml:97-108`). Both steps are `continue-on-error`
(`:366`, `:412`), and the `runners` target is computed but never checked. For real health see
the [deploy gotchas](../../agents.md#deploy-gotchas-read-before-shipping-a-change).

## Rollback

`rollback.yml` is dry run by default (`:24`) and needs `confirm: ROLLBACK` to go
live (`:48`). It checks out the target commit (default `HEAD~1`) and re-applies
the `playbooks/*.yaml` files that differ between target and HEAD (`:91`). If
none differ it applies all of `03_ocean_services.yaml` (`:95`).

NOTE: it ignores `files/`, `vars/` and `roles/` diffs, so rolling back a
template-only change re-runs 03 instead of the owner. It also leaves master
alone, so the next main-apply puts the bad change back. The durable rollback is
`git revert` on master, which main-apply deploys through the normal detector.

## Never auto-applied

The detector's `emit()` drops these on push, however they are reached
(`lib/detect-impacted-playbooks.sh:104-113`): terminalbench, `*zfs*`,
`operations/backup/**`, `operations/decommission/**`, `archive/**`,
`deprecated/**`, `netboot_image.yaml`, and `github_docker_runners.yaml`, because
main-apply runs on those runners and would recreate itself mid-apply.

## Phase 1 tracers

Run on 2026-05-10 per [`deploy-tracer.md`](../operations/deploy-tracer.md):
`3d48ede` (tracer 1c, homepage play), `74924d2` and `10646b8` (2c, 2d, runners
play), `9149754` (3c, nginx vhost). The markers are still in the files
(`playbooks/individual/ocean/services/gethomepage.yaml:2-4`,
`files/nginx-compose/proxy_hostname_web_proxy.conf:60`). Tracer 2 cannot be
rerun as written: the runners play has since been excluded from push detection.

## Not built

- `vars/vars_web_services.yaml` service registry: absent.
- Generated per-service nginx vhosts: `files/nginx-compose/proxy_hostname_web_proxy.conf` is still one hand-listed file (30 `server_name` lines).
- Cloudflared ingress from a registry: `vars/vars_cloudflared.yaml` is still the hand list.
- `tests/render_diff.sh` and `pr-render-check.yml`: absent (no `tests/` dir).
- Vault password as a runner mount: absent; see Vault password.
- Callable `deploy-service.yml`: absent. The only `workflow_call` is `health-check.yml`.
- `@v1` tag for external repos, `scripts/new-web-service.sh`, `docs/setup/adding-a-web-service.md`: absent. External repos still use one `repository_dispatch` workflow each.

## Backout

The detector and resolver are each one `bash` call in main-apply
(`:89`, `:95`). Replacing the detector with an explicit playbook list, or the
resolver output with a literal `deploy-ocean`, puts back the coarser behaviour
without touching any playbook. Any single deploy can be redone with
`gh workflow run main-apply.yml -f playbooks=<path>`.
