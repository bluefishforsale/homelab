# Homelab Infrastructure: Agent Reference

Ansible-driven homelab managing bare metal, VMs, Docker services, GPU
passthrough, and CI/CD across a multi-host Proxmox cluster.

This file is the only one loaded into every session, so it holds the rules
that apply to any change and an index to the rest. Open the linked file for the
task in front of you before acting on it; the detail and the reasons live there.

## Always true

- **Default branch is `master`.** PRs target it and are squash-merged.
- **Merge to `master` is a production deploy.** `main-apply.yml` runs a real
  `ansible-playbook` apply on push. CI green is lint and syntax only, so watch
  the `main-apply` run and then verify with `scripts/homelab-health.sh`.
- **Never run `ansible-playbook` on the laptop.** Non-mutating runs (`--check`,
  `--syntax-check`) happen on agentbox; mutating applies go through CI.
- **Never automate ZFS.** No committed playbook mutates `zfs` or `zpool`, and a
  drive is always named by WWN, never `/dev/sdX`.
- **Nothing with a write-ahead log or journal goes on NFS.** A service's state
  dir is on a local disk of the host that runs it.
- **One branch and worktree per change.** Never commit to `master`, and stage
  explicit paths, never `git add -A`.
- **Fix through code.** A playbook or workflow change is the fix; a hand edit
  on a host is drift the next apply overwrites.
- **Cloud resources are not created here.** Cloudflare, GCP, Azure and Proxmox
  VMs belong to `terraform-root`.
- **Every service ships monitored**, as a compose project with a health check.
- No `Co-Authored-By` trailers, no em dashes, terse commits.

## Read before you act

| When you are about to | Read |
|---|---|
| Ship any change, or wonder why a deploy did or didn't run | [`docs/agents/deploy.md`](docs/agents/deploy.md) |
| Change `files/`, `vars/`, `roles/` or inventory, or add a service | [`docs/agents/impact-detection.md`](docs/agents/impact-detection.md) |
| Touch disks, ZFS, `/data01`, NFS, or where a service keeps its state | [`docs/agents/storage.md`](docs/agents/storage.md) |
| Need DNS, a tunnel, an Access app, a bucket, or a VM | [`docs/agents/cloud-boundary.md`](docs/agents/cloud-boundary.md) |
| Hit a 403 or an odd result from `gh` or the GitHub API | [`docs/agents/github-traps.md`](docs/agents/github-traps.md) |
| Work alongside another agent, deploy outside CI, or decide who merges | [`docs/agents/coordination.md`](docs/agents/coordination.md) |
| Choose how to build something (monitoring, Ansible style, vault layout, PRDs) | [`docs/agents/preferences.md`](docs/agents/preferences.md) |
| Need the vocabulary, topology, or the two deployment shapes | [`CONTEXT.md`](CONTEXT.md) |
| Diagnose the fleet (metrics, logs, alerts, containers, secrets) | [`scripts/README.md`](scripts/README.md), before hand-rolling `ssh` or `curl` |
| Run a service on a data01 worker such as harbor | [`docs/architecture/data01-workers.md`](docs/architecture/data01-workers.md) |
| Onboard a new external-repo service | [`docs/operations/deploy-pattern.md`](docs/operations/deploy-pattern.md) |
| Look for anything else (setup, architecture, operations, troubleshooting) | [`docs/README.md`](docs/README.md) |

Validate with `make validate`.

When a rule here and a linked file disagree, the linked file is the detail and
this file is the summary, so fix whichever is stale in the same change.
