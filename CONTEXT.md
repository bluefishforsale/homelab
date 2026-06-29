# CONTEXT

Orientation and shared vocabulary for the homelab repo. Read this before the
code; read `agents.md` and `docs/architecture/deployment-flow.md` for the
deeper mechanics.

## What this repo is

`homelab/` is the master configuration repo for the fleet. Everything is
Ansible: playbooks under `playbooks/`, reusable roles under `roles/`, host
inventory under `inventories/production/`, rendered config templates under
`files/<service>/`, and shared variables under `vars/`. Secrets live in
`vault/secrets.yaml`, ansible-vault encrypted, decrypted with the password file
at `~/.ansible_vault_pass`.

The laptop is dev/test only. **Never run `ansible-playbook` locally.** Changes
reach hosts through CI and a self-hosted runner inside the homelab network.

## Topology

- **node005 / node006** - bare-metal Proxmox hypervisors.
- **ocean** - the workhorse VM on node006, RTX 3090. Runs the monitoring stack
  (Prometheus, Loki, Grafana), the local inference server (`llama.cpp` behind
  `llama.home`), and most container services.
- **VMs** - dns01/02, registry-cache, gh-runner, agentbox, etc. All VMs are in
  the `[vms]` inventory group, which feeds the fleet-wide loops (node-exporter,
  cadvisor, promtail scrape targets).

## Deployment model

A deployable project is an external repo (not this one) that ships a service to
a homelab host. Two shapes:

- **Shape A - container service.** CI builds a Docker image, pushes to
  `ghcr.io/bluefishforsale/<name>`, then `repository_dispatch`es to this repo.
  The runner runs the matching playbook, which pulls `:latest` and recreates the
  compose service.
- **Shape B - static site.** CI builds static assets, commits them back, and
  dispatches. The playbook git-clones the project on the target host; nginx
  serves the built output directly. No image, no GHCR.

The target host is encoded by the playbook path here, not by the project. A
push to this repo auto-deploys only on `playbooks/**` changes; `files/**` and
`vars/**` changes (dashboards, prometheus config, model definitions) need a
manual `gh workflow run`. A merge to a deployable repo's default branch, or to
this repo's master, is a production change, not a checkpoint.

## The agent fleet (agentbox)

`agentbox` is an always-on VM that watches the deployable repos' open issues and
resolves them, while staying drivable live from the phone. Its design and the
fixed-cost constraints are in the plan; the autonomy policy is
[ADR 0001](docs/adr/0001-tiered-agent-autonomy.md). Vocabulary:

- **Lane** - a path that work takes through a model tier. The **premium lane**
  is Claude Code on the Max subscription (sanctioned, fixed-cost). The **free
  lanes** are opencode routed to Gemini (drafting) and the local GPU (triage).
  Lanes are labelled on telemetry (`lane=free|claude`) so cost breaks down by
  tier.
- **Drafter** - the opencode `build` agent that writes the actual fix on the
  free lane. Routed to Gemini 2.5 Flash because the draft has to be a mergeable
  diff; the local model handles triage and bulk, not drafting.
- **Escalation** - handing an issue from the free lane to the premium lane
  (relabel `needs-claude`). Failure-driven: it happens when the draft yields no
  usable diff, or the opened PR's CI goes red. Not predicted from "complexity."
- **RC session** - a persistent `claude remote-control` process for one repo,
  run as a systemd template instance (`agentbox-rc@<repo>.service`, cwd
  `repos/<repo>`), drivable live from the Android app or desktop. One per repo
  in the curated interactive set; idle sessions hold a process but spend almost
  nothing until driven.
- **Deployable repo** - see the deployment model above. The watcher runs against
  all of them; RC sessions run only for the few actively driven.

## Conventions

- Service ports are centralized in `vars/vars_service_ports.yaml`.
- A host joins fleet-wide monitoring by being in `[vms]` (or `groups['all']`);
  the Prometheus loops pick it up by hostname.
- No em dashes, terse commits, no generated-by tag lines.
