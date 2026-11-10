# PRD: 2027 platform migration (k3s, Argo CD, Salt)

Date: 2026-11-09
Status: draft, awaiting review
Design: `2026-11-09-k8s-salt-migration-design.md`
Roadmap: `../plans/2026-11-09-k8s-salt-migration-roadmap.md`

## Problem

Standing up, securing, and moving a service takes a template, a playbook, a
port entry, vault plumbing, firewall rules and a hand-managed cert. Moving a
service between hosts means rewriting its play and migrating its state by hand.
Host configuration and service orchestration are tangled in one Ansible repo
where merge is deploy.

## Goals

- **G1.** Every container service on the fleet runs on one k3s cluster,
  deployed by Argo CD from Helm charts in `homelab-kube`, onboarded one at a
  time through an app-of-apps.
- **G2.** A new or moved service needs only a chart and a values file:
  secrets, TLS, ingress, network policy and monitoring come from the platform.
- **G3.** Kubernetes upgrades are controlled: pinned version, deliberate
  dispatch, rehearsed procedure, tested restore.
- **G4.** Every host is configured by Salt, and the `homelab` repo is archived.
- **G5.** No fleet-wide outage at any point. Downtime is per service, planned,
  and short.
- **G6.** The fleet never falls behind on CVE patches or OS majors again:
  security patches roll continuously, and every VM is on a supported Debian
  major, reached by replacing the VM, not upgrading it in place.

## Non-goals (not in 2027)

- HA control plane. One k3s server; the API may be down for minutes during an
  upgrade or a server reboot while workloads keep running.
- Multi-cluster or a separate management cluster.
- Replacing the DNS pair, DHCP, Proxmox, or the ZFS pool layout.
- Moving `/data01` off ocean, or replacing NFS with a distributed storage
  system (Ceph, Longhorn).
- Rewriting app charts that work; use upstream charts where they exist.

## Anti-goals (failure even if it technically works)

- **A1.** Anything in tier 0 depends on the cluster being up (Salt master,
  DNS, registry cache, the k3s server's backup, the dead-man alert).
- **A2.** Any database or SQLite file ends up on NFS.
- **A3.** A service runs as a Docker container and a pod at the same time
  against the same state.
- **A4.** A Kubernetes upgrade happens as a side effect of a merge.
- **A5.** A secret lives in both the vault and sops after its Ansible consumer
  is gone, or anywhere in plaintext in git.
- **A6.** A host concern managed by both Ansible and Salt at once.
- **A7.** Any DNS record, resolver, sshd, `authorized_keys` or port 22 change
  made by an agent as part of this work.
- **A8.** A service migrated without its health check, blackbox probe and
  metrics scrape (the "every service is monitored" rule).
- **A9.** An in-place OS major upgrade (`apt full-upgrade` across a release,
  `do-release-upgrade`) on any VM. Hypervisors are the only exception.
- **A10.** `zpool export`, `zpool import` or any other mutating ZFS verb in a
  committed, runnable play. The passthrough swap's ZFS steps are hand-run.
- **A11.** Two VMs configured to own the same passthrough device, or the old
  passthrough VM deleted before its 14-day backout period ends.
- **A12.** A kernel reboot on a ZFS or NVIDIA host before `dkms status` shows
  both modules built for the new kernel.

## Decisions

| # | Decision | Made by |
|---|---|---|
| D1 | k3s with SQLite via kine. No etcd. | terrac |
| D2 | One cluster. No management cluster; tier 0 VMs cover the break-glass role. | design call, signed off by terrac |
| D3 | Cluster installed and upgraded by Ansible until proven, then by Salt. | terrac |
| D4 | Ocean joins as the GPU agent in phase 3 and runs Docker and k3s side by side for the whole migration; services move one at a time (least downtime per service). | design call, signed off by terrac |
| D5 | New small server VM on node006; new agent VM on node005 replaces the harbor VM. Both defined in `terraform-root`. | terrac (server VM), design call (server is not ocean) |
| D6 | Argo CD, Helm charts, app-of-apps, in `homelab-kube` brought up to date. | terrac |
| D7 | Salt master outside the cluster, on its own VM. | design call (resolves chicken-and-egg) |
| D8 | Upgrade drill: start three minors behind stable, upgrade three times with Argo CD and test apps running, before any real service moves. | terrac |
| D9 | `/data01` reaches the cluster via NFS from the ZFS passthrough on ocean, as today. | terrac |
| D10 | No new IP ranges and no DNS changes; ocean's IP stays the front door. | design call |
| D11 | OS majors by VM replacement only. Patches and package upgrades continuous, for CVEs. | terrac |
| D12 | Passthrough VMs (ocean, `k3s-agent01`) are replaced by the passthrough swap: build and patch a new VM, stop both, move the devices, start the new one, pods reschedule onto it. | terrac (pattern), design call (details) |
| D13 | Ocean's passthrough swap does not wait for Docker to be empty. The replacement VM carries both Docker (every service not yet migrated) and k3s, so the migration continues across the swap. First swap to Debian 13 after the phase 3 join is proven, by end of 2027. | terrac (both runtimes), design call (timing) |
| D14 | Pods pin to node role labels, never to a hostname, so they follow a replaced VM unchanged. | design call |
| D15 | CNI is Cilium. | terrac |
| D16 | node005 RAM upgrade lands before phase 2 (the harbor VM and its replacement overlap). | terrac |

## Open questions for terrac

- **Q3.** The dead-man alert target outside ocean and the cluster:
  healthchecks.io (new external service) or a check on agentbox.
- **Q4.** Do the old 2021 app charts in `homelab-kube` get reused, or replaced
  by upstream/community charts per app (recommendation: replace, case by case)?
- **Q5.** Weekly reboot window for kured: proposal Tuesday 03:00 to 05:00
  Pacific (clear of the Sunday image refresh).

## Requirements

- **R1.** k3s version pinned in one variable; install and upgrade plays are
  on the never-auto-apply list.
- **R2.** k3s server state (SQLite DB, token, `server/tls`) backed up nightly
  to GCS; restore rehearsed in phase 1 and documented as a runbook.
- **R3.** Argo CD manages itself and the app-of-apps root from `homelab-kube`
  `master`. Sync policy `allowEmpty: false`. Every `/data01` and state PV is
  `Retain`.
- **R4.** All image pulls go through registry-cache-01.
- **R5.** NVIDIA device plugin with time-slicing on ocean; at least Plex,
  Tdarr and llama.cpp schedule onto the GPU together.
- **R6.** Every migrated service has a default-deny NetworkPolicy plus explicit
  allows, a health check, a blackbox probe, and a metrics scrape if it exposes
  metrics.
- **R7.** One service per cutover. Each cutover is one `homelab-kube` PR (chart,
  values, secrets) and one `homelab` PR (retire play, files, port, vault keys).
- **R8.** Upgrade log: each upgrade records add-on compatibility checked,
  removed APIs found, what broke, what the play missed, API downtime and app
  downtime.
- **R9.** Salt states applied through CI with drift reported by a scheduled
  `test=True` run. No scheduled highstate.
- **R10.** `unattended-upgrades` (security origin) on every VM except the DNS
  pair, which terrac patches by hand; NVIDIA, ZFS and
  kernel packages held on passthrough and module-building hosts, and moved in
  a supervised monthly window.
- **R11.** kured reboots k8s nodes one at a time in the agreed window, draining
  first.
- **R12.** Pending-security-update and reboot-required metrics on every host,
  alerting at 7 and 14 days.
- **R13.** Every VM on a Debian major within 6 months of its release and before
  the previous major's regular security support ends. The Proxmox template is
  rebuilt per Debian release.
- **R14.** The passthrough swap is a written runbook, rehearsed first on
  `k3s-agent01` (passed-through scratch disk) before it is used on ocean.

## Acceptance (per phase gate)

| Phase | Gate |
|---|---|
| 0 | `homelab-kube` defects fixed (Retain, allowEmpty, age), Q3 to Q5 answered. |
| 1 | Three upgrades completed with Argo CD and three test apps running; the third upgrade needs no step outside the play. One deliberate restore from backup onto a fresh server succeeds. 14-day soak with no unplanned API outage. |
| 1b | Patch pipeline live on the new VMs: unattended-upgrades, kured, update and reboot metrics and alerts. |
| 2 | node005 RAM upgrade done before work starts. harbor VM workloads (nzbget, tdarr-node) run on `k3s-agent01`; harbor VM decommissioned. |
| 3 | Docker and k3s coexist on ocean through a reboot with no Docker service impact; GPU time-slicing runs llama.cpp plus an NVENC transcode in pods; NVENC session cap measured and recorded. |
| 4 | Every Docker service on ocean is a pod or explicitly parked. Docker nginx retired; ingress owns 80/443 on ocean. |
| 5 | Prometheus, Loki, Grafana and Alertmanager run in cluster; the external dead-man alert fires when the cluster is down (tested). |
| 5b | Passthrough swap rehearsed on `k3s-agent01`, then ocean swapped to a Debian 13 VM carrying Docker and k3s, every service healthy afterwards; old VM deleted after 14 days with no backout. |
| 6 | Every host is a Salt minion; every Ansible role and play is deleted or ported. |
| 7 | `homelab` archived. |

## Success measures

- Onboarding a new service touches one repo and takes no new template, port
  entry or firewall rule.
- Moving a stateless service between nodes is a values change.
- A minor-version k3s upgrade is a PR plus one dispatch, with app downtime of
  zero for stateless apps.
