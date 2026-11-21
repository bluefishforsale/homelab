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
- **G6.** The fleet never falls behind on CVE patches again: security patches
  roll continuously, and a host that falls behind raises an alert. This holds
  for as long as Debian patches the release a host runs (D53).

## Non-goals (not in this plan)

- Debian major upgrades, and replacing ocean or harbor with new VMs. Every VM
  stays on the Debian major it runs today. The distro lifecycle, and how an
  old ocean becomes a new one, get their own plan once k3s and Salt are in
  and upgrades have been proven in operation (D11).
- New hardware. No disks, no bays, no new hosts.
- New cluster VMs. The cluster is ocean and harbor as they are (D5).
- HA control plane. One k3s server, on ocean; the API is down for minutes
  during an upgrade or an ocean reboot, while harbor's workloads keep running.
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
  changed in one step. (ExternalDNS writing a service record it owns is not
  this; ExternalDNS touching any other record is, D55.) A wildcard DNS record. Any sshd, `authorized_keys` or port 22 change made by an
  agent as part of this work.
- **A8.** A service migrated without its health check, blackbox probe and
  metrics scrape (the "every service is monitored" rule).
- **A9.** An OS major upgrade on any VM as part of this work (`apt
  full-upgrade` across a release, `do-release-upgrade`, or a replacement VM
  on a newer major). The hypervisors' PVE 9 upgrade is the only exception
  (D48).
- **A10.** `zpool export`, `zpool import` or any other mutating ZFS verb in a
  committed, runnable play.
- **A12.** A kernel reboot on a ZFS or NVIDIA host before `dkms status` shows
  both modules built for the new kernel.
- **A13.** Any step of the k3s rollout that leaves Docker on a host unable to
  keep its containers running, start a new one, or route to one, while that
  host still has Docker services. A step that was not checked for this counts
  the same as one that failed it.

## Decisions

"Made by" says whose call it was: `terrac` chose it, `design call` came out of
the design work and is still open to challenge in review, and `design call,
approved` is one terrac has confirmed. Numbers are stable, so a missing
number (here or in the requirements and anti-goals) is one that was dropped.

| # | Decision | Made by |
|---|---|---|
| D1 | k3s with SQLite via kine. No etcd. | terrac |
| D2 | One production cluster. No management cluster; tier 0 VMs cover the break-glass role. The rehearsal cluster is a second cluster, a server and one agent, that holds nothing (D51). | design call, approved |
| D3 | Cluster installed and upgraded by Ansible until proven, then by Salt. | terrac |
| D4 | Ocean is a cluster node from phase 1 and runs Docker and k3s side by side for the whole migration; services move one at a time (least downtime per service). | design call, approved |
| D5 | No new cluster VMs and no new hardware. The k3s server runs on ocean, configured as it is today, and harbor is the node005 agent. | terrac |
| D6 | Argo CD, Helm charts, app-of-apps, in `homelab-kube` brought up to date. | terrac |
| D7 | Salt runs masterless: `salt-call --local` on each host from a checkout of `homelab-salt`, driven by CI over SSH. No master VM, no minion daemon. | terrac |
| D8 | Upgrade drill: start three minors behind stable, upgrade three times with Argo CD and test apps running, before any real service moves. It runs on the rehearsal VM (D51), so ocean is never the first host to see an upgrade. | terrac (drill), design call, approved (where it runs) |
| D9 | `/data01` reaches the cluster via NFS from the ZFS passthrough on ocean, as today. | terrac |
| D10 | The cluster gets a load-balancer address pool, `192.168.1.64/27`, which sits outside the DHCP pool (`.150` to `.239`) and is empty today apart from one stale record. Cilium hands the addresses out (LB IPAM) and answers ARP for them (L2 announcements). MetalLB in L2 mode is the fallback if Cilium's way fails the Docker gate on the rehearsal cluster. BGP is wanted later and is not in this plan. The top `/29` of the pool belongs to the rehearsal cluster. New VMs still take addresses from the existing static range, and ntfy's name resolves to agentbox, since a push that travels through ocean dies with ocean (D19). | terrac |
| D11 | Patches and package upgrades are continuous, for CVEs. OS majors, and replacing ocean or harbor, are out of this plan: they are planned after everything is on Salt and k3s and upgrades have been proven in operation, even if that runs past the end of Debian 12 LTS (D53). | terrac |
| D13 | Ocean runs Docker and k3s side by side until Docker is completely drained, and ends this plan as the same VM with Docker removed. | terrac |
| D14 | Pods pin to node role labels, never to a hostname, so a later replacement VM needs no chart change. Where a service's state lives is decided by whether it has a SQLite database or other write-ahead log that NFS could corrupt (harbor's pods can reach the pool over NFS, so the rule has to be held by the charts), how much disk latency it tolerates, and how much bandwidth it moves to and from the pool. | design call, approved (terrac: the storage criteria) |
| D15 | CNI is Cilium. | terrac |
| D17 | "Cluster is down" is reported by email and ntfy. | terrac |
| D18 | Image updates take two paths: third-party images by a bot that watches registries and opens tag-bump PRs; home-grown images by push then dispatch that updates the chart. | terrac |
| D19 | The dead-man watchdog and ntfy live on agentbox (tier 0), built in phase 1; ntfy never moves into the cluster. The watchdog's email leaves by a relay that does not pass through ocean. Alertmanager keeps the exim smarthost on the ocean host as its own email route, so node005 dying is still reported. agentbox is slimmed to what must survive the cluster: the watchdog, ntfy, the remote-control sessions and break-glass applies. Its alert receiver and media timers become pods. | design call, approved |
| D20 | North-south traffic uses Cilium's Gateway API on an address of its own from the D10 pool, on 80/443 from phase 1. No host ports and no per-service NodePorts. A service's names, LAN and public, move to the Gateway at its own cutover (D55). Public traffic reaches the Gateway through one Cloudflare tunnel that belongs to the cluster: `cloudflared` as a Deployment with a replica per node and a single catch-all rule to the Gateway's in-cluster Service, written once in `terraform-root`. The tunnel never names a host or a pod, so a pod that moves between nodes changes no tunnel config and no DNS record. Ocean's tunnel and nginx serve what has not moved and retire at W7. ingress-nginx (retired March 2026), a tunnel per service and the tunnel ingress controller are not used. | terrac |
| D21 | Third-party update PRs: patch and minor for stateless apps auto-merge in the Sunday window; majors, stateful apps and platform charts wait for terrac. Renovate self-hosted on the runner. | design call, approved |
| D22 | Ocean is excluded from kured; it reboots only in the supervised monthly window. harbor is excluded too until its Docker is gone (D54). An alert fires when ocean has had a reboot pending for more than 35 days, which means a window was missed (R12). Kernel live patching is not used: nothing free supports Debian, it would not cover the ZFS and NVIDIA modules, and it would be changing kernel code on the host that owns the pool. It is a question for the distro-lifecycle plan. | design call, approved |
| D23 | Shape B static sites become small nginx images and use the home-grown path. | design call, approved |
| D24 | The 2021 app charts in `homelab-kube` are upgraded, not replaced, and are the style basis for all app charts. Their dead `k8s-at-home` dependency moves to `bjw-s-labs/app-template`, the maintained descendant of the same library, starting on 5.x (5.3.0 was current on 2026-11-19) and pinned. A library major is a supervised change, run on the rehearsal cluster first: majors have broken the values schema four times, and the move to 5.x changed Deployment selector labels, which means deleting and recreating each Deployment. | terrac (upgrade, style), design call, approved (which dependency; terrac: 5.x) |
| D25 | DNS and DHCP are Salt states applied by CI, not by hand: agents may author changes, terrac is the only merger, and every apply goes through the pair gates (R9a). Service records written by ExternalDNS are the one exception (D55). sshd, `authorized_keys`, the fleet key and port 22 stay terrac-by-hand. | terrac |
| D26 | VM root disks are cattle: no VM-disk backups; every VM rebuilds from git, GCS and `/data01`, proven by the rebuild drill. A VM that fails the drill gets a weekly vzdump until it passes. | design call, approved |
| D28 | Each hypervisor's `/var/lib/pve-cluster/config.db` backed up nightly to GCS, age-encrypted; restoring it is how surviving VM disks get reattached after a reinstall. | design call, approved |
| D29 | One Salt bootstrap script for every host: by hand on bare metal, from cloud-init on VMs, from an Ansible play during the migration. Cloud-init is the preferred path: a new VM goes from `terraform apply` to its full highstate with no manual step. The apply dispatches the Salt workflow for the new host, and the rekey PR that lets the host read its own pillar merges itself under R31's guards. The DNS pair and the hypervisors are the exceptions; their rekey waits for terrac. | design call, approved (terrac: hands-off through cloud-init, highstate after provision, guarded auto-merge of the rekey) |
| D30 | The VM base image is a golden image built on a schedule by plain CI, no image framework. Each run resolves Debian's newest dated cloud build (a build ID and sha512, never the `latest` tag), boots it as a throwaway VM, lets cloud-init run the D29 bootstrap, the updates and a `base`-role highstate with no secrets, scrubs the VM's identity, and converts it to a template with cloud-init still enabled. The disk is exported to GCS with its sha512; `terraform-root` pins that hash and downloads it per node. Moving the pin is a PR, opened after the rehearsal VM is recreated from the new image and reaches a clean highstate. | terrac |
| D31 | Pods with node-local databases restore through a per-chart restore Job: manual, dry-run by default, never auto-synced. Manual is where this starts; automating a restore after a rebuild is a later change, made once the manual path has been used. | design call, approved |
| D32 | An offline recovery age key is a recipient on every sops file in every repo, with a quarterly decrypt test. | design call, approved |
| D33 | The DNS pair gate has a `rebuild_peer` dispatch mode, approved only by terrac, that rebuilds a dead peer without touching the survivor. | design call, approved |
| D34 | Salt owns the hypervisors, passthrough and HBA import guards included. Their network bridges are Salt-rendered but applied only by terrac with the iDRAC console open; CI runs them `test=True`. | design call, approved |
| D35 | Every Postgres database runs as a single-instance CloudNativePG cluster with continuous WAL archiving to GCS: RPO about five minutes, 14 days of point-in-time recovery. The nightly logical dump stays. No Postgres replicas. | terrac (WAL archiving), design call, approved (shape) |
| D36 | A second self-hosted runner on node006, so losing node005 does not also lose CI. A runner is never applied or rebooted by its own job. | design call, approved |
| D37 | Phases gate on what they depend on, not on their number: phase 2 (harbor) and phase 3 (the GPU gate) run in parallel after phase 1, and W1 starts after the phase 1 soak. Blackbox, pushgateway and karma move in W6, not W1. | design call, approved |
| D38 | kured reboots k8s nodes Wednesday 03:00 to 05:00 Pacific, one node at a time, clear of the Sunday image refresh. | terrac |
| D39 | Internal TLS: the `homelab-ca` design changes before it is built to a `pathlen:1` root, and the cluster gets its own replaceable intermediate for cert-manager. The root key never enters the cluster. The root already permits `cluster.local` but not pod or service IPs, and an intermediate cannot widen its root's constraints, so the CA change also decides whether the root admits the cluster's IP ranges. | terrac |
| D40 | Masterless Salt stays the host tool and its bootstrap is phase 0 work: `homelab-salt`, the bootstrap script and the first base states land in phases 0 and 1, so the D28 timer, the patch pipeline and the watchdog are written in Salt once. D3 still keeps the k3s play in Ansible until it has passed the drill. | terrac |
| D41 | `glances` is dropped. `avahi-daemon` and `netatalk` move to pods on ocean in W5, on hostNetwork, since mDNS needs the LAN. | terrac |
| D42 | Each host holds its own age key and decrypts only the pillar encrypted to it, so the drift timer and a break-glass local apply work without CI. | terrac |
| D43 | No third resolver, and the UniFi gateway is not a fallback resolver. Nothing off the hypervisors exists to host one (pihole is a VM there, like dns01 and dns02), and reliable pihole filtering and `.home` lookups every day are worth more than external names during a double hypervisor loss. Losing both hypervisors takes DNS down until node005 is back; the recovery order is what shortens that. | terrac |
| D44 | Each service runs as its own uid and its state directory is `0700` to that uid; the `media` gid stays as the shared group on the media tree (R26, and the design's "Storage"). | terrac |
| D45 | 14 days of soak between a cutover and its retire PR, the same length as the ocean swap's backout. | terrac |
| D46 | Everything on harbor moves in phase 2 with harbor: nzbget, tdarr-node, ml-log-analyzer, mem0 and archmap. That puts three of the four D35 databases in phase 2. ml-log-analyzer goes first, as the least critical, and gets a nightly dump now, since it is in no backup today. | terrac |
| D47 | The cluster is never more than two minors behind k3s stable (R23). | terrac |
| D48 | Both hypervisors are upgraded in place to PVE 9 in phase 0, node005 first, one per window, because PVE 8 reached end of life in August 2026. | terrac |
| D49 | k3s upgrades run only in ocean's supervised monthly window (R29): ocean is the server, a minor upgrade needs a drain, and draining ocean stops nearly everything. harbor's agent follows in the same window. harbor cannot go first as the canary: upstream's skew policy says a kubelet must not be newer than the API server, and the API server is on ocean. The canary is the rehearsal cluster, which has a server and an agent (D51) so the same server-then-agent upgrade runs there a week earlier. | terrac |
| D50 | An nginx feature with no Gateway equivalent is decided per feature at the phase 1 gate: a Cloudflare rule for a public vhost, an nginx pod behind the Gateway for a LAN-only one, dropping it last. | terrac |
| D51 | A standing rehearsal cluster: two small VMs on node005's existing `vmpool-storage`, a server and one agent (terrac, with D49), each with Docker installed, that takes every risky step before ocean does: the Docker coexistence proof, the upgrade drill, the break-and-restore, and each later upgrade. It is never a member of the production cluster, runs its own k3s version and chart versions, has its own sops key, and holds no real data or secrets, so it can be rebuilt at will. | design call, approved (terrac: standing, not rebuilt per upgrade) |
| D52 | node006's boot disk stays a single point: it holds Proxmox and every VM disk on that node, and nothing mirrors it. Neither hypervisor's boot disk is mirrored. The protection is rebuild from git, the nightly k3s server backup (R2) and the config DB backup (D28). | terrac (no physical change), design call, approved (what covers the gap) |
| D53 | Running past the end of Debian 12 LTS (June 2028) is accepted. This plan ends in 2028 H1 and the distro-lifecycle plan only starts after it (D11), so ocean, harbor and the other Debian 12 VMs may run unpatched by Debian for a while. That is preferred to folding an OS major into this work. | terrac |
| D54 | kured stays off harbor until Docker is removed from it. Through phase 2 harbor reboots only in a supervised window, with the Docker gate before and after, the same as ocean. It joins kured's rota as the last step of phase 2. | terrac |
| D55 | No wildcard DNS. The `*.home` wildcard to ocean is removed in phase 0, replaced by one explicit record per name nginx serves, and the stale `*.cluster.local` wildcard goes with it. Cluster services get real `<svc>.home` records in PowerDNS, so a laptop resolves them the way it resolves any host; `cluster.local` stays inside the cluster. ExternalDNS in the cluster writes those records from each service's route, with a short TTL. The PowerDNS pair stays two independent primaries with no replication, so one ExternalDNS instance runs per server and each keeps its own server complete. This is the one exception to D25: it touches only records it created, a `homelab-kube` CI check refuses a hostname that is in the address table, and it never writes host, DHCP or resolver data. A second instance writes a public service's record in Cloudflare the same way, so a service and its names live in one repo and nothing is coordinated with `terraform-root` after the move; those records leave `terraform-root` once, in phase 1. The tunnels, the Access apps and every record that is not a cluster service stay there. A route publishes a public name only if it is labelled to. | terrac |

## Open items

Not decided yet. Each needs terrac, and none blocks phase 0.

- axiomd runs as a compose project on agentbox. It is in no tier and no wave:
  does it stay on agentbox, or become a pod?
- llmeval is a VM on node006 with a 300G disk on the single boot SSD. Nothing
  here says what runs on it, whether it is tier 0, or what on that disk
  cannot be rebuilt.
- D26 says a VM that fails the rebuild drill gets a weekly vzdump, but the
  drill rebuilds nested test VMs, not agentbox, harbor, pihole or llmeval,
  and there is no backup storage for a vzdump to land on.
- `homelab-salt`: public or private. A private repo on a user account may not
  get the environment approval rule that D25 and D33 lean on.

## Requirements

- **R1.** k3s version pinned in one variable per cluster (production and
  rehearsal, since the rehearsal cluster runs ahead); install and upgrade
  plays are on the never-auto-apply list.
- **R2.** k3s server state on ocean (SQLite DB, token, `server/tls`, the
  node password file) backed up nightly to GCS, with a freshness alert;
  restore rehearsed in phase 1 on the rehearsal VM, both onto a fresh build
  and in place as an upgrade backout, and documented as a runbook. Ocean's
  own backup is restored onto a scratch VM once before W1 and again in each
  yearly rebuild drill.
- **R3.** Argo CD manages itself and the app-of-apps root from `homelab-kube`
  `master`. Each cluster has its own root there (production and rehearsal),
  so the rehearsal cluster can run chart versions production has not taken
  yet. Sync policy `allowEmpty: false`. Every `/data01` and state PV is
  `Retain`, `local-path` volumes included: the default StorageClass reclaims
  with `Retain` and the stock `Delete` class is removed.
- **R4.** Image pulls prefer registry-cache-01 and fall back to the upstream
  registry when it is down.
- **R5.** NVIDIA device plugin with time-slicing on ocean; at least Plex,
  Tdarr and llama.cpp schedule onto the GPU together.
- **R6.** Every migrated service has a default-deny network policy plus
  explicit allows (a CiliumNetworkPolicy where the peer is the Gateway or a
  host, which a plain NetworkPolicy cannot name), a health check, a blackbox
  probe, and a metrics scrape if it exposes metrics. Its alert rules and dashboard panels return series after the move.
  Its namespace runs at Pod Security `baseline` unless the chart lists why it
  cannot (hostNetwork, GPU, a host device).
- **R7.** One service per cutover. Each cutover is one `homelab-kube` PR (chart,
  values, secrets) and one `homelab` PR (retire play, files, port, vault keys,
  timers), preceded where needed by a `terraform-root` PR (bucket, Access app)
  and a PR in the service's own repo (dispatch target). The window runs as one
  dispatch workflow with a `back` direction, which is the only thing that
  applies to the host while it runs; scaling a service up or down is a
  commit to `homelab-kube`, never a change made straight to the cluster. The
  `homelab` retire PR waits out the 14-day soak (D45). Through the soak a committed per-service
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
  rollback. Nothing on those paths auto-merges, and the apply waits for
  terrac's approval on a GitHub environment. A required review cannot be the
  control: agents open PRs from terrac's own account, and GitHub does not let
  an account approve its own PR. Host, IP, MAC,
  reservation and DNS record render from one address table that CI checks
  against `terraform-root`'s VM MACs. No zone holds a wildcard. Service
  records are ExternalDNS's (D55): Salt's render leaves alone any record
  that carries ExternalDNS's ownership marker, and ExternalDNS leaves alone
  any that does not.
- **R10.** `unattended-upgrades` (security origin) on every VM; the DNS pair
  reboots one at a time through the pair gates, never both in a window run;
  NVIDIA, ZFS and kernel packages held on passthrough and module-building
  hosts, and moved in a supervised monthly window.
- **R11.** kured reboots k8s nodes one at a time in the D38 window, draining
  first. Ocean is excluded for good (D22) and harbor until its Docker is
  gone (D54), so kured's first production reboot is harbor at the end of
  phase 2. Before that it runs on the rehearsal cluster.
  CloudNativePG clusters set `enablePDB: false` so a drain is not blocked by
  a single-instance database.
- **R12.** Pending-security-update and reboot-required metrics on every host,
  alerting at 7 and 14 days. Ocean's reboot alert is at 35 days: it reboots
  monthly by design (D22), so 14 would fire every month and be ignored,
  and 35 means a window was missed.
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
  metric reports the gap and an alert fires past it, the same way R12
  does for the OS.
- **R24.** Two self-hosted runners on different hypervisors (D36). A runner's
  own states and reboots are applied by the other runner, or from agentbox.
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
- **R29.** k3s upgrades run in ocean's supervised monthly window, then
  harbor (D49). The rehearsal cluster takes the same upgrade at least a week
  earlier (D51), server first and then its agent, so a failure there has
  time to be understood.
- **R30.** The dead-man watchdog alerts only past a ten-minute outage, so a
  planned ocean reboot does not page.
- **R31.** The rekey workflow reads a host's public key from the host. It
  never accepts a key as an input. For a new VM its PR merges without
  terrac only if all of these hold: the host is in the address table and in
  `terraform-root`; the key was read from the host at its table IP; the diff
  touches only `.sops.yaml` recipients for that host's own pillar file and
  the shared files its role already lists, and the re-encrypted files
  themselves. Anything else, and any rekey of dns01, dns02, pihole or a
  hypervisor, waits for terrac's merge. The highstate that follows is
  dispatched by CI; a host still never fetches the tree itself (R9).
- **R32.** Deploying k3s never breaks Docker or the ability to run containers
  under it (terrac). On every host with Docker services, from its first k3s
  step until its last container has moved to containerd under k3s, each step
  is bracketed by the Docker gate in the design: before and after the k3s
  install or join, every k3s and Cilium upgrade or datapath change, every
  Docker or NVIDIA runtime package upgrade, every reboot, and every service
  cutover. No reboot of such a host is unattended (D54). A failed gate halts the step and its
  backout runs. The gate is one committed check, proven on the rehearsal VM
  first.

## Acceptance (per phase gate)

Every gate from phase 1 through phase 5 also requires that the Docker gate
(R32) ran before and after each step in that phase and never ended a step
failing.

| Phase | Gate |
|---|---|
| 0 | `homelab-kube` defects fixed (Retain, allowEmpty, age). `homelab-salt` and the bootstrap script exist and CI has applied one state to one host. ml-log-analyzer's database is in the nightly dump. Both hypervisors on PVE 9 (D48). Pillar decryption settled and the per-host deploy lock (R27) in place. The config DB backup live on both hypervisors (D28), the recovery age key in place (D32), and everything on a node006 VM disk (ocean's root, dns02, llmeval) that is in none of git, GCS or `/data01` listed and given a home (D52), before node006's PVE upgrade. The `*.home` and `*.cluster.local` wildcards are gone, every name nginx serves has its own record, and the running PowerDNS pair is confirmed to be two independent primaries (D55). |
| 1 | On the rehearsal VM: Docker and k3s coexist through a reboot; three upgrades completed with Argo CD and three test apps running, the third needing no step outside the play; the Gateway on its own pool address on 80/443, announced on the LAN, with ExternalDNS writing a test record a laptop resolves; one deliberate restore from backup onto a fresh build succeeds, and one upgrade is backed out in place. Then on ocean: k3s runs beside Docker through a reboot with no Docker service impact; the Gateway on its pool address on 80/443, with the sonarr reference chart behind it under a record ExternalDNS wrote; the cluster tunnel up, with one public test name served through it; the existing service records, LAN and public, handed to ExternalDNS, still pointing at ocean; each nginx feature in use has an equivalent proven on it or a decided fallback (D50). The dead-man watchdog and ntfy on agentbox, tested by stopping the API. Cluster access written down (R28). 14-day soak on ocean with no unplanned API outage. |
| 1b | Patch pipeline live on ocean, harbor, the rehearsal VM and the tier 0 VMs: unattended-upgrades, update and reboot metrics and alerts. kured has rebooted the rehearsal cluster's node in its window; ocean and harbor are excluded from it (D22, D54). |
| 2 | harbor has joined as an agent beside Docker. Its workloads (nzbget, tdarr-node, ml-log-analyzer, mem0, archmap, the nightly DB restore test) run as pods on it, each through its own cutover and soak, the three Postgres databases on CloudNativePG. Docker removed from harbor. Then harbor joins kured's rota, and a kured drain of it completes with the databases on it. |
| 3 | GPU time-slicing runs a CUDA pod plus an NVENC transcode pod on ocean while Docker's llama.cpp still runs; NVENC session cap measured and recorded. |
| 4 | Every Docker service on ocean is a pod or explicitly parked. |
| 5 | Stopping the k3s API produces both an email and an ntfy push from agentbox (tested before Alertmanager moves). Prometheus, Loki, Grafana and Alertmanager run in cluster. Stopping agentbox still produces an email from Alertmanager. Docker nginx and ocean's tunnel retired; no service record, LAN or public, points at ocean. Docker removed from ocean. |
| 6 | Every Linux host applies masterless from `homelab-salt` and reports drift; the Salt k3s state has done an install and an upgrade on the rehearsal VM; a host cannot decrypt another host's pillar (tested); the DNS and DHCP rehearsal drills pass on the isolated test pair and Salt's rendered DNS and DHCP configs match Ansible's byte for byte before the flip; every Ansible role and play is deleted or ported. |
| 7 | The operational tooling `homelab` carries (`scripts/`, `health-check.yml`, the alert-to-issue receiver's target repo) has a new home or a recorded retirement. Every vault key has a new home (including the cluster age key and the Access email lists `terraform-root` reads). The rebuild drill passes from git and GCS only, with each step timed and recorded, and no Ansible. `homelab` archived. |

## Success measures

- Onboarding a new service touches one repo and takes no new template, port
  entry or firewall rule.
- Moving a stateless service between nodes is a values change.
- A minor-version k3s upgrade is a PR plus one dispatch in ocean's monthly
  window, already run once on the rehearsal VM. Its cost is one planned drain
  of ocean, which that window takes anyway.
