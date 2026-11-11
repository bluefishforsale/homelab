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
- Inventing a new chart style. The 2021 wrapper charts are upgraded and are
  the style every new app chart follows (D24).

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
| D13 | Ocean runs Docker and k3s side by side until Docker is completely drained. Only then does the first passthrough swap move ocean to a clean Debian 13 k8s-only VM, by end of 2027. | design call, confirmed by terrac |
| D14 | Pods pin to node role labels, never to a hostname, so they follow a replaced VM unchanged. | design call |
| D15 | CNI is Cilium. | terrac |
| D16 | node005 RAM upgrade lands before phase 2 (the harbor VM and its replacement overlap). | terrac |
| D17 | "Cluster is down" is reported by email and ntfy. | terrac |
| D18 | Image updates take two paths: third-party images by a bot that watches registries and opens tag-bump PRs; home-grown images by push then dispatch that updates the chart. | terrac |
| D19 | The dead-man watchdog and ntfy live on agentbox (tier 0); ntfy never moves into the cluster. Email leaves by a relay that does not pass through ocean. | design call, needs terrac's sign-off |
| D20 | North-south traffic uses Cilium's Gateway API in host-network mode on ocean. ingress-nginx (retired March 2026), MetalLB and the tunnel ingress controller are not used. | design call |
| D21 | Third-party update PRs: patch and minor for stateless apps auto-merge in the Sunday window; majors, stateful apps and platform charts wait for terrac. Renovate self-hosted on the runner. | design call, needs terrac's sign-off |
| D22 | Ocean is excluded from kured; it reboots only in the supervised monthly window. | design call |
| D23 | Shape B static sites become small nginx images and use the home-grown path. | design call, needs terrac's sign-off |
| D24 | The 2021 app charts in `homelab-kube` are upgraded, not replaced, and are the style basis for all app charts. Their dead `k8s-at-home` dependency moves to `bjw-s-labs/app-template`, the maintained descendant of the same library. | terrac (upgrade, style), design call (which dependency) |

## Open questions for terrac

- **Q5.** Weekly reboot window for kured: proposal Tuesday 03:00 to 05:00
  Pacific (clear of the Sunday image refresh).
- **Q6.** Internal TLS in the cluster. The `homelab-ca` root (never expires,
  no revocation, `pathlen:0`, names limited to `home`, `cluster.local` and
  192.168.1.0/24) cannot sign an intermediate, so cert-manager could only use
  it by holding the root key in the cluster. Options: (a) change the CA design
  before it is built, to `pathlen:1` with a cluster intermediate that can be
  replaced; (b) keep it for hosts only and give the cluster its own CA;
  (c) no internal TLS in the cluster for 2027. Recommendation: (a), because
  the CA is not implemented yet and this is the cheap moment to change it.
- **Q7.** Is Salt still worth it? After this migration, services are in
  Kubernetes and VMs are in `terraform-root`. What is left for a host tool is
  small: base packages, users, exporters, the k3s install, NFS and drivers, on
  about a dozen hosts. Salt adds a master VM, a minion on every host, a new
  repo and a new CI path for that. The alternative is a slimmed-down Ansible
  in a renamed repo. Salt stays in the plan as decided; this asks for a
  deliberate yes before phase 6 work starts.
- **Q8.** `avahi-daemon`, `netatalk` and `glances` run on ocean outside any
  container: keep on the host or drop.

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
  values, secrets) and one `homelab` PR (retire play, files, port, vault keys,
  timers), preceded where needed by a `terraform-root` PR (tunnel rule, bucket)
  and a PR in the service's own repo (dispatch target).
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
  first. Ocean is excluded (D22).
- **R15.** Every image the cluster runs is pinned to an immutable tag or
  digest. Nothing runs `:latest`. Tags change only by commit to `homelab-kube`.
- **R16.** Metrics and logs for a migrated service keep flowing to the existing
  Prometheus and Loki from the day it moves (in-cluster agent with remote
  write), not from phase 5.
- **R17.** No pod starts on ocean unless `/data01` is a mountpoint.
- **R18.** Every host unit and timer on ocean has a named destination before
  phase 4 starts (spec table), not only the containers.
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
| 0 | `homelab-kube` defects fixed (Retain, allowEmpty, age), Q5, Q6 and Q8 answered, D19, D21 and D23 signed off. |
| 1 | Three upgrades completed with Argo CD and three test apps running; the third upgrade needs no step outside the play. One deliberate restore from backup onto a fresh server succeeds. 14-day soak with no unplanned API outage. |
| 1b | Patch pipeline live on the new VMs: unattended-upgrades, kured, update and reboot metrics and alerts. |
| 2 | node005 RAM upgrade done before work starts. harbor VM workloads (nzbget, tdarr-node, the nightly DB restore test) run on `k3s-agent01`; harbor VM decommissioned. |
| 3 | Docker and k3s coexist on ocean through a reboot with no Docker service impact; GPU time-slicing runs llama.cpp plus an NVENC transcode in pods; NVENC session cap measured and recorded. |
| 4 | Every Docker service on ocean is a pod or explicitly parked. |
| 5 | Stopping the k3s API produces both an email and an ntfy push from agentbox (tested before Alertmanager moves). Prometheus, Loki, Grafana and Alertmanager run in cluster. Docker nginx retired; the Cilium Gateway owns 80/443 on ocean. Docker removed from ocean. |
| 5b | Passthrough swap rehearsed on `k3s-agent01`, then, with Docker drained, ocean swapped to a clean Debian 13 k8s-only VM; old VM deleted after 14 days with no backout. |
| 6 | Every host is a Salt minion; every Ansible role and play is deleted or ported. |
| 7 | Every vault key has a new home (including the cluster age key and the Access email lists `terraform-root` reads). `homelab` archived. |

## Success measures

- Onboarding a new service touches one repo and takes no new template, port
  entry or firewall rule.
- Moving a stateless service between nodes is a values change.
- A minor-version k3s upgrade is a PR plus one dispatch, with app downtime of
  zero for stateless apps.
