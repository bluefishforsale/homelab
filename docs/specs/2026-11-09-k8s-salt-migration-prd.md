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
- An off-site copy of `/data01`. RAIDZ2 on the HBA is its only protection;
  everything else needed to rebuild is in git or GCS.
- Inventing a new chart style. The 2021 wrapper charts are upgraded and are
  the style every new app chart follows (D24).

## Anti-goals (failure even if it technically works)

- **A1.** Anything in tier 0 depends on the cluster being up (DNS, registry
  cache, the k3s server's backup, the dead-man alert), or a host's Salt apply
  depends on anything but its own checkout and pillar.
- **A2.** Any database or SQLite file ends up on NFS.
- **A3.** A service runs as a Docker container and a pod at the same time
  against the same state.
- **A4.** A Kubernetes upgrade happens as a side effect of a merge.
- **A5.** A secret lives in both the vault and sops after its Ansible consumer
  is gone, or anywhere in plaintext in git.
- **A6.** A host concern managed by both Ansible and Salt at once.
- **A7.** A DNS, DHCP or resolver change that reaches a host without terrac's
  merge, outside CI, or without passing the pair gates; both DNS servers
  changed in one step. Any sshd, `authorized_keys` or port 22 change made by an
  agent as part of this work.
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
| D7 | Salt runs masterless: `salt-call --local` on each host from a checkout of `homelab-salt`, driven by CI over SSH. No master VM, no minion daemon. Supersedes the 2026-11-09 `salt01` master design. | terrac (2026-11-14) |
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
| D25 | DNS and DHCP are Salt states applied by CI, not by hand: agents may author changes, terrac is the only merger, and every apply goes through the pair gates (R9a). sshd, `authorized_keys`, the fleet key and port 22 stay terrac-by-hand. | terrac (2026-11-14) |
| D26 | VM root disks are cattle: no VM-disk backups; every VM rebuilds from git, GCS and `/data01`, proven by the rebuild drill. A VM that fails the drill gets a weekly vzdump until it passes. | design call, needs terrac's sign-off |
| D27 | Both hypervisors: VM disks on a ZFS mirror `vmpool` separate from boot, and a mirrored boot disk. node006's VM storage moves first, in 2026, not 2027. | terrac asked for every rebuild gap closed (2026-11-14); shape is a design call |
| D28 | Each hypervisor's `/var/lib/pve-cluster/config.db` backed up nightly to GCS, age-encrypted; restoring it is how surviving VM disks get reattached after a reinstall. | design call |
| D29 | One Salt bootstrap script for every host: by hand on bare metal, from cloud-init on VMs, from an Ansible play during the migration. | design call |
| D30 | The VM base image is stock Debian genericcloud, pinned by hash, mirrored to GCS, downloaded per node by `terraform-root`. No custom template build. | design call |
| D31 | Pods with node-local databases restore through a per-chart restore Job: manual, dry-run by default, never auto-synced. | design call |
| D32 | An offline recovery age key is a recipient on every sops file in every repo, with a quarterly decrypt test. | design call, needs terrac's sign-off |
| D33 | The DNS pair gate has a `rebuild_peer` dispatch mode, approved only by terrac, that rebuilds a dead peer without touching the survivor. | design call |
| D34 | Salt owns the hypervisors, passthrough and HBA import guards included. Their network bridges are Salt-rendered but applied only by terrac with the iDRAC console open; CI runs them `test=True`. | design call |
| D35 | Every Postgres database runs as a single-instance CloudNativePG cluster with continuous WAL archiving to GCS: RPO about five minutes, 14 days of point-in-time recovery. The nightly logical dump stays. No Postgres replicas. | terrac (2026-11-15); shape is a design call |

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
  about a dozen hosts. Masterless (D7) drops the master VM, but it also drops
  most of what Salt has over Ansible (mine, reactor, event bus, central
  targeting): CI SSHing in to run a local apply is Ansible's model with a
  different DSL. What masterless Salt still adds is a local apply that works
  with CI and the network down, and a cheap per-host `test=True` drift timer.
  The alternative is a slimmed-down Ansible in a renamed repo plus a `--check`
  drift timer. Salt stays in the plan as decided; this asks for a deliberate
  yes before phase 6 work starts.
- **Q8.** `avahi-daemon`, `netatalk` and `glances` run on ocean outside any
  container: keep on the host or drop.
- **Q10.** A third resolver off the hypervisors. With dns01, dns02 and pihole
  all VMs on node005 and node006, losing both hosts takes DNS away from the
  whole house until node005 is back. Option: the Raspberry Pi
  (`raspberry_pi.yaml` exists) as a third DHCP-advertised resolver, a
  recursor with a copy of the `.home` zone rendered from the same address
  table, managed by Salt like the pair. Recommendation: yes; it is cheap, and
  it is the only item here that keeps the house online during the outage
  itself rather than shortening it.
- **Q9.** Masterless pillar secrets: (a) each host holds its own age key and
  decrypts only the pillar encrypted to it; (b) hosts hold no key and CI
  decrypts and pushes just that host's pillar to tmpfs for each run.
  Recommendation: (a), because the drift timer and a break-glass local apply
  both need pillar on the host when CI is not there; (b) keeps keys off hosts
  but means secret-bearing states cannot be drift-checked or applied by hand.

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
- **R9.** Salt runs masterless. States applied through CI (`salt-call --local`
  over SSH at the merged SHA); drift reported by a per-host `test=True` timer
  as a node-exporter metric with an alert. No scheduled highstate. A host
  decrypts only the pillar encrypted to its own key. CI ships the tree to the
  host by rsync at the SHA; hosts never fetch from GitHub.
- **R9a.** DNS and DHCP (PowerDNS, recursor, dnsdist, zones, Kea DHCP4 and
  DDNS, reservations, netboot.xyz on the pair, pihole, fleet resolver config)
  are Salt states applied by CI in the order and with the gates in the
  design's "DNS and DHCP under Salt": offline config validation, pair
  pre-flight, standby first, per-host gate, primary, pair gate, resolvers
  canary first and runner last. Halt and alert on any failure; no automatic
  rollback. terrac is the required reviewer on those paths. Host, IP, MAC,
  reservation and DNS record render from one address table that CI checks
  against `terraform-root`'s VM MACs.
- **R10.** `unattended-upgrades` (security origin) on every VM; the DNS pair
  reboots one at a time through the pair gates, never both in a window run;
  NVIDIA, ZFS and
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
- **R19.** The fleet rebuilds from git plus GCS plus `/data01`, in the order in
  the design's "Disaster recovery" section, with no Ansible. Every input
  outside git (age keys, Terraform state, DB dumps, Postgres WAL archives,
  the k3s backup, the Proxmox config DBs, the Salt `.deb`, the Debian image) has a named home and
  a freshness alert or test.
- **R20.** No hypervisor keeps VM disks on its boot disk, and every boot disk
  is mirrored (D27).
- **R21.** A reinstalled node006 never imports `/data01` on the host: HBA bound
  to vfio-pci before `mpt3sas`, pool auto-import off (D34).
- **R22.** Every Postgres database archives WAL to GCS (D35). An alert fires
  when the newest archived segment is older than 15 minutes or the newest base
  backup is older than 8 days, and a monthly recovery into a scratch cluster
  proves the archive restores.

## Acceptance (per phase gate)

| Phase | Gate |
|---|---|
| 0 | `homelab-kube` defects fixed (Retain, allowEmpty, age), Q5, Q6, Q8, Q9 and Q10 answered, D19, D21, D23, D26 and D32 signed off. node006's VM disks off its boot disk (D27), the config DB backup live on both hypervisors (D28), and the recovery age key in place (D32). |
| 1 | Three upgrades completed with Argo CD and three test apps running; the third upgrade needs no step outside the play. One deliberate restore from backup onto a fresh server succeeds. 14-day soak with no unplanned API outage. |
| 1b | Patch pipeline live on the new VMs: unattended-upgrades, kured, update and reboot metrics and alerts. |
| 2 | node005 RAM upgrade done before work starts. harbor VM workloads (nzbget, tdarr-node, the nightly DB restore test) run on `k3s-agent01`; harbor VM decommissioned. |
| 3 | Docker and k3s coexist on ocean through a reboot with no Docker service impact; GPU time-slicing runs llama.cpp plus an NVENC transcode in pods; NVENC session cap measured and recorded. |
| 4 | Every Docker service on ocean is a pod or explicitly parked. |
| 5 | Stopping the k3s API produces both an email and an ntfy push from agentbox (tested before Alertmanager moves). Prometheus, Loki, Grafana and Alertmanager run in cluster. Docker nginx retired; the Cilium Gateway owns 80/443 on ocean. Docker removed from ocean. |
| 5b | Passthrough swap rehearsed on `k3s-agent01`, then, with Docker drained, ocean swapped to a clean Debian 13 k8s-only VM; old VM deleted after 14 days with no backout. |
| 6 | Every Linux host applies masterless from `homelab-salt` and reports drift; a host cannot decrypt another host's pillar (tested); the DNS and DHCP rehearsal drills pass on the isolated test pair and Salt's rendered DNS and DHCP configs match Ansible's byte for byte before the flip; every Ansible role and play is deleted or ported. |
| 7 | Every vault key has a new home (including the cluster age key and the Access email lists `terraform-root` reads). The rebuild drill passes from git and GCS only, with each step timed and recorded, and no Ansible. `homelab` archived. |

## Success measures

- Onboarding a new service touches one repo and takes no new template, port
  entry or firewall rule.
- Moving a stateless service between nodes is a values change.
- A minor-version k3s upgrade is a PR plus one dispatch, with app downtime of
  zero for stateless apps.
