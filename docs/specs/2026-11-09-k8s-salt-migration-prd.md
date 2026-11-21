# PRD: 2027 platform migration (k3s, Argo CD, Salt)

Date: 2026-11-09
Status: decisions settled 2026-11-21; reviewed 2026-11-21 (findings folded
in, open questions listed under "Open questions")
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

- Changing how the DNS pair replicates. dns02 stays the PowerDNS primary and
  dns01 its AXFR secondary, and ExternalDNS writes to the primary (D55).
  Pushing records to both servers is only on the table as part of Q1.
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

Every decision here is settled: terrac made it or approved it, the last of
them on 2026-11-21. Numbers are stable, so a missing number (here or in the
requirements and anti-goals) is one that was dropped. A decision changes only
by a PR that edits this table and says why.

| # | Decision |
|---|---|
| D1 | k3s with SQLite via kine. No etcd. |
| D2 | One production cluster. No management cluster; tier 0 VMs cover the break-glass role. The rehearsal cluster is a second cluster, a server and one agent, that holds nothing (D51). |
| D3 | Cluster installed and upgraded by Ansible until proven, then by Salt. |
| D4 | Ocean is a cluster node from phase 1 and runs Docker and k3s side by side for the whole migration; services move one at a time (least downtime per service). |
| D5 | No new cluster VMs and no new hardware. The k3s server runs on ocean, configured as it is today, and harbor is the node005 agent. |
| D6 | Argo CD, Helm charts, app-of-apps, in `homelab-kube` brought up to date. |
| D7 | Salt runs masterless: `salt-call --local` on each host from a checkout of `homelab-salt`, driven by CI over SSH. No master VM, no minion daemon. |
| D8 | Upgrade drill: start three minors behind stable, upgrade three times with Argo CD and test apps running, before any real service moves. It runs on the rehearsal cluster (D51), so ocean is never the first host to see an upgrade. |
| D9 | `/data01` reaches the cluster via NFS from the ZFS passthrough on ocean, as today. |
| D10 | The cluster gets a load-balancer address pool, `192.168.1.64/27`, which sits outside the DHCP pool (`.150` to `.239`) and is empty today apart from one stale record. Cilium hands the addresses out (LB IPAM) and answers ARP for them (L2 announcements). MetalLB in L2 mode is the fallback if Cilium's way fails the Docker gate on the rehearsal cluster, or if it cannot move an address to the surviving node while the API server is down (R33). BGP is wanted later and is not in this plan. The top `/29` of the pool belongs to the rehearsal cluster. New VMs still take addresses from the existing static range, and ntfy's name resolves to agentbox, since a push that travels through ocean dies with ocean (D19). |
| D11 | Patches and package upgrades are continuous, for CVEs. OS majors, and replacing ocean or harbor, are out of this plan: they are planned after everything is on Salt and k3s and upgrades have been proven in operation, even if that runs past the end of Debian 12 LTS (D53). |
| D13 | Ocean runs Docker and k3s side by side until Docker is completely drained, and ends this plan as the same VM with Docker removed. |
| D14 | Pods pin to node role labels, never to a hostname, so a later replacement VM needs no chart change. Where a service's state lives is decided by whether it has a SQLite database or other write-ahead log that NFS could corrupt (harbor's pods can reach the pool over NFS, so the rule has to be held by the charts), how much disk latency it tolerates, and how much bandwidth it moves to and from the pool. |
| D15 | CNI is Cilium. |
| D17 | "Cluster is down" is reported by email and ntfy. |
| D18 | Image updates take two paths: third-party images by a bot that watches registries and opens tag-bump PRs; home-grown images by push then dispatch that updates the chart. |
| D19 | The dead-man watchdog and ntfy live on agentbox (tier 0), built in phase 1; ntfy never moves into the cluster. The watchdog's email leaves by a relay that does not pass through ocean. Alertmanager keeps the exim smarthost on the ocean host as its own email route, so node005 dying is still reported. agentbox is slimmed to what must survive the cluster: the watchdog, ntfy, the remote-control sessions and break-glass applies. Its alert receiver and media timers become pods. |
| D20 | North-south traffic uses Cilium's Gateway API on an address of its own from the D10 pool, on 80/443 from phase 1. No host ports and no per-service NodePorts. A service's names, LAN and public, move to the Gateway at its own cutover (D55). Public traffic reaches the Gateway through one Cloudflare tunnel that belongs to the cluster: `cloudflared` as a Deployment with a replica per node and a single catch-all rule to the Gateway's in-cluster Service, written once in `terraform-root`. The tunnel never names a host or a pod, so a pod that moves between nodes changes no tunnel config and no DNS record. Ocean's tunnel and nginx serve what has not moved and retire at W7. ingress-nginx (retired March 2026), a tunnel per service and the tunnel ingress controller are not used. |
| D21 | Third-party update PRs: patch and minor for stateless apps auto-merge in the Sunday window; majors, stateful apps and platform charts wait for terrac. Renovate self-hosted on the runner. |
| D22 | Ocean is excluded from kured; it reboots only in the supervised monthly window. harbor is excluded too until its Docker is gone (D54). An alert fires when ocean has had a reboot pending for more than 35 days, which means a window was missed (R12). Kernel live patching is not used: nothing free supports Debian, it would not cover the ZFS and NVIDIA modules, and it would be changing kernel code on the host that owns the pool. It is a question for the distro-lifecycle plan. |
| D23 | Shape B static sites become small nginx images and use the home-grown path. |
| D24 | The 2021 app charts in `homelab-kube` are upgraded, not replaced, and are the style basis for all app charts. Their dead `k8s-at-home` dependency moves to `bjw-s-labs/app-template`, the maintained descendant of the same library, starting on 5.x (5.3.0 was current on 2026-11-19) and pinned. A library major is a supervised change, run on the rehearsal cluster first: majors have broken the values schema four times, and the move to 5.x changed Deployment selector labels, which means deleting and recreating each Deployment. |
| D25 | DNS and DHCP are Salt states applied by CI, not by hand: agents may author changes, terrac is the only merger, and every apply goes through the pair gates (R9a). Service records written by ExternalDNS are the one exception (D55). sshd, `authorized_keys`, the fleet key and port 22 stay terrac-by-hand. |
| D26 | VM root disks are cattle: no VM-disk backups; every VM rebuilds from git, GCS and `/data01`, proven twice: by the nested rebuild drill, and by destroying and rebuilding each real VM once, from the golden image, as part of its port to Salt. Before a VM's turn, one vzdump of it goes to node005's `vmpool-storage` and is kept until the rebuilt VM has run clean for two weeks; that is the fallback, and there is no standing VM backup. Ocean is the exception and stays one: it is never destroyed or rebuilt by automation, and anything done to it has terrac in the loop. |
| D28 | Each hypervisor's `/var/lib/pve-cluster/config.db` backed up nightly to GCS, age-encrypted; restoring it is how surviving VM disks get reattached after a reinstall. |
| D29 | One Salt bootstrap script for every host: by hand on bare metal, from cloud-init on VMs, from an Ansible play during the migration. Cloud-init is the preferred path: a VM that is already in the address table goes from `terraform apply` to its full highstate with no manual step, which covers every rebuild. The apply dispatches the Salt workflow for the host, and the rekey PR that lets the host read its own pillar merges itself under R31's guards. A VM that has never existed still starts with terrac merging its address-table entry (D25). The DNS pair, pihole and the hypervisors are the exceptions; their rekey waits for terrac. |
| D30 | The VM base image is a golden image built on a schedule by plain CI, no image framework. Each run resolves Debian's newest dated cloud build (a build ID and sha512, never the `latest` tag), boots it as a throwaway VM, lets cloud-init run the D29 bootstrap, the updates and a `base`-role highstate with no secrets, scrubs the VM's identity, and converts it to a template with cloud-init still enabled. The disk is exported to GCS with its sha512; `terraform-root` pins that hash and downloads it per node. Moving the pin is a PR, opened after a throwaway clone of the new image has booted and reached a clean highstate. The rehearsal cluster is not that clone: it is standing (D51) and is not rebuilt for an image. |
| D31 | Pods with node-local databases restore through a per-chart restore Job: manual, dry-run by default, never auto-synced. Manual is where this starts; automating a restore after a rebuild is a later change, made once the manual path has been used. |
| D32 | An offline recovery age key is a recipient on every sops file in every repo, with a quarterly decrypt test. |
| D33 | The DNS pair gate has a `rebuild_peer` dispatch mode, approved only by terrac, that rebuilds a dead peer without touching the survivor. |
| D34 | Salt owns the hypervisors, passthrough and HBA import guards included. Their network bridges are Salt-rendered but applied only by terrac with the iDRAC console open; CI runs them `test=True`. |
| D35 | Every Postgres database runs as a single-instance CloudNativePG cluster with continuous WAL archiving to GCS: RPO about five minutes, 14 days of point-in-time recovery. The nightly logical dump stays. No Postgres replicas. |
| D36 | A second self-hosted runner on node006, so losing node005 does not also lose CI. A runner is never applied or rebooted by its own job. |
| D37 | Phases gate on what they depend on, not on their number: phase 2 (harbor) and phase 3 (the GPU gate) run in parallel after phase 1, and W1 starts after the phase 1 soak. Blackbox, pushgateway and karma move in W6, not W1. |
| D38 | kured reboots k8s nodes Wednesday 03:00 to 05:00 Pacific, one node at a time, clear of the Sunday image refresh. |
| D39 | Internal TLS: the `homelab-ca` design changes before it is built to a `pathlen:1` root, and the cluster gets its own replaceable intermediate for cert-manager. The root key never enters the cluster. The root already permits `cluster.local` but not pod or service IPs, and an intermediate cannot widen its root's constraints, so the CA change also decides whether the root admits the cluster's IP ranges. |
| D40 | Masterless Salt stays the host tool and its bootstrap is phase 0 work: `homelab-salt`, the bootstrap script and the first base states land in phases 0 and 1, so the D28 timer, the patch pipeline and the watchdog are written in Salt once. D3 still keeps the k3s play in Ansible until it has passed the drill. |
| D41 | `glances` is dropped. `avahi-daemon` and `netatalk` move to pods on ocean in W5, on hostNetwork, since mDNS needs the LAN. |
| D42 | Each host holds its own age key and decrypts only the pillar encrypted to it, so the drift timer and a break-glass local apply work without CI. |
| D43 | No third resolver, and the UniFi gateway is not a fallback resolver. Nothing off the hypervisors exists to host one (pihole is a VM there, like dns01 and dns02), and reliable pihole filtering and `.home` lookups every day are worth more than external names during a double hypervisor loss. Losing both hypervisors takes DNS down until node005 is back; the recovery order is what shortens that. |
| D44 | Each service runs as its own uid and its state directory is `0700` to that uid; the `media` gid stays as the shared group on the media tree (R26, and the design's "Storage"). |
| D45 | 14 days of soak between a cutover and its retire PR, the same length as the ocean swap's backout. |
| D46 | Everything on harbor moves in phase 2 with harbor: nzbget, tdarr-node, ml-log-analyzer, mem0 and archmap. That puts three of the four D35 databases in phase 2. ml-log-analyzer goes first, as the least critical, and gets a nightly dump now, since it is in no backup today. |
| D47 | The cluster is never more than two minors behind k3s stable (R23). |
| D48 | Both hypervisors are upgraded in place to PVE 9 in phase 0, node005 first, one per window, because PVE 8 reached end of life in August 2026. |
| D49 | k3s upgrades run only in ocean's supervised monthly window (R29): ocean is the server, a minor upgrade needs a drain, and draining ocean stops nearly everything. harbor's agent follows in the same window. harbor cannot go first as the canary: upstream's skew policy says a kubelet must not be newer than the API server, and the API server is on ocean. The canary is the rehearsal cluster, which has a server and an agent (D51) so the same server-then-agent upgrade runs there a week earlier. |
| D50 | An nginx feature with no Gateway equivalent is decided per feature at the phase 1 gate: a Cloudflare rule for a public vhost, an nginx pod behind the Gateway for a LAN-only one, dropping it last. |
| D51 | A standing rehearsal cluster: two small VMs on node005's existing `vmpool-storage`, a server and one agent, each with Docker installed, that takes every risky step before ocean does: the Docker coexistence proof, the upgrade drill, the break-and-restore, and each later upgrade. It is never a member of the production cluster, runs its own k3s version and chart versions, has its own sops key, and holds no real data, so it can be rebuilt at will. The one real thing it holds is a DNS credential for its test names, which is why that credential's scope is an open question below. |
| D52 | node006's boot disk stays a single point: it holds Proxmox and every VM disk on that node, and nothing mirrors it. Neither hypervisor's boot disk is mirrored. The protection is rebuild from git, the nightly k3s server backup (R2) and the config DB backup (D28). |
| D53 | Running past the end of Debian 12 LTS (June 2028) is accepted. This plan ends in 2028 H1 and the distro-lifecycle plan only starts after it (D11), so ocean, harbor and the other Debian 12 VMs may run unpatched by Debian for a while. That is preferred to folding an OS major into this work. |
| D54 | kured stays off harbor until Docker is removed from it. Through phase 2 harbor reboots only in a supervised window, with the Docker gate before and after, the same as ocean. It joins kured's rota as the last step of phase 2. |
| D55 | No wildcard DNS. The `*.home` wildcard to ocean is already gone (`homelab` #1359 and #1363: every name nginx serves has its own record, and a CI test keeps the list in step with nginx). The stale `*.cluster.local` zone goes in phase 0. Cluster services get real `<svc>.home` records in PowerDNS, so a laptop resolves them the way it resolves any host; `cluster.local` stays inside the cluster. ExternalDNS in the cluster writes those records from each service's route, with a short TTL. It writes through the API of the PowerDNS primary on dns02, and the pair's existing NOTIFY and AXFR carry each record to dns01, the same path a DHCP name takes today. This is the one exception to D25: it touches only records it created, a `homelab-kube` CI check refuses a hostname that is in the address table, and it never writes host, DHCP or resolver data. A second instance writes a public service's record in Cloudflare the same way, so a service and its names live in one repo and nothing is coordinated with `terraform-root` after the move; those records leave `terraform-root` once, in phase 1. The tunnels, the Access apps and every record that is not a cluster service stay there. A route publishes a public name only if it is labelled to. |
| D56 | axiomd stays on agentbox as a compose project, tier 0, never a pod. It triages alerts today and will later be allowed limited actions on the VMs and on the cluster, so it runs on a VM outside the cluster it acts on and keeps working when the cluster is what is broken. Salt owns its compose project like ntfy's. What it may act on, and with which credentials, is decided when it gets actions, not in this plan. |
| D57 | llmeval is a lasting VM, not a temporary one: ported to Salt in phase 6 with the other plain VMs, outside the cluster and outside tier 0. It rebuilds from `terraform-root` plus Salt, and its models download again against their recorded sha256, so its 300G disk holds nothing that needs a backup (D52). |
| D58 | Salt lives in two repos so that as much as possible is public. `homelab-salt` is public: states, templates, bootstrap, CI, and an example pillar the states are tested against. `homelab-salt-pillar` is private and holds only this site's values: the address table, MACs, reservations, zone records and the sops-encrypted secrets. Every apply runs from the public repo's workflows, so the environment approval D25 and D33 need sits there. A change that needs both is two PRs, pillar first. |

## Open questions

No decision above is open. These came out of the 2026-11-21 review. Each is
a choice with a cost, so each waits for terrac; none blocks phase 0, and the
design says what it assumes until each is answered.

- **Q1. A zone of their own for service names.** In the shared `home.` zone
  the cluster holds a PowerDNS API key that can change any record in it,
  host records included; ExternalDNS's ownership markers are its own
  restraint, not a permission. A separate service zone would let the
  credential be scoped to that zone, would let records be pushed to both
  DNS servers directly (terrac's stated preference, with AXFR kept for DHCP
  names; one zone cannot do both), and would give the rehearsal cluster a
  credential that cannot touch production names. The cost is that service
  names either change (`<svc>.<zone>.home`) or each needs a static alias in
  `home.`. Until answered: `<svc>.home`, the shared zone, and the
  replication that is deployed.
- **Q2. One Gateway or two.** With one Gateway, what keeps a LAN-only
  service off the internet is a CI check on a label. With a LAN Gateway on
  the pool address and a second, public Gateway that only `cloudflared` can
  reach, a route that is not attached to the public one cannot be served
  through the tunnel whatever DNS says. The cost is a second Gateway and a
  `parentRefs` line per public route. Until answered: one Gateway and the
  CI check.
- **Q3. Public names without an Access app.** A public route goes live from
  one commit in `homelab-kube`, and its Cloudflare Access app is a separate
  PR in `terraform-root`. A wildcard Access app that denies by default
  would make a forgotten one fail closed. Until answered: a CI check that
  a public hostname has an Access app or says in the chart why it needs
  none.
- **Q4. Who reports the whole site going dark.** Each hypervisor reports
  the other's death; nothing reports both, a power cut or the uplink. An
  outside heartbeat check would. It means one outside service. Until
  answered: that failure is silent, as it is today.
- **Q5. App state that is not a database.** `/data01/services/<svc>` holds
  config, metadata and keys as well as databases. The databases are dumped
  to GCS; the rest has RAIDZ2 and nothing else, and the pool has no
  scheduled snapshots because ZFS is never automated. A small nightly copy
  of those directories to GCS, or a snapshot terrac takes by hand before
  each wave, would cover a bad cutover, a bad `chown` (R26) or a lost
  pool. Until answered: as today.
- **Q6. The cluster-admin kubeconfig on agentbox.** Agents and axiomd run
  on agentbox, and the break-glass kubeconfig is planned to sit there too.
  Either it stays off agentbox (laptop only) or it sits under a user the
  agents cannot read.

What is still unproven is listed in the design under "Assumptions to
verify", each with the phase that checks it.

## Requirements

- **R1.** k3s version pinned in one variable per cluster (production and
  rehearsal, since the rehearsal cluster runs ahead); install and upgrade
  plays are on the never-auto-apply list.
- **R2.** k3s server state on ocean (SQLite DB, token, `server/tls`, the
  node password file) backed up nightly to GCS, age-encrypted to terrac's
  key and the recovery key before it leaves the host, since it holds the
  cluster's CA keys, its join token and every Secret; with a freshness alert;
  restore rehearsed in phase 1 on the rehearsal cluster, both onto a fresh build
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
  host by rsync at the SHA; hosts never fetch from GitHub. The CI age key,
  which can decrypt every pillar file, is needed only to rekey: it is a
  secret of the rekey workflow's GitHub environment and is never on a
  runner's disk or in any other job.
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
  records are ExternalDNS's (D55): whatever writes the static records
  leaves alone any record that carries ExternalDNS's ownership marker, and
  ExternalDNS leaves alone any that does not. A static record change never
  deletes and recreates a zone. Today's import does exactly that on the
  primary, which empties the zone of every DHCP and service name and
  answers REFUSED while it refills, so it is replaced by a record-level
  change in phase 0, in Ansible, before ExternalDNS writes its first
  record; the Salt states inherit it.
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
  time to be understood. CI holds this, not memory: a PR that raises the
  production version of k3s, Cilium or a tier 1 chart fails unless the
  rehearsal cluster has run that version for seven days.
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
  under it. On every host with Docker services, from its first k3s
  step until its last container has moved to containerd under k3s, each step
  is bracketed by the Docker gate in the design: before and after the k3s
  install or join, every k3s and Cilium upgrade or datapath change, every
  Docker or NVIDIA runtime package upgrade, every reboot, and every service
  cutover. No reboot of such a host is unattended (D54). A failed gate halts the step and its
  backout runs. The gate is one committed check, proven on the rehearsal
  cluster first.
- **R33.** harbor's running services survive ocean being down, which takes
  the API server with it. Proven on the rehearsal pair with its server
  stopped: pods on the agent keep running and keep resolving names (CoreDNS
  has a replica on every node), a public name still answers through the
  agent's `cloudflared`, and the Gateway's LAN address is still answered, or
  the announcer is changed until it is (D10). What cannot work without the
  API is written in the runbook: nothing is scheduled, and a node that
  reboots starts no pods until the server is back.
- **R34.** Every backup bucket keeps what was written to it. The service
  account a host or the cluster writes with can create objects and cannot
  delete or overwrite history: the buckets are versioned, pruning is a
  bucket lifecycle rule, and where a tool needs to delete to prune (the
  Postgres WAL archive) a retention lock longer than the alert window sits
  under it. A compromised cluster must not be able to erase its own
  backups.

## Acceptance (per phase gate)

Every gate from phase 1 through phase 5 also requires that the Docker gate
(R32) ran before and after each step in that phase and never ended a step
failing.

| Phase | Gate |
|---|---|
| 0 | `homelab-kube` defects fixed (Retain, allowEmpty, age). `homelab-salt` and the bootstrap script exist and CI has applied one state to one host. ml-log-analyzer's database is in the nightly dump. Both hypervisors on PVE 9 (D48). Pillar decryption settled and the per-host deploy lock (R27) in place. The config DB backup live on both hypervisors (D28), the recovery age key in place (D32), and everything on a node006 VM disk (ocean's root, dns02, llmeval) that is in none of git, GCS or `/data01` listed and given a home (D52), before node006's PVE upgrade. The `*.cluster.local` zone is gone and no zone holds a wildcard; a static record change no longer deletes and recreates a zone (R9a); and a record written through the PowerDNS primary's API is shown to reach dns01 (D55). The golden image builds, and two clones of it have different identities (D30). |
| 1 | On the rehearsal cluster: Docker and k3s coexist through a reboot; the agent has joined and three upgrades completed, server then agent, with Argo CD and three test apps running, the third needing no step outside the play; the Gateway on its own pool address on 80/443, announced on the LAN, with ExternalDNS writing a test record a laptop resolves; the agent's services survive the server being stopped (R33); one deliberate restore from backup onto a fresh build succeeds, and one upgrade is backed out in place. Then on ocean: k3s runs beside Docker through a reboot with no Docker service impact; the Gateway on its pool address on 80/443, with the sonarr reference chart behind it under a record ExternalDNS wrote; the cluster tunnel up, with one public test name served through it; the existing service records, LAN and public, handed to ExternalDNS, still pointing at ocean; each nginx feature in use has an equivalent proven on it or a decided fallback (D50). The dead-man watchdog and ntfy on agentbox, tested by stopping the API. Cluster access written down (R28). 14-day soak on ocean with no unplanned API outage. |
| 1b | Patch pipeline live on ocean, harbor, the rehearsal VMs and the tier 0 VMs: unattended-upgrades, update and reboot metrics and alerts. kured has rebooted the rehearsal cluster's nodes, one at a time, in its window; ocean and harbor are excluded from it (D22, D54). |
| 2 | harbor has joined as an agent beside Docker. Its workloads (nzbget, tdarr-node, ml-log-analyzer, mem0, archmap, the nightly DB restore test) run as pods on it, each through its own cutover and soak, the three Postgres databases on CloudNativePG. Docker removed from harbor. Then harbor joins kured's rota, and a kured drain of it completes with the databases on it. |
| 3 | GPU time-slicing runs a CUDA pod plus an NVENC transcode pod on ocean while Docker's llama.cpp still runs; NVENC session cap measured and recorded. |
| 4 | Every Docker service on ocean is a pod or explicitly parked. |
| 5 | Stopping the k3s API produces both an email and an ntfy push from agentbox (tested before Alertmanager moves). Prometheus, Loki, Grafana and Alertmanager run in cluster. Stopping agentbox still produces an email from Alertmanager. Docker nginx and ocean's tunnel retired; no service record, LAN or public, points at ocean. Docker removed from ocean. |
| 6 | Every Linux host applies masterless from `homelab-salt` and reports drift; every real VM but ocean has been destroyed and rebuilt once (D26); the Salt k3s state has done an install and an upgrade on the rehearsal cluster; a host cannot decrypt another host's pillar (tested); the DNS and DHCP rehearsal drills pass on the isolated test pair and Salt's rendered DNS and DHCP configs match Ansible's byte for byte before the flip; every Ansible role and play is deleted or ported. |
| 7 | The operational tooling `homelab` carries (`scripts/`, `health-check.yml`, the alert-to-issue receiver's target repo) has a new home or a recorded retirement. Every vault key has a new home (including the cluster age key and the Access email lists `terraform-root` reads). The rebuild drill passes from git and GCS only, with each step timed and recorded, and no Ansible. `homelab` archived. |

## Success measures

- Onboarding a new service touches one repo and takes no new template, port
  entry or firewall rule.
- Moving a stateless service between nodes is a values change.
- A minor-version k3s upgrade is a PR plus one dispatch in ocean's monthly
  window, already run a week earlier on the rehearsal cluster. Its cost is one planned drain
  of ocean, which that window takes anyway.
