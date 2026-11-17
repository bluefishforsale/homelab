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

- **G1.** Every tier 2 container service on the fleet runs on one k3s cluster,
  deployed by Argo CD from Helm charts in `homelab-kube`, onboarded one at a
  time through an app-of-apps. Tier 0 keeps its compose stacks (the DNS pair,
  registry-cache, ntfy), owned by Salt and refreshed by `docker-refresh@`.
  That is the intended end state, not a leftover: two update systems, split
  by tier.
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

"Made by" says whose call it was: `terrac` chose it, `design call` came out of
the design work and is still open to challenge in review, and `design call,
approved` is one terrac has confirmed.

| # | Decision | Made by |
|---|---|---|
| D1 | k3s with SQLite via kine. No etcd. | terrac |
| D2 | One cluster. No management cluster; tier 0 VMs cover the break-glass role. | design call, approved |
| D3 | Cluster installed and upgraded by Ansible until proven, then by Salt. | terrac |
| D4 | Ocean joins as the GPU agent in phase 3 and runs Docker and k3s side by side for the whole migration; services move one at a time (least downtime per service). | design call, approved |
| D5 | New small server VM on node006; new agent VM on node005 replaces the harbor VM. Both defined in `terraform-root`. | terrac (server VM), design call (server is not ocean) |
| D6 | Argo CD, Helm charts, app-of-apps, in `homelab-kube` brought up to date. | terrac |
| D7 | Salt runs masterless: `salt-call --local` on each host from a checkout of `homelab-salt`, driven by CI over SSH. No master VM, no minion daemon. | terrac |
| D8 | Upgrade drill: start three minors behind stable, upgrade three times with Argo CD and test apps running, before any real service moves. | terrac |
| D9 | `/data01` reaches the cluster via NFS from the ZFS passthrough on ocean, as today. | terrac |
| D10 | No new IP ranges and no DNS changes; ocean's IP stays the front door. | design call |
| D11 | OS majors by VM replacement only. Patches and package upgrades continuous, for CVEs. | terrac |
| D12 | Passthrough VMs (ocean, `k3s-agent01`) are replaced by the passthrough swap: build and patch a new VM, stop both, move the devices, start the new one, pods reschedule onto it. | terrac (pattern), design call (details) |
| D13 | Ocean runs Docker and k3s side by side until Docker is completely drained. Only then does the first passthrough swap move ocean to a clean Debian 13 k8s-only VM, by end of 2027. | design call, approved |
| D14 | Pods pin to node role labels, never to a hostname, so they follow a replaced VM unchanged. | design call |
| D15 | CNI is Cilium. | terrac |
| D16 | node005 RAM upgrade lands before phase 2 (the harbor VM and its replacement overlap). Done: node005 has 252 GiB. | terrac |
| D17 | "Cluster is down" is reported by email and ntfy. | terrac |
| D18 | Image updates take two paths: third-party images by a bot that watches registries and opens tag-bump PRs; home-grown images by push then dispatch that updates the chart. | terrac |
| D19 | The dead-man watchdog and ntfy live on agentbox (tier 0), built in phase 1; ntfy never moves into the cluster. The watchdog's email leaves by a relay that does not pass through ocean. Alertmanager keeps the exim smarthost on the ocean host as its own email route, so node005 dying is still reported. agentbox is slimmed to what must survive the cluster: the watchdog, ntfy, the remote-control sessions and break-glass applies. Its alert receiver and media timers become pods. | design call, approved |
| D20 | North-south traffic uses Cilium's Gateway API in host-network mode: on alternate host ports from phase 1, with nginx proxying each migrated service to it, then on 80/443 on ocean at W7. No per-service NodePorts. ingress-nginx (retired March 2026), MetalLB and the tunnel ingress controller are not used. | design call (mode), terrac (staging on alternate ports) |
| D21 | Third-party update PRs: patch and minor for stateless apps auto-merge in the Sunday window; majors, stateful apps and platform charts wait for terrac. Renovate self-hosted on the runner. | design call, approved |
| D22 | Ocean is excluded from kured; it reboots only in the supervised monthly window. | design call |
| D23 | Shape B static sites become small nginx images and use the home-grown path. | design call, approved |
| D24 | The 2021 app charts in `homelab-kube` are upgraded, not replaced, and are the style basis for all app charts. Their dead `k8s-at-home` dependency moves to `bjw-s-labs/app-template`, the maintained descendant of the same library. | terrac (upgrade, style), design call (which dependency) |
| D25 | DNS and DHCP are Salt states applied by CI, not by hand: agents may author changes, terrac is the only merger, and every apply goes through the pair gates (R9a). sshd, `authorized_keys`, the fleet key and port 22 stay terrac-by-hand. | terrac |
| D26 | VM root disks are cattle: no VM-disk backups; every VM rebuilds from git, GCS and `/data01`, proven by the rebuild drill. A VM that fails the drill gets a weekly vzdump until it passes. | design call, approved |
| D27 | Both hypervisors: VM disks on a ZFS mirror `vmpool` separate from boot, and a mirrored boot disk. node006's VM storage moves first, in 2026, not 2027. Each boot-mirror reinstall lands on the current PVE major. | terrac (close every rebuild gap), design call (shape) |
| D28 | Each hypervisor's `/var/lib/pve-cluster/config.db` backed up nightly to GCS, age-encrypted; restoring it is how surviving VM disks get reattached after a reinstall. | design call |
| D29 | One Salt bootstrap script for every host: by hand on bare metal, from cloud-init on VMs, from an Ansible play during the migration. | design call |
| D30 | The VM base image is stock Debian genericcloud, pinned by hash, mirrored to GCS, downloaded per node by `terraform-root`. No custom template build. | design call |
| D31 | Pods with node-local databases restore through a per-chart restore Job: manual, dry-run by default, never auto-synced. | design call |
| D32 | An offline recovery age key is a recipient on every sops file in every repo, with a quarterly decrypt test. | design call, approved |
| D33 | The DNS pair gate has a `rebuild_peer` dispatch mode, approved only by terrac, that rebuilds a dead peer without touching the survivor. | design call |
| D34 | Salt owns the hypervisors, passthrough and HBA import guards included. Their network bridges are Salt-rendered but applied only by terrac with the iDRAC console open; CI runs them `test=True`. | design call |
| D35 | Every Postgres database runs as a single-instance CloudNativePG cluster with continuous WAL archiving to GCS: RPO about five minutes, 14 days of point-in-time recovery. The nightly logical dump stays. No Postgres replicas. | terrac (WAL archiving), design call (shape) |
| D36 | A second self-hosted runner on node006, so losing node005 does not also lose CI. A runner is never applied or rebooted by its own job. | design call, approved |
| D37 | Phases gate on what they depend on, not on their number: phases 2 and 3 run in parallel after phase 1, and W1 starts on `k3s-agent01` after the phase 1 soak and the coexistence proof. Blackbox, pushgateway and karma move in W6, not W1. | design call, approved |
| D38 | kured reboots k8s nodes Wednesday 03:00 to 05:00 Pacific, one node at a time, clear of the Sunday image refresh. | terrac |
| D39 | Internal TLS: the `homelab-ca` design changes before it is built to a `pathlen:1` root, and the cluster gets its own replaceable intermediate for cert-manager. The root key never enters the cluster. The root already permits `cluster.local` but not pod or service IPs, and an intermediate cannot widen its root's constraints, so the CA change also decides whether the root admits the cluster's IP ranges. | terrac |
| D40 | Masterless Salt stays the host tool and its bootstrap is phase 0 work: `homelab-salt`, the bootstrap script and the first base states land in phases 0 and 1, so the D28 timer, the patch pipeline and the watchdog are written in Salt once. D3 still keeps the k3s play in Ansible until it has passed the drill. | terrac |
| D41 | `glances` is dropped. `avahi-daemon` and `netatalk` move to pods on ocean in W5, on hostNetwork, since mDNS needs the LAN. | terrac |
| D42 | Each host holds its own age key and decrypts only the pillar encrypted to it, so the drift timer and a break-glass local apply work without CI. | terrac |
| D43 | No third resolver, and the UniFi gateway is not a fallback resolver. Nothing off the hypervisors exists to host one (pihole is a VM there, like dns01 and dns02), and reliable pihole filtering and `.home` lookups every day are worth more than external names during a double hypervisor loss. Losing both hypervisors takes DNS down until node005 is back; the recovery order is what shortens that. | terrac |
| D44 | Each service runs as its own uid and its state directory is `0700` to that uid; the `media` gid stays as the shared group on the media tree (R26, and the design's "Storage"). | terrac |
| D45 | 14 days of soak between a cutover and its retire PR, the same length as the ocean swap's backout. | terrac |
| D46 | ml-log-analyzer is live on harbor, so it is the fourth D35 database: it moves to `k3s-agent01` in phase 2 with harbor's other workloads, and gets a nightly dump now, since it is in no backup today. | terrac |
| D47 | The cluster is never more than two minors behind k3s stable (R23). | terrac |
| D48 | Both hypervisors are upgraded in place to PVE 9 in phase 0, node005 first, one per window, because PVE 8 reached end of life in August 2026. The D27 boot-mirror reinstall stays a later, separate step. | terrac |
| D49 | Ocean's k3s agent upgrades only in its supervised monthly window, trailing the server in between (R29). A minor upgrade needs a drain, and draining ocean stops nearly everything. | terrac |
| D50 | An nginx feature with no Gateway equivalent is decided per feature at the phase 1 gate: a Cloudflare rule for a public vhost, an nginx pod behind the Gateway for a LAN-only one, dropping it last. | terrac |

No questions are open.

## Requirements

- **R1.** k3s version pinned in one variable; install and upgrade plays are
  on the never-auto-apply list.
- **R2.** k3s server state (SQLite DB, token, `server/tls`) backed up nightly
  to GCS; restore rehearsed in phase 1 and documented as a runbook.
- **R3.** Argo CD manages itself and the app-of-apps root from `homelab-kube`
  `master`. Sync policy `allowEmpty: false`. Every `/data01` and state PV is
  `Retain`, `local-path` volumes included: the default StorageClass reclaims
  with `Retain` and the stock `Delete` class is removed.
- **R4.** Image pulls prefer registry-cache-01 and fall back to the upstream
  registry when it is down.
- **R5.** NVIDIA device plugin with time-slicing on ocean; at least Plex,
  Tdarr and llama.cpp schedule onto the GPU together.
- **R6.** Every migrated service has a default-deny NetworkPolicy plus explicit
  allows, a health check, a blackbox probe, and a metrics scrape if it exposes
  metrics. Its alert rules and dashboard panels return series after the move.
  Its namespace runs at Pod Security `baseline` unless the chart lists why it
  cannot (hostNetwork, GPU, a host device).
- **R7.** One service per cutover. Each cutover is one `homelab-kube` PR (chart,
  values, secrets) and one `homelab` PR (retire play, files, port, vault keys,
  timers), preceded where needed by a `terraform-root` PR (tunnel rule, bucket)
  and a PR in the service's own repo (dispatch target). The window runs as one
  dispatch workflow with a `back` direction, and the `homelab` retire PR
  waits out the 14-day soak (D45). Through the soak a committed per-service
  flag makes the play hold the Docker side down, units masked, and the
  service's image is frozen. A chart's CronJobs are suspended until its
  window.
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
  NVIDIA, ZFS and kernel packages held on passthrough and module-building
  hosts, and moved in a supervised monthly window.
- **R11.** kured reboots k8s nodes one at a time in the D38 window, draining
  first. Ocean is excluded (D22). CloudNativePG clusters set `enablePDB:
  false` so a drain is not blocked by a single-instance database.
- **R12.** Pending-security-update and reboot-required metrics on every host,
  alerting at 7 and 14 days.
- **R13.** Every VM on a Debian major within 6 months of its release and before
  the previous major's regular security support ends. The Proxmox template is
  rebuilt per Debian release.
- **R14.** The passthrough swap is a written runbook, rehearsed first on
  `k3s-agent01` (passed-through scratch disk) before it is used on ocean.
- **R15.** Every image the cluster runs is pinned to an immutable tag or
  digest. Nothing runs `:latest`. Tags change only by commit to `homelab-kube`.
- **R16.** Metrics and logs for a migrated service keep flowing to the existing
  Prometheus and Loki from the day it moves (in-cluster agent with remote
  write), not from phase 5.
- **R17.** No pod starts on ocean unless `/data01` is a mountpoint.
- **R18.** Every host unit and timer on ocean has a named destination before
  phase 4 starts (spec table), not only the containers.
- **R19.** The fleet rebuilds from git plus GCS plus `/data01`, in the order in
  the design's "Disaster recovery" section, with no Ansible. Every input
  outside git (age keys, Terraform state, DB dumps, Postgres WAL archives,
  the k3s backup, the Proxmox config DBs, the Salt `.deb`, the Debian image)
  has a named home and a freshness alert or test.
- **R20.** No hypervisor keeps VM disks on its boot disk, and every boot disk
  is mirrored (D27).
- **R21.** A reinstalled node006 never imports `/data01` on the host: HBA bound
  to vfio-pci before `mpt3sas`, pool auto-import off (D34). Until that state
  has applied, the first boot runs with the shelf disconnected or `mpt3sas`
  blacklisted.
- **R22.** Every Postgres database archives WAL to GCS (D35). An alert fires
  when the newest archived segment is older than 15 minutes or the newest base
  backup is older than 8 days, and a monthly recovery into a scratch cluster
  proves the archive restores. A third alert watches `pg_wal` size, since a
  stalled archive fills the local disk.
- **R23.** The cluster is never more than two minors behind k3s stable (D47). A
  metric reports the gap and an alert fires past it, the same way R12 and R13
  do for the OS.
- **R24.** Two self-hosted runners on different hypervisors (D36). A runner's
  own states and reboots are applied by the other runner, or from agentbox.
- **R25.** Proxmox startup order on node006 starts `k3s-server01` before
  ocean.
- **R26.** Every service runs as its own uid, allocated from one registry
  file in `homelab-kube`, and its state directory is `0700` to that uid
  (D44). The change lands after the service's soak, never in its cutover
  window. Cross-uid hardlinks on the media tree are proven first (umask 002,
  setgid directories).
- **R27.** Every apply to a host, from any repo or runner, holds a lock on
  that host. A concurrency group alone does not serialize across repos.
- **R28.** Cluster access is written down before phase 1 ends: terrac's
  break-glass kubeconfig, a read-only default for agents, and a sync-only
  Argo token for CI.
- **R29.** Ocean's k3s agent upgrades in its supervised monthly window, not
  with the other nodes (D49).
- **R30.** The dead-man watchdog alerts only past a ten-minute outage, so a
  planned server reboot does not page.
- **R31.** The rekey workflow reads a host's public key from the host. It
  never accepts a key as an input.

## Acceptance (per phase gate)

| Phase | Gate |
|---|---|
| 0 | `homelab-kube` defects fixed (Retain, allowEmpty, age). `homelab-salt` and the bootstrap script exist and CI has applied one state to one host. ml-log-analyzer's database is in the nightly dump. Both hypervisors on PVE 9 (D48). Pillar decryption settled and the per-host deploy lock (R27) in place. node006's VM disks off its boot disk (D27), the config DB backup live on both hypervisors (D28), and the recovery age key in place (D32). |
| 1 | The dead-man watchdog and ntfy on agentbox, tested by stopping the API. The Gateway in host-network mode on alternate ports, with the sonarr reference chart behind it; each nginx feature in use has an equivalent proven on it or a decided fallback (D50); a listener bound on 80/443 on `k3s-agent01` once. Cluster access written down (R28). Three upgrades completed with Argo CD and three test apps running; the third upgrade needs no step outside the play. One deliberate restore from backup onto a fresh server succeeds. 14-day soak with no unplanned API outage. |
| 1b | Patch pipeline live on the new VMs: unattended-upgrades, kured, update and reboot metrics and alerts. |
| 2 | harbor VM workloads (nzbget, tdarr-node, ml-log-analyzer on CloudNativePG, the nightly DB restore test) run on `k3s-agent01`; harbor VM decommissioned. |
| 3 | Docker and k3s coexist on ocean through a reboot with no Docker service impact; GPU time-slicing runs llama.cpp plus an NVENC transcode in pods; NVENC session cap measured and recorded. |
| 4 | Every Docker service on ocean is a pod or explicitly parked. |
| 5 | Stopping the k3s API produces both an email and an ntfy push from agentbox (tested before Alertmanager moves). Prometheus, Loki, Grafana and Alertmanager run in cluster. Stopping agentbox still produces an email from Alertmanager. Docker nginx retired; the Cilium Gateway has moved to 80/443 on ocean. Docker removed from ocean. |
| 5b | Passthrough swap rehearsed on `k3s-agent01`, then, with Docker drained, ocean swapped to a clean Debian 13 k8s-only VM; old VM deleted after 14 days with no backout. |
| 6 | Every Linux host applies masterless from `homelab-salt` and reports drift; the Salt k3s state has done an install and an upgrade on the rehearsal cluster; ocean's host states existed before 5b; a host cannot decrypt another host's pillar (tested); the DNS and DHCP rehearsal drills pass on the isolated test pair and Salt's rendered DNS and DHCP configs match Ansible's byte for byte before the flip; every Ansible role and play is deleted or ported. |
| 7 | The operational tooling `homelab` carries (`scripts/`, `health-check.yml`, the alert-to-issue receiver's target repo) has a new home or a recorded retirement. Every vault key has a new home (including the cluster age key and the Access email lists `terraform-root` reads). The rebuild drill passes from git and GCS only, with each step timed and recorded, and no Ansible. `homelab` archived. |

## Success measures

- Onboarding a new service touches one repo and takes no new template, port
  entry or firewall rule.
- Moving a stateless service between nodes is a values change.
- A minor-version k3s upgrade is a PR plus one dispatch, with app downtime of
  zero for stateless apps off ocean. Ocean's share of an upgrade is one
  planned drain in its monthly window.
