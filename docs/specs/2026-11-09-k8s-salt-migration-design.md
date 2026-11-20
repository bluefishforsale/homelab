# 2027 platform migration: Kubernetes for services, Salt for hosts

Date: 2026-11-09
Status: draft, awaiting review
Companion docs: PRD `2026-11-09-k8s-salt-migration-prd.md`, roadmap
`../plans/2026-11-09-k8s-salt-migration-roadmap.md`

## Intent

Move service orchestration off hand-built Ansible plays and onto Kubernetes
(k3s, deployed by Argo CD from Helm charts), move host configuration off Ansible
and onto Salt, and finish with the `homelab` repo archived. Do it one service at
a time, with no fleet-wide outage at any point.

### Why

Every service today is a Jinja template, a playbook, a port registry entry, a
vault key threaded into a template, host firewall rules and, if it is public, a
certbot cert on ocean's nginx. The five things that hurt, in terrac's words:

1. **Configuration**: per-service templates and plays.
2. **Secrets management**: one vault file threaded into templates.
3. **Moving services between hosts**: the target host is encoded in the
   playbook path; moving means rewriting the play and hand-migrating state.
4. **Locking down ports**: a central registry plus host firewall rules, nothing
   per-service.
5. **SSL certificates**: per-service, by hand.

Kubernetes gives each of those a platform answer (values files, sealed secrets,
scheduling plus PVs, NetworkPolicy, cert-manager), plus finer network and
filesystem control per service. Salt takes over the part Kubernetes does not
own: the hosts themselves.

## Current state (probed 2026-11-09)

| Thing | Reality |
|---|---|
| node005 | Proxmox 8.1, 56 threads, 252 GiB RAM (`node_memory_MemTotal_bytes` read 2026-11-15). VM disks on `vmpool-storage` (ZFS on SATA/NVMe SSDs). Runs dns01, pihole, netboot01, gh-runner-01, harbor, registry-cache-01, agentbox. |
| node006 | Proxmox 8.4, 40 threads, 660 GiB RAM (498 used, ocean holds 366 GiB). VM disks on `local-lvm` (870 EVO 2TB). Runs dns02, ocean, llmeval. |
| ocean | Debian 12, kernel 6.1, Docker 29.6, cgroup v2, ~60 containers. RTX 3090 (driver 610.43) and the SAS HBA passed through; owns the `data01` pool (87 TiB raw, 72% used) and exports it over NFSv4.2. nginx on 80/443 is the front door for every service. Docker sets `iptables -P FORWARD DROP`. |
| GPU today | Already shared: llama-server 19.6 GiB and tdarr-ffmpeg 376 MiB on the one card, via Docker. |
| "harbor" VM | Not Harbor. A `data01_workers` host (re-read from `master` on 2026-11-17): nzbget, a CPU tdarr-node, ml-log-analyzer (two Redis, a Postgres, in no backup), mem0 (API, pgvector Postgres, ollama embedder, MCP bridge, a gateway nginx that is the origin for `mem0.terrac.com`) and archmap, both moved here from ocean. NFS `/data01` with a local 200G state disk over `/data01/services`, and a passed-through 870 EVO 2TB that holds NZBGet's scratch and mem0's Postgres (`/srv/nzbget/mem0`) (`docs/architecture/data01-workers.md`). |
| agentbox | Also runs axiomd as a compose project (`playbooks/individual/workers/axiomd.yaml`). Its place in this plan is an open item in the PRD. |
| registry-cache-01 | Pull-through caches for docker.io, ghcr, gcr, nvcr, gitlab. |
| `homelab-kube` repo | 2021-era kubeadm cluster config, 479 commits. Has an app-of-apps chart (`charts/argocd-applications`), sops-secrets-operator (GPG), cert-manager, MetalLB, Cilium, ingress-nginx, cloudflare tunnel ingress, kube-prometheus-stack, promtail, and charts for plex, tdarr, sonarr, radarr, lidarr, tautulli, overseerr, nzbget. |

Defects in `homelab-kube` to fix before reuse:

- `ocean-nfs-pvc.yaml` gives the 18T `/data01` PV `persistentVolumeReclaimPolicy: Delete`. Must be `Retain`.
- `charts/argocd-applications/values.yaml` sync policy has `prune: true` with `allowEmpty: true`: an empty render prunes everything. Must be `allowEmpty: false`.
- sops via GPG, promtail (end of life), chart dependencies four years stale.
- The app charts (plex, sonarr, radarr, lidarr, tdarr, tautulli, overseerr,
  nzbget) each depend on a `k8s-at-home` chart. That project was archived in
  August 2022; its chart index still answers but nothing in it has been
  updated since, and the `ghcr.io/k8s-at-home/*` images are frozen at 2021
  versions. See "App charts" below for what upgrading them means.

A note on etcd: the original reason for avoiding it was spinning disks. VM disks
are on consumer SSDs, not spinning disks. Consumer SSDs without power-loss
protection still give poor fsync latency under ZFS sync writes, so the decision
stands; the reason is fsync latency, not rotation.

## Architecture

### Three tiers

| Tier | What | Runs on | Managed by |
|---|---|---|---|
| 0: break-glass | DNS pair, DHCP, Proxmox, ZFS, NFS and the k3s server process on the ocean host, registry-cache, gh-runner, agentbox, ntfy and the dead-man watchdog (both on agentbox) | Plain VMs and hosts | VM definitions: `terraform-root`. Inside the VM: Ansible now, Salt later |
| 1: platform | Argo CD, cert-manager, secrets operator, Cilium Gateway, NVIDIA device plugin, NFS and local storage classes, kube-prometheus-stack, Alloy | The cluster | Argo CD |
| 2: apps | Every service that runs as a container today | The cluster | Argo CD |

Rule: anything you need to repair the cluster lives in tier 0. Nothing in tier 0
may depend on the cluster being up.

### One cluster, not a management cluster plus a workload cluster

One production cluster (D2). A second, management cluster doubles the
upgrade and maintenance load for one operator on two hypervisors, and it does
not protect against the failure that matters (the cluster is down and you
cannot reach anything), which tier 0 covers. Upgrade risk is controlled with
a standing rehearsal cluster (D51): one VM, built from the same playbook,
holding nothing. It is a second cluster to patch and upgrade, and that cost
is the point of it: each upgrade is paid for there before ocean sees it.

### Cluster shape

No new cluster VMs and no new hardware (D5). The cluster is the two VMs that
already do the work, each as it is configured today.

| Node | Host | Role | What changes |
|---|---|---|---|
| `ocean` | existing VM on node006 | k3s server, and the GPU and `/data01` node. Tainted so only workloads that tolerate it land there. | k3s installed beside Docker. The VM, its OS and its disks stay as they are. |
| `harbor` | existing VM on node005 | agent. Its Docker services (nzbget, tdarr-node, ml-log-analyzer, mem0, archmap) become pods on it. | k3s agent installed beside Docker. The VM, its OS and its disks stay as they are. |

The server runs on ocean because ocean is where nearly everything runs. A
separate server VM would keep the API answering through an ocean reboot, but
with ocean down there is almost nothing left for it to schedule, and it would
be one more VM to build, patch and back up. The tradeoff is real though: an
ocean reboot or drain takes the API down with it. While it is down, harbor's
pods keep running; only scheduling and reconciliation stop.

One exception: a node that boots while the server is down starts no pods,
because the kubelet learns what to run from the API. So harbor coming up
while ocean is down waits for ocean. Ocean itself has no such wait: its
kubelet and the server are the same process.

The server's state (SQLite) lives on ocean's root disk, which is on node006's
one boot SSD. Nothing mirrors that disk and there is no bay to add one, so the
nightly backup to GCS and a rehearsed restore are what protect it (R2).

**The rehearsal cluster.** Installing a CNI on a host that fronts sixty Docker
containers is not something to try first on that host. A standing VM
(`k3s-rehearsal`, on node005's existing `vmpool-storage`, same Debian major,
kernel line, Docker version and iptables backend as ocean) is built by the
same play and takes every risky step first: the Docker coexistence proof, the
three-upgrade drill, the deliberate break and restore, and from then on
every k3s and Cilium upgrade at least a week before ocean gets it. It is a
single-node cluster of its own, never a member of the production one, and it
holds nothing, so it can be deleted and rebuilt at will. Backout for the
whole idea is deleting it.

Being a second cluster, it needs its own of three things. Its k3s version is
its own variable, because it runs ahead of production. Its Argo CD reads its
own root in `homelab-kube` (`clusters/rehearsal` beside
`clusters/production`), so a chart version can be raised there first. And it
has its own sops age key and its own CA intermediate: the production
cluster's key never sits on a VM that is rebuilt at will, and the rehearsal
root carries only test secrets.

What it cannot prove, because it has none of them: the GPU and the NVIDIA
runtime, ZFS (the pool guard is simulated, and kubelet's view of memory with
an ARC in it is not), the NFS mount, and anything that takes two nodes (an
agent join, agent and server skew, Cilium's overlay between hosts). Those are
first seen on ocean in phase 3 and on harbor in phase 2, each in a
supervised window with its own backout. Whether a second rehearsal node is
worth it for the last group is an open item in the PRD.

- **Distribution**: k3s. **Datastore**: embedded SQLite via kine (single
  server, no `--cluster-init`, no etcd). HA control plane is an anti-goal for
  2027 (see PRD).
- **Bundled components**: k3s ships Traefik, ServiceLB, a `local-path`
  provisioner and metrics-server, and re-applies its own manifests for them
  when the server starts. All four are turned off at install
  (`--disable=traefik,servicelb,local-storage,metrics-server`). ServiceLB is
  the dangerous one: it claims host ports 80 and 443 for Traefik, and on
  ocean Docker's nginx holds those. The provisioner and metrics-server come
  back as Argo apps, so the `Retain` class below is not overwritten on the
  next restart. CoreDNS stays bundled.
- **Addresses and names**: the pod and service ranges are chosen in phase 1
  against the LAN, Docker's bridge ranges on both hosts and Cilium's own
  defaults, and written into `vars/vars_k3s.yaml` before the first install,
  since they cannot change afterwards and the cluster intermediate's name
  constraints depend on them (D39). Node names are the inventory names.
- **Version pinning**: one variable per cluster in `vars/vars_k3s.yaml`
  (`k3s_version` for production, `k3s_rehearsal_version` for the rehearsal
  cluster, which runs ahead).
  Install and upgrade plays are dispatch-only (added to the never-auto-apply
  list in `detect-impacted-playbooks.sh`). An upgrade is a PR that bumps the
  variable plus a deliberate dispatch, never a side effect of a merge.
- **Backup**: nightly `sqlite3 .backup` of `server/db/state.db` on ocean, plus
  the server token, `server/tls/` and the node password file
  (`/etc/rancher/node/password`; a rebuilt node that comes back under its old
  name without it is refused), to GCS beside the DB dumps. Restore is
  rehearsed in phase 1, on the rehearsal VM, two ways: onto a fresh build,
  which is the disaster path, and in place, which is the upgrade backout
  (next).
- **Data on ocean's root disk**: k3s keeps its image store and its database
  under `/var/lib/rancher` on ocean's root disk (129G, shared with the OS).
  Docker's image store looks to be on the pool (the `data01/docker` dataset;
  confirmed in phase 1), so this is new load on that disk: every
  migrated service's image lands there, the GPU images among them. Phase 1
  measures the free space and the image set before the install and sets
  kubelet's image garbage collection and eviction thresholds to fit. No
  service state goes there (Storage, below).
- **Upgrades**: upstream requires a drain before a minor kubelet upgrade, and
  draining ocean stops nearly every service on the fleet. Ocean is also the
  server, so a k3s upgrade is an ocean event whichever way it is cut. It runs
  in ocean's supervised monthly window (D49), where ocean is drained and
  rebooted anyway: ocean, then harbor's agent, which never runs ahead of the
  server. The rehearsal cluster takes the same upgrade at least a week
  before (R29). R23 measures the server.
- **Upgrade backout**: k3s has no supported downgrade, so backing out an
  upgrade on ocean is not the fresh-build restore. It is in place: stop k3s,
  put the previous binary back, restore the database copy taken just before
  the upgrade, start, and run the Docker gate. That path is drilled on the
  rehearsal cluster in phase 1 beside the fresh-build restore.
- **Images**: k3s `registries.yaml` mirrors docker.io, ghcr.io, gcr.io and
  nvcr.io through registry-cache-01. A mirror is a preference, not a
  dependency: containerd falls back to the upstream registry when the cache is
  down, and phase 1 tests that by stopping the cache and pulling.

### Networking

- **CNI**: Cilium (terrac, PRD D15). "Advanced network control" is one of
  the reasons for doing this, and swapping the CNI later means rebuilding the
  cluster, so the upgrade drill runs on Cilium from day one.
  - k3s installs with `--flannel-backend=none` and
    `--disable-network-policy`; Cilium enforces NetworkPolicy.
  - Kube-proxy replacement (`--disable-kube-proxy`) is on, because Cilium's
    L2 announcements need it (D10). Docker coexistence (below) is tested
    with it on: that is where Docker's iptables rules and Cilium's eBPF
    datapath meet. If it cannot pass the Docker gate, kube-proxy comes back
    and MetalLB announces the pool.
  - Cilium is an add-on in the upgrade drill: its supported Kubernetes range
    is checked on every hop, and a Cilium minor upgrade is part of at least
    one hop.
  - Hubble on, so per-flow visibility is available when a NetworkPolicy
    blocks something.
- **A load-balancer pool** (terrac, D10): `192.168.1.64/27`. It is outside
  the DHCP pool (`.150` to `.239`, about a quarter used: a peak of 23 of 90
  leases over 30 days, measured 2026-11-19), so the DHCP pair is not
  reshaped. A ping and ARP sweep on 2026-11-19 found nothing in it; the only
  claim on it in the repo is the stale `*.cluster.local` wildcard at `.64`,
  which phase 0 deletes. The block is a clean CIDR, which is what a BGP
  advertisement will want when that comes, after this plan.
  - Cilium hands out the addresses (LB IPAM) and answers ARP for them from
    one node at a time (L2 announcements). MetalLB in L2 mode is the written
    fallback, taken only if Cilium's way fails the Docker gate on the
    rehearsal cluster. The MetalLB chart in `homelab-kube` is archived with
    the rest and comes back only then.
  - The rehearsal cluster owns the top `/29` (`.88` to `.95`), production
    the rest. Two clusters answering ARP for one address is the failure to
    avoid, so each cluster's pool object names only its own range.
  - New VMs (the rehearsal VM, the second runner) still take an address and
    a record from the existing static range like any other host.
- **The Gateway** (D20) is Cilium's Gateway API on a fixed address from the
  pool, on 80/443 from phase 1, on the rehearsal cluster through the drill
  and then on production. It binds no host port, so it never competes with
  Docker's nginx on ocean, and it is not tied to one node: whichever node
  announces the address takes the traffic. That is its final form from day
  one, so it goes through the whole upgrade drill. There is no NodePort per
  service, so no second port registry grows beside the one being retired.
  - k3s and Cilium bind host ports of their own (the API, the kubelet,
    Cilium's health, Hubble and metrics ports, the overlay's UDP port). Before
    the install each is checked against `vars/vars_service_ports.yaml` and
    against what is listening on ocean and harbor, and registered there. A
    DaemonSet that wants a port a Docker container holds (node-exporter on
    9100 is one) waits until that container retires.
- **Names** (terrac, D55). No wildcard, anywhere. Today `*.home` points at
  ocean's nginx, so a client with a stale or wrong answer talks to ocean
  when it meant another host, and the 2021 cluster did the same thing with
  `*.cluster.local`. Both go.
  - Phase 0 removes the `*.home` wildcard. First every name nginx serves
    gets its own record to ocean, taken from nginx's `server_name` list and
    checked against the resolver's query log for names only the wildcard
    was answering. The `*.cluster.local` zone and its record at `.64` are
    deleted in the same pass.
  - A cluster service is `<svc>.home`, a real record in PowerDNS that a
    laptop or the mac mini resolves like any host. `cluster.local` is
    CoreDNS's, inside the cluster, and is never served on the LAN.
  - ExternalDNS, a tier 1 platform app, writes those records through the
    PowerDNS API from each service's HTTPRoute, with a 60 second TTL so a
    cutover or a backout is not held up by caches. Names that have not
    moved yet are `DNSEndpoint` objects in `homelab-kube` pointing at ocean:
    in phase 1 the static service records from phase 0 are handed to
    ExternalDNS once, unchanged, so from then on one writer owns every
    service name and a cutover is one commit that swaps a `DNSEndpoint`
    for a route.
  - The fence. ExternalDNS is the one exception to "only terrac merges DNS"
    (D25), so it is kept narrow. It marks what it creates with ownership
    TXT records and changes nothing without one, so it cannot overwrite a
    host record. CI in `homelab-kube` refuses a route or `DNSEndpoint`
    whose hostname is in the address table. Its API key lives in sops and
    is the only DNS credential in the cluster. The rehearsal cluster's
    ExternalDNS has its own owner id and only writes names under
    `rehearsal.home`.
  - What has to be true first: a record written through the API reaches
    both DNS servers. This spec says PowerDNS is primary on dns02 with an
    AXFR secondary on dns01; `scripts/dns-drift-check.sh` says dual-primary
    with no replication. Phase 0 finds out which, and if it is the second,
    fixes replication before ExternalDNS exists.
  - Tier 0 does not come to depend on the cluster (A1): a record stays in
    PowerDNS when ExternalDNS or the whole cluster is down. Only changes
    stop.
- **How a service's traffic moves.**
  - A LAN name moves at its service's cutover: the record goes from ocean
    to the Gateway address, and nginx is out of that service's path from
    then on. The Gateway terminates TLS with a cert cert-manager issued
    before the window. Backout is the commit reverted; clients follow
    within the TTL.
  - A public name moves the same way, at the same cutover, in the same
    commit (terrac, D20). The cluster has one Cloudflare tunnel of its own,
    with `cloudflared` as a Deployment, a replica on each node, and a single
    catch-all rule that sends everything to the Gateway. That rule is
    written once in `terraform-root` and no cutover touches it. It names
    the Gateway's in-cluster Service, not its LAN address, so the public
    path does not depend on the L2 announcement.
  - The tunnel never points at a host or a pod, which is what makes a pod
    easy to move (terrac). Cloudflare reaches any `cloudflared` replica,
    the Gateway matches the hostname to a route, and the route's Service
    finds the pod on whichever node it runs. A pod changing node changes no
    tunnel config, no DNS record and nothing in `terraform-root`. With a
    node down, Cloudflare uses the other replica and the surviving node
    announces the Gateway's LAN address. What ties a pod to a node is its
    state, the GPU or the pool's local mount (Storage, below), never its
    names. A public
    name is a CNAME in Cloudflare pointing at a tunnel, so moving a service
    is pointing its CNAME at the cluster tunnel instead of ocean's. A second
    ExternalDNS instance, with the Cloudflare provider, writes that record
    from the same route. Backout is the commit reverted.
  - The reason for ExternalDNS here, and not a `terraform-root` PR per
    cutover: a service and its names then live in one place, and nothing
    has to be coordinated across two repos after the move. Its token is
    scoped to DNS edit on the zones in use. That is no new exposure: ocean
    holds such a token today for certbot
    (`files/ocean-certbot/cloudflare.ini.j2`), and cert-manager needs one
    in the cluster for Let's Encrypt regardless.
  - The handover mirrors the LAN one. In phase 1 each public service
    record leaves `terraform-root` (removed from state, not destroyed) and
    becomes a `DNSEndpoint` still pointing at ocean's tunnel, so no name
    changes its answer and one writer owns it from then on. The tunnels
    themselves, the Access apps and every record that is not a cluster
    service (mail, apex, verification) stay in `terraform-root`.
  - The catch-all means any hostname pointed at the cluster tunnel reaches
    the Gateway, which answers only names that have a route. So "a route
    with a public hostname is public", and CI in `homelab-kube` refuses a
    public hostname on a route that does not carry an explicit label
    saying so.
  - At the end (W7) nothing is left on ocean's tunnel or its nginx: the
    last names move, then both retire, with certbot. Every route has
    carried traffic since its own cutover.
  - nginx does more than proxy today: `files/nginx-compose` carries 3
    `limit_req`, 5 `client_max_body_size`, 6 websocket upgrades and 1 IP
    allowlist. Each needs a Gateway or Cilium equivalent, proven on a test app
    in phase 1, and the migration table records which ones a service uses. A
    feature with no equivalent is settled at the phase 1 gate, not found at
    W7 (D50): a Cloudflare rule in `terraform-root` for a public vhost,
    an nginx pod behind the Gateway for a LAN-only one, or dropping it.
  - Client addresses. A LAN client now reaches the Gateway directly, so a
    rate limit or allowlist keyed on client IP is proven at that service's
    own cutover, provided the Gateway's service keeps the source address
    (`externalTrafficPolicy: Local`), which phase 1 checks. A public
    request arrives from `cloudflared`, so the client's address is in
    Cloudflare's header, as it is behind nginx today; a rule keyed on it has
    to read that header, and phase 1 proves one. W7 has its own 14-day soak
    before Docker is removed.
  - Not ingress-nginx: the project was retired in March 2026 and gets no more
    fixes. The `ingress-nginx` chart in `homelab-kube` is archived, not reused.
    Cilium is already the CNI, so its Gateway is one less component.
- **Public traffic** today arrives through `cloudflared` on ocean, a host
  service whose ingress rules are remote config in `terraform-root`. It
  keeps serving every public name that has not moved, and retires at W7.
  The cluster tunnel above is a second, separate tunnel. Not a tunnel per
  service, and not the `cloudflare-tunnel-ingress-controller` (its chart in
  `homelab-kube` is archived): many tunnels and credentials to keep, and
  the controller writes tunnel config that `terraform-root` also owns.
- **hostNetwork pods** where the app needs the LAN: Home Assistant (discovery),
  Plex (GDM discovery, real client IPs for local-network detection), avahi
  and netatalk (mDNS advertisement, AFP).
- **Ports lockdown**: a default-deny policy per app namespace, with explicit
  allows. Traffic that arrives through the Gateway carries Cilium's `ingress`
  identity, and a hostNetwork pod's carries the host's; a plain Kubernetes
  NetworkPolicy has no selector for either, so the allow for "from the
  Gateway" is a CiliumNetworkPolicy. The reference chart carries both
  kinds, and phase 1 proves a default-deny namespace still serves through the
  Gateway. `vars/vars_service_ports.yaml` entries retire with their
  service. Limit: NetworkPolicy does not apply to `hostNetwork` pods (Plex,
  Home Assistant, the Gateway). Those, and the node's own ports, are locked
  down with Cilium's host firewall, turned on in audit mode first and enforced
  only after a clean week, because a wrong host rule can drop SSH (hard limit:
  port 22 rules are terrac's to approve).
- **Pod Security Admission**: app namespaces run at `baseline`. A namespace is
  `privileged` only for a reason listed in its chart (hostNetwork, GPU, a
  host device). That is why app state on ocean uses static local PVs and not
  raw `hostPath` (Storage, below): `baseline` forbids `hostPath` volumes.

### Storage

The `agents.md` rule "Nothing with a write-ahead log goes over NFS" carries over
unchanged. SQLite and Postgres state (the *arrs, Plex, Tautulli, Bazarr,
Grafana's MySQL, Prometheus TSDB, Loki) must be on a node-local disk.

Which node a service's state lives on is decided by three things (terrac,
D14), and the migration table records the answer per service:

- whether it keeps a SQLite database or anything else with a write-ahead
  log, which NFS can corrupt. harbor mounts the pool over NFS, so a pod
  there can reach `/data01` and nothing but this rule stops a chart from
  putting such state on it. That state goes on a local disk of the node the
  pod runs on, always;
- how much disk latency it tolerates;
- how much bandwidth it moves to and from the pool. A service that streams
  or rewrites large files belongs on ocean, beside the disks, not on
  harbor across the network.

The pod then follows its state, by role label, never by hostname.

| Kind | Where | How |
|---|---|---|
| Media (read/write, hardlinks) | `/data01` | One static NFS PV of the whole pool, `ReadWriteMany`, `Retain`, mounted on harbor, which is an NFS client of the pool today. On ocean, a static local PV of `/data01` (no NFS loopback). One mount of the whole pool so *arr hardlinks do not hit `EXDEV`. A PV binds one claim, so each consuming namespace gets its own PV onto the same path. |
| App state on ocean | `/data01/services/<svc>` | A static local PV pinned to ocean, pointing at the same directory the Docker container uses today. Zero data copy at cutover. Not raw `hostPath`, so the namespace can stay at `baseline`. |
| App state on harbor | `/data01/services/<svc>`, which on harbor is its local state disk | A static local PV pinned to harbor at the directory the Docker container uses today, so zero data copy there too. Volumes that are new with the move (CloudNativePG, Redis) come from the `local-path` provisioner on the same disk, through a class with `reclaimPolicy: Retain`. mem0's Postgres is the exception: its data is on the passed-through 870 EVO today, and its CloudNativePG volume is a static local PV on that disk. |

A trap on harbor. The host mounts NFS at `/data01` and then its local disk
over `/data01/services`, and a Docker bind mount of the host path sees both.
A pod's NFS volume is a fresh mount of the export, so inside the pod
`/data01/services` is ocean's directory over NFS, not harbor's disk. A pod
that kept its state at that path through the media mount would be writing a
database to NFS (A2). So on harbor a pod never reaches its state through the
media mount: its state PV is mounted at its own path, and the phase 2 test
pod writes a file there and confirms on the host that it landed on the local
disk.

`local-path` is for harbor only. Its provisioner writes under the k3s data
directory, which on ocean is the root disk that D26 treats as disposable, so
the provisioner's node map gives ocean no path and a claim that lands there
fails to provision. State on ocean is always a static local PV on the pool.

Known limit of the media mount: the pool is one dataset (`data01`, plus
`data01/docker`; `zfs list` on 2026-11-15), and `complete`, `incoming`,
`media` and `services` are sibling directories in it. Hardlinks need
downloads and library under one mount, and their only common parent is the
pool root. So every pod with the media mount can also reach
`/data01/services/*`, and with one shared `media` uid it could read other
apps' databases too.

The mount stays whole and the isolation comes from ownership (D44).
Each service runs as its own uid, allocated from one registry file in
`homelab-kube` in the managed range above the fleet ids (terrac 2000, media
2100), and its state directory is owned by that uid at `0700` (on
harbor, the `local-path` volume directories likewise). The `media` gid
stays as a supplementary group on pods that write the media tree, so
hardlinks and shared library access keep working, on one condition: with
`fs.protected_hardlinks` a process may only hardlink a file it does not own
if it can read and write it. So downloaders write with umask 002 and the
media directories are setgid `media`, or *arr imports fall back to copies or
fail. That is tested with one downloader and one *arr before the first
service changes uid. The rule covers new files only. Files already in the
tree keep the mode they were written with, so before the first uid change
the tree is audited for files the `media` group cannot write, and they are
fixed in one supervised pass, or old downloads stop hardlinking the day
their *arr gets its own uid. The change is not part of
a cutover: backout needs the Docker container to read the state under the
old uid. It lands after the service's soak, as a `homelab-kube` PR that
chowns the directory in a one-shot init container and switches the pod's
`runAsUser`. Until then the exposure above stands for that service.

Two guards the charts and hosts must carry:

- **No pod may start on ocean before the pool is mounted.** A `hostPath` into
  an unmounted `/data01` writes onto the root disk, and the data disappears
  from view when the pool mounts (the trap `agents.md` describes). So the k3s
  unit on ocean gets a systemd drop-in that refuses to start unless
  `/data01` is a mountpoint (the same idea as today's Docker guards), and the
  state PVs point at `/data01/services/<svc>`, which does not exist when the
  pool is absent, so the mount fails instead of writing to the root disk
  (confirmed on the rehearsal VM before k3s goes onto ocean). On ocean that
  unit is the server too, so the API stays down while the pool is absent.
  That is the right way round: an ocean without its pool should schedule
  nothing, and the watchdog reports it.
- **`local-path` volumes must be `Retain`.** The provisioner's stock
  StorageClass reclaims with `Delete` (confirm at build), and Argo prunes, so
  an app dropped from the app-of-apps would take its state with it. The
  cluster's default class is a `local-path` class with `reclaimPolicy:
  Retain`; the stock class is removed. CloudNativePG volumes use it too.
- **`local-path` volumes are tied to a node name.** Their PVs carry node
  affinity on `kubernetes.io/hostname`. Whatever replaces a VM one day has to
  rejoin under the same node name with the same state disk, or its volumes
  never bind. That is a constraint on the later distro-lifecycle plan, not
  work in this one.

The NFS export on ocean only admits listed client IPs with `root_squash`.
harbor is already in `data01_workers`, so its pods mount `/data01` through
the export it uses today, by numeric uid with the `media` gid (one uid per
service once R26 has been applied to it).

### GPU

- The 3090 is passed through to ocean, and a PCI device can only be passed to
  one VM, so every GPU workload stays on ocean. Ocean is a cluster node from
  phase 1 (D4); the GPU half gets its own go/no-go in phase 3, before any GPU
  service moves.
- NVIDIA device plugin only, with time-slicing (`replicas: 8`). Not the GPU
  operator: the driver is already installed and managed on the host.
- k3s detects `nvidia-container-runtime` and registers it; pods use
  `runtimeClassName: nvidia`.
- Time-slicing gives the same sharing Docker gives today: no VRAM isolation, no
  fault isolation. VRAM contention (llama.cpp holds ~20 GiB) is the real limit,
  and it is unchanged by the migration.
- The concurrent NVENC session cap on GeForce drivers is shared by Plex, Tdarr
  and (later) Jellyfin. Measured, not assumed, in the phase 3 gate.

### Docker and k3s side by side on ocean

Ocean runs Docker and k3s (its own containerd) together from phase 1 until
Docker is drained at the end of phase 5. harbor does the same through phase 2.
Known risks: Docker's `FORWARD DROP` policy against pod traffic, two
container runtimes each needing the NVIDIA runtime configured, host port
collisions, and Cilium replacing kube-proxy on a host where Docker writes
its own iptables rules. Mitigations:

1. Prove the exact coexistence on the rehearsal VM first: with Docker there,
   run a bridge-network container with a published port and pods, then verify
   pod-to-pod, pod-to-service, the Gateway's address from another host, **a
   Docker container reaching the Gateway's address from the host that is
   announcing it** (any Docker service calling a migrated peer by name
   takes exactly this path; it is
   where Cilium's datapath and Docker's bridge meet), **a pod reaching a Docker container's published
   port on its own host** (a migrated pod calling a peer by its front-door
   name lands on Docker's nginx this way, and so do Alloy's writes to
   Prometheus and Loki and a Postgres import), and the published Docker port
   still reachable from the LAN. Reboot and verify all of it again. The rehearsal VM keeps
   Docker, so every later upgrade is rehearsed against the same mix.
2. Install k3s on ocean in a maintenance window, with a written backout
   (`k3s-uninstall.sh`, restart docker). harbor joins the same way in phase 2
   (`k3s-agent-uninstall.sh`). The uninstall script removes k3s, not what
   Cilium left in the kernel (its BPF programs, interfaces and iptables
   chains), so the backout also runs Cilium's cleanup and is not called done
   until the Docker gate passes; if it does not, a reboot is part of it. The
   whole backout is run on the rehearsal VM before ocean's window. It is
   only a clean backout until W1: after that, uninstalling takes the migrated
   services with it.
3. If coexistence cannot be made clean on the rehearsal VM, k3s does not go
   onto ocean and the plan stops for a re-plan. k3s's `--docker` mode (one
   runtime, through cri-dockerd) is the first thing that re-plan would look
   at, but it is not a quiet fallback: pods become Docker containers, so
   Docker could not be removed at the end of phase 5 without a second
   runtime switch.

### The Docker gate

Docker, and the ability to run containers under it, is never broken by the
k3s rollout (R32). That holds on each host from the first k3s step there
until Docker is removed from it: harbor through phase 2, ocean through
phase 5. It is not enough for it to be true; every step confirms it.

"Docker works" is one committed check, the same script every time, so that
forty windows do not each invent their own idea of it. It runs on the host
and from a second host, and it passes only if all of these do:

- The daemon answers, and every container that was running before the step
  is running after it, compared against a list taken just before. Services
  the step moved on purpose are the only allowed difference.
- A new container can be created: pull an image through the registry cache,
  run it on a bridge network with a published port, and reach that port from
  the LAN.
- That container can resolve a name, reach another container on its network,
  reach the internet, and reach the Gateway's address from its own host. A
  pod can reach that container's published port.
- On ocean, a container started with the NVIDIA runtime sees the GPU.
- No Docker service's blackbox probe is red that was green in the "before"
  run. With sixty services one is often red for its own reasons, and that
  must not block every window; a probe already red before the step is named
  in the run's output and left out of the comparison.

Where it runs, before and after each:

- the k3s install on ocean and the agent join on harbor;
- every k3s upgrade, and every Cilium upgrade or datapath change (kube-proxy
  replacement, the host firewall going from audit to enforce);
- every upgrade of the held Docker, containerd and NVIDIA container toolkit
  packages, which restart the daemon and rewrite its iptables rules under
  Cilium;
- every reboot of a host that has both runtimes;
- every service cutover, in both directions.

That list is why no reboot of a dual-runtime host is unattended. Ocean was
never in kured's rota (D22), and harbor stays out of it until its Docker is
gone (D54). `unattended-upgrades` still runs on both, since it installs
without restarting anything the gate covers: Docker, containerd, the NVIDIA
toolkit, the kernel and ZFS are held.

A failed "before" means the step does not start: the host was already
unhealthy and the step would only muddy why. (The probe line cannot fail a
"before" run; the other lines can.) A failed "after" halts the
workflow, alerts, and the step's own backout runs. No automatic rollback
beyond that, per the standing default. The rehearsal VM runs the same gate
through its coexistence proof and all three upgrades, so the script has been
exercised, and has failed on purpose once, before ocean depends on it.

### Cutover of one service (the unit of migration)

1. Chart and values in `homelab-kube`, Application added to the app-of-apps but
   scaled to 0, with its CronJobs at `suspend: true`. A CronJob runs at zero
   replicas, and would write to state Docker still owns.
2. Secrets moved: added to sops in `homelab-kube`. Removed from the vault in the
   same PR that retires the Ansible play (never present in both once the play is
   gone; see PRD R7).
3. Window: retire the Docker side ("Staying down", below), scale the
   Deployment to 1 and unsuspend its CronJobs, move the name to the Gateway
   (the route replaces the service's `DNSEndpoint`, and ExternalDNS moves
   the record, the public one too if it has one),
   verify with the service's health check and blackbox probe. Then
   check that its alert rules and dashboard panels still return series. The
   Docker gate runs before and after: moving one service must not disturb the
   ones still in Docker.
4. Soak: the play, the compose files and the stopped container stay in place
   for the 14-day soak (D45). Backout during the soak is the window in
   reverse: scale to 0, bring the compose project back up through its play,
   the name back to ocean. It works because the pod runs the same image against
   the same state directory, so the service's image is frozen for the soak:
   Renovate skips it and a home-grown release waits. An image that has to
   change in the soak ends the backout for that service, and is recorded as
   doing so. A Postgres service is different, because its data was copied,
   not shared: the Docker database stops at the cutover and goes stale.
   `back` for one of those first takes a logical dump of the cluster
   database and restores it into the Docker container with `db-restore.sh`,
   then starts the app. That is slower than five minutes and is drilled on
   ml-log-analyzer before mem0 depends on it.
5. Retire, after the soak: the Ansible play, its `files/<svc>`, its port
   entry, its vault keys, its `docker-refresh@` and `docker-restart@` timers
   and any host timer that belongs to it are removed in one `homelab` PR.
   This is the point of no return, which is why it is not in the window.

The window runs as one dispatch workflow in `homelab` with a direction
(`forward` or `back`), holding the per-host deploy lock ("One lock per host",
below). The PRs in both repos are written and reviewed beforehand; the
workflow merges them in order and gates each step on the last. Three things
follow from that, and the builder settles the mechanics in the phase 4 plan:

- The workflow is the applier. It holds the host's lock, so a `main-apply`
  set off by its own merge would fail on that lock. Its merges are marked so
  `main-apply` skips them, and the workflow runs the service's play itself.
- "Scale to 1" is a commit. Argo auto-syncs from git, so a replica count
  changed on the cluster is put back. The workflow merges the `homelab-kube`
  PR that sets the replica count and unsuspends the CronJobs, then asks Argo
  to sync. `back` merges the revert.
- So it needs two credentials: the Argo token that can only sync named apps
  (Cluster access, below), and a GitHub token that can merge in `homelab`
  and `homelab-kube`.

The reason for a workflow at all: Three hand-merged PRs and a `main-apply` queue do not fit a
five-minute target, and forty hand-run windows will not all be run the same
way. The first W1 cutover drills `back` on purpose.

**Staying down.** `docker compose down` does not keep a migrated service's
container down for 14 days, and a container that comes back writes to the
same state as its pod. Two things revive it. Any inventory or
`group_vars/all` change replays the site orchestrators, which run the
service's play. And host units restart what they watch: `plex-autoheal`,
`docker-restart@<project>`, a compose unit with `Restart=`. So `forward`
commits a per-service flag (`migrated_to_k8s: true`) that turns the play into
"ensure down": compose project down, its units and timers disabled and
masked. An orchestrator replay then keeps it down instead of starting it.
`back` flips the flag. The flag goes away with the play at retire.

Two things move with a service that are not in its chart:

- **Its callers.** Many apps keep their peers' URLs in their own database
  (the *arr download clients, overseerr's sonarr and radarr entries). Before
  W2 those are repointed to the front-door names (`<svc>.home`, which
  resolves to ocean's nginx until the peer's cutover and to the Gateway
  after), so a peer moving between Docker and the cluster changes nothing for
  its callers. The migration table's "inbound dependants" column is the list.
- **Its alerts and panels.** Rules and dashboards keyed on Docker labels
  (container name, compose project, cadvisor series) stop matching when the
  container becomes a pod, and a rule with no series is silent, not firing.
  The cutover PR updates them, and step 3 checks they return data.

A cutover can touch up to four repos, in this order: `terraform-root` (only if
a bucket or an Access app changes), the service's own repo (only for home-grown
images, to switch its dispatch target), `homelab-kube`, then `homelab`.

Never run the container and the pod at the same time against the same state
directory. Downtime per service is the stop-to-healthy window (target under 5
minutes), not zero.

### One lock per host, across repos

`deploy-ocean` is a GitHub Actions concurrency group, and those look to be
scoped to one repository (the REST API addresses them per repo; confirmed in
phase 0). Through the migration three things apply to the same hosts from
different repos: `homelab`'s Ansible, `homelab-salt`, and the cutover
workflow. So every apply, from any repo and either runner, takes `flock` on a
lock file on the target host for as long as it runs, and fails fast naming
the holder if it is taken. `deploy-ocean` stays as the queue inside
`homelab`.

### Cluster access

- terrac: a cluster-admin kubeconfig on the laptop and on agentbox, reaching
  the API on ocean by IP. That is the break-glass path when Argo or the Gateway
  is the thing that broke.
- Agents: a read-only ServiceAccount by default (get, list, watch, logs; no
  Secrets). Changes go through `homelab-kube` PRs. Recorded in
  `docs/architecture/agent-autonomy.md` with the merge policy.
- CI: an Argo CD token scoped to sync and wait on named apps, for the cutover
  workflow. Argo polls git on an interval (about three minutes by default,
  to be confirmed), which does not fit a five-minute window, so the workflow
  asks for the sync itself. GitHub cannot reach a webhook inside the LAN.

### Ocean to k8s node: the full strategy

Ocean runs Docker and k3s side by side for the whole migration, and services
move one at a time (D4). That is the least downtime per service: each cutover
is one container stopping and one pod starting on the same host, against the
same state directory and the same GPU.

The k3s install is in place and additive: no OS upgrade, no new VM, no disk
moved. Ocean stays the VM it is today for the whole of this plan, and ends it
as the same VM with Docker gone (D13). What replaces that VM, and when, is the
later distro-lifecycle plan (D11).

**Step 1: prepare (no downtime).**

- Prove Docker and k3s coexistence on the rehearsal VM (see above).
- Inventory every ocean container into a migration table: class (stateless,
  WAL-bearing state, GPU, hostNetwork, platform), state path, ports, inbound
  dependants, nginx vhost features in use (rate limit, body size, websocket,
  allowlist), host devices, external repo that deploys it. This table is the
  wave plan.
- Inventory the host's own units and timers the same way; containers are not
  the whole of ocean. Probed 2026-11-10, each needs a named destination:

  | Host unit or timer | Destination |
  |---|---|
  | `nfs-server`, `zfs-zed`, `zpool-metrics`, `sas-phy-metrics`, `farm-metrics`, `nvidia-persistenced`, `fail2ban` | Stay on the host for good (Salt). They are the storage and GPU node itself. |
  | `cloudflared` | Stays on the host (Salt); config in `terraform-root`. |
  | `exim4` smarthost | Stays on the host for good (Salt). It is Alertmanager's route for reporting node005 down, so it must not move to agentbox (see dead-man alert). |
  | `cloudflare_ddns`, `certbot-renew` | Retire into the cluster: a CronJob and cert-manager. |
  | `db-backup` | Per-database CronJobs; Postgres also gets WAL archiving (D35). |
  | `plex-autoheal`, `kometa-metrics`, `mem0-metrics`, `tdarr-playback-window`, `wordpress-cron`, `wordpress-update`, `audible-download` | Move with their service, as CronJobs or sidecars, in that service's cutover. |
  | `promtail` | Replaced by Alloy. |
  | `avahi-daemon`, `netatalk` | Pods on ocean in W5, `hostNetwork` (mDNS needs the LAN); netatalk mounts its share from `/data01` (D41). |
  | `glances` | Dropped (D41). |
  | host exporter containers (node, smart, process, fail2ban, cadvisor, nvidia-gpu, dcgm) | DaemonSets on cluster nodes. On tier 0 VMs they stay as they are, owned by Salt. |
- Node labels and taint chosen up front, so pods pin to a role, not a host
  name: label `homelab.role/data01=local`, `homelab.role/gpu=rtx3090`; taint
  `homelab.role/ocean=true:NoSchedule`. Workloads that belong on ocean tolerate
  the taint and select on the labels, so a later replacement VM needs no
  chart changes (D14).

**Step 2: install (phase 1, one maintenance window, about 30 minutes).**

0. Before the window: cap the ZFS ARC on ocean and set kubelet's reserved
   memory and eviction thresholds to match. Only node005 has a cap today
   (`playbooks/individual/base/io_cpu_ups.yaml:9`). Kubelet counts ARC as
   used memory, not as cache it can reclaim, so an uncapped ARC on a 366 GiB
   host can look like memory pressure and get pods evicted. The rehearsal VM
   has no pool and cannot show this, so the numbers are set from ocean's own
   ARC history and watched through the soak.
1. Hold `docker-ce`, `containerd.io`, the NVIDIA driver and container
   toolkit, and the ZFS packages (`apt-mark hold`) for the duration of the
   overlap. Held means kept out of unattended upgrades, not frozen: they move
   in the supervised monthly window with the kernel, so the front door does
   not sit on a Docker that is six months stale.
2. Configure `nvidia-container-runtime` for k3s's containerd (Docker's config
   stays as is).
3. Install k3s (the server, which carries ocean's kubelet) at the version
   the rehearsal VM ended its drill on, with the labels and taint above.
4. Verify: the Docker gate passes (it also ran before step 1), and Docker's
   `FORWARD DROP` has not broken pod traffic. Then reboot ocean and verify
   both again.
5. Backout: `k3s-uninstall.sh`, restart Docker, re-verify probes.

**Step 3: GPU gate (phase 3).** Device plugin with time-slicing; run an NVENC transcode
pod and a CUDA pod while llama.cpp still runs in Docker. Record the NVENC
session cap. This is the go/no-go for the GPU half of the plan. A no-go
leaves the GPU services in Docker and W4 unplanned until the cause is
understood; nothing about the rest of the migration waits on it except
removing Docker from ocean.

**Step 4: waves (phase 4).** One service per cutover, per the cutover procedure
above. Waves, lowest risk first:

| Wave | Services | Notes |
|---|---|---|
| W1 | app exporters, homepage, languagetool, mtr, ndt | stateless; learn the name move and its backout. Starts after the phase 1 soak, and waits for neither harbor nor the GPU gate. |
| W2 | sonarr, radarr, lidarr, prowlarr, bazarr, overseerr, tautulli, kometa, jellystat | WAL state on local PVs on ocean; hardlinks need the one whole-pool mount. Service-to-service URLs move to front-door names first. jellystat is ocean's one Postgres (D35). |
| W3 | photonic_inventory, my_ta_jose, paia, etrading_boy, blog_saetnere_com_wp, and the Shape B static sites | external repos; each repo switches its deploy pattern in the same cutover. The static sites are served by the nginx that retires in W7, so they move here as small nginx images (D23). |
| W4 | tdarr server, llama.cpp, open-webui, plex | GPU; plex in quiet hours. The desktop tdarr nodes are outside the cluster and keep pointing at the server's front-door name. |
| W5 | homeassistant, avahi, netatalk | hostNetwork |
| W6 | prometheus, loki, grafana, alertmanager, blackbox, pushgateway, karma, and the host exporters (node, smart, process, fail2ban, cadvisor, nvidia-gpu, dcgm) as DaemonSets | this is phase 5. Blackbox stays in Docker until here: it verifies every earlier cutover, so it must not run on the thing it checks. The node-exporter DaemonSet mounts the host's textfile directory, since the drift, patch and pool metrics are written there by host timers. |
| W7 | nginx | The last names leave ocean, then ocean's tunnel, nginx and certbot retire. cert-manager has issued every cert since each service's own cutover. |

The services that live on harbor (nzbget, tdarr-node, ml-log-analyzer, mem0,
archmap) are not in these waves. They move in phase 2, on harbor, through
the same cutover procedure. mem0 is the agents' shared memory, so it is cut
over with no agent run in flight; its public name reaches a container port
on harbor through ocean's `cloudflared` today, and moves to the cluster
tunnel with the rest of it.

At the end of W7, Docker on ocean is empty and is removed. That is where this
plan leaves ocean. `fail2ban` stays, but its nginx jails lose their log
source when nginx retires; W7 decides per jail whether a Gateway access log
replaces it or the jail is dropped.

### OS patching (majors are a later plan)

This plan covers patches. OS majors are out of it, for the reason at the end
of this section.

**Patches (packages, kernels, CVE fixes): continuous, automated, rolling.**

- `unattended-upgrades` for the Debian security origin on every VM. It
  installs; it does not reboot.
- Held packages on the passthrough VMs and anything that builds a kernel
  module: NVIDIA driver, ZFS (`zfs-dkms`, `zfsutils-linux`), the kernel itself.
  Those move in the supervised monthly window, and the window checks
  `dkms status` shows both the NVIDIA and ZFS modules built for the new kernel
  **before** rebooting. A ZFS module that failed to build means a pool that
  does not import on boot.
- Reboots on k8s nodes: kured, Wednesday 03:00 to 05:00 Pacific (D38), clear
  of the Sunday 08:00 to 14:00 image refresh, one node at a time, drain first.
  Ocean is never in the rota (next). harbor enters it only when its Docker
  is gone, at the end of phase 2 (D54); until then it reboots in a
  supervised window with the Docker gate, like ocean. So kured's first
  reboots are of the rehearsal cluster's node.
- **Ocean is not in kured's rota.** An unattended reboot would skip the
  `dkms status` check above, take the API down, and during the overlap take
  down every Docker service too. Ocean reboots only in the supervised monthly
  window.
  Its `reboot-required` alert fires at 35 days, not the fleet's 14: a
  monthly reboot would trip 14 every month and teach terrac to ignore it,
  and 35 says a window was missed. Kernel live patching is not the answer
  here (D22): nothing free supports Debian, it would not cover the ZFS and
  NVIDIA modules, so the window stays regardless, and it would be patching
  running kernel code on the host that owns the pool.
- Reboots on tier 0 VMs: one at a time in the same window, driven by the
  patch play (Ansible now, Salt later). The DNS pair patches in the same
  window, one of the pair per run, through the pair gates in "DNS and DHCP
  under Salt": the second is never touched until the first is back and
  answering, and neither is touched while the pair is already degraded.
- Proxmox hosts: monthly patch window, hand-run. A hypervisor reboot takes
  every VM on it down (no shared storage, and passthrough VMs cannot
  live-migrate), so it is scheduled, and it takes one of the DNS pair with
  it, so it runs only when the pair pre-flight is green. PVE major upgrades
  are in place by necessity (bare metal) and get their own runbook, one node
  at a time. Both nodes are on PVE 8 (8.1 and 8.4), which reached end of life
  in August 2026, so both are unsupported today. Per D48,
  each is upgraded in place to PVE 9 in phase 0 by Proxmox's documented
  8-to-9 path, one node per window with the pair pre-flight green, node005
  first. That path starts from the last 8.x release, so node005 goes from
  8.1 to 8.4 in a window of its own before it. node006 goes second and only
  after the D28 backup is live and the D52 list is done, because its one
  boot disk holds every VM disk on it and nothing else has a copy. The D28
  `config.db` restore across that version jump is tried on the nested
  rehearsal host first.
- Visibility: a node-exporter textfile metric for pending security updates and
  `reboot-required`; alert when security updates are pending more than 7 days
  or a reboot is pending more than 14 days (35 on ocean). Falling behind becomes an alert,
  not a discovery.
- Container images: Renovate (see platform table) covers CVEs inside images.

**OS majors: not in this plan (D11).** Every VM stays on the Debian major it
runs today. How the fleet moves to a new major, and how ocean and harbor get
replaced when one of them holds a GPU, an HBA, a pool and node-local state, is
undefined on purpose: it gets planned once k3s and Salt are in and upgrades
have been proven in day-to-day operation, because that plan should be written
from what running the cluster taught, not guessed at now. Two things in this
plan keep its options open: pods pin to role labels, never to a hostname
(D14), and the VM base image is a pin in `terraform-root` (D30).

The clock that later plan runs against: ocean is on Debian 12, which left
regular security support in June 2026 and is on LTS until June 2028. This
plan ends in 2028 H1 and the later plan starts after it, so the two do not
fit in front of that date. terrac accepts running past it (D53): from the end
of LTS until the later plan lands, Debian sends those VMs no security
patches, the pending-update metric reads zero because nothing is offered,
and G6 does not hold for them. That is the price of keeping an OS major out
of this work, and the first thing the later plan has to close.

### Platform components

| Concern | Today | Target |
|---|---|---|
| Deploy | `main-apply` runs a playbook | Argo CD app-of-apps from `homelab-kube` `master`. Auto-sync; `prune: true`, `allowEmpty: false`. |
| Home-grown images | `repository_dispatch` to `homelab`, playbook pulls `:latest` | See "Keeping images current" below: the repo's CI pushes an immutable tag and dispatches to `homelab-kube`, which commits the tag into the chart values. |
| Third-party images | `docker-refresh@<project>` timers pull floating tags weekly | See "Keeping images current": Renovate watches registries and opens PRs that bump the pinned tag. |
| Secrets | ansible-vault | SOPS with age, decrypted in-cluster by sops-secrets-operator (already in `homelab-kube`, switched from GPG to age). The age private key is the one bootstrap secret; it stays in the vault and the bootstrap play installs it. |
| Public cert | certbot on ocean, Cloudflare DNS-01 | cert-manager `ClusterIssuer` for Let's Encrypt, Cloudflare DNS-01, same token. |
| Internal TLS | `homelab-ca` (spec 2026-10-14, not yet implemented) | That spec's root never expires, has no revocation, and is `pathlen:0`, so it cannot sign an intermediate for the cluster; the only way cert-manager could issue from it is to hold the root key itself in a cluster Secret. Its name constraints also exclude pod and service IPs. So the CA design moves to a `pathlen:1` root before it is built (D39), and cert-manager issues from a replaceable cluster intermediate; the root key never enters the cluster. Building the CA and signing the intermediates (one for production, one for the rehearsal cluster) is terrac's, by hand, with the root key offline; it is a roadmap box, not a gate. Until it is done the cluster has no internal issuer: public names use the Let's Encrypt cert at the Gateway and `.home` names are served the way nginx serves them today. |
| Metrics | Prometheus on ocean, file-based scrape config | Phases 1 to 4: an Alloy (or Prometheus agent) in the cluster discovers ServiceMonitors and remote-writes to ocean's Prometheus, because pod IPs are not reachable from outside the cluster and a migrated service must not lose its metrics. Phase 5: kube-prometheus-stack in cluster, TSDB at the path it uses today on ocean, the agent retires. |
| Logs | Loki plus promtail | Alloy in the cluster pushes pod logs to ocean's Loki from phase 1. Phase 5: Loki moves in cluster. Promtail is end of life and is not carried over. |
| Dead-man alert | none outside ocean | "Cluster is down" goes to email and ntfy (terrac, D17). Both of those run on ocean today (exim smarthost, ntfy container), so neither can be allowed to depend on the thing being reported dead. See "Dead-man alert" below. |
| DB backups | `files/db-backup/db-backup.sh` | Per-database CronJobs writing the same GCS object names, so `db-restore.sh` keeps working. Both remain authoritative until the last database moves. Postgres additionally archives WAL through CloudNativePG (D35). |

### App charts

The 2021 app charts are upgraded, not replaced, and they set the style for
every new app chart (terrac, D24).

The style, as it exists in `homelab-kube` today:

- One directory per app, `charts/<app>/`, holding a thin wrapper chart: a
  `Chart.yaml` at `version: 1.0.0` with exactly one dependency, and a
  `values.yaml` whose top-level key is that dependency's name.
- All app config lives in that one values file, in the common-library layout:
  `image`, `env`, `service`, `ingress`, `probes`, `persistence`, with the
  `# --` doc comments kept.
- Local templates only for what the dependency cannot express.
- One entry per app in `charts/argocd-applications/values.yaml`.

What "upgrade" has to mean, because the upstream is gone: the dependency
changes from the dead `k8s-at-home/<app>` chart to `bjw-s-labs/app-template`
(v5.3.0 on 2026-11-10), the maintained descendant of the same common library.
The wrapper layout and the one-values-file habit carry over unchanged. What
does change inside the values:

- The schema: app-template nests the same settings under `controllers` and
  `containers`. Each chart's values are rewritten once to the new schema.
- Images: from frozen `ghcr.io/k8s-at-home/*` tags to maintained images,
  pinned by tag and digest (R15). Where the Docker service on ocean already
  runs a given image, the chart uses that same image, so the cutover does not
  change the app version and the container at the same moment.
- `ingress` with `ingressClassName: nginx` becomes a `route` (HTTPRoute) on
  the Cilium Gateway.
- Persistence follows the storage rules above (local PV state on ocean,
  the whole-pool `/data01` mount), not the 2021 Ceph and NFS claims.
- Each chart gains what the PRD requires of every service: NetworkPolicy,
  probes, ServiceMonitor where there are metrics.

New apps with no 2021 chart (the *arr exporters, homepage, archmap and the
rest) get a wrapper chart in the same style on the same dependency. Platform
components (Cilium, Argo CD, cert-manager, kube-prometheus-stack) keep using
their own upstream charts, wrapped the same way.

### Keeping images current

Two paths, split by who builds the image (terrac, D18).

**Third-party images: a bot opens PRs.** Renovate runs on a schedule as a
GitHub Actions workflow in `homelab-kube`, on the self-hosted runner (not the
hosted Renovate app, so no outside service holds a token for the repo). It
reads the image tags in the chart values, asks the registries for newer ones,
and opens a PR per update that changes the tag (and digest).

- Every third-party image is pinned to a version tag plus digest. Floating
  tags (`latest`, `nightly`) get a digest pin, so even those change only by PR.
- A merge is a deploy (Argo CD syncs it), so the merge policy is the control:
  - patch and minor updates for stateless apps auto-merge once CI passes
    (`helm template` plus schema validation), batched into the Sunday 08:00 to
    14:00 Pacific window so the weekly rhythm stays what it is today;
  - majors, anything with a database or WAL state, and every tier 1 platform
    chart (Cilium, Argo CD, cert-manager) wait for terrac.
- Helm chart versions are updated by the same bot under the same rules.
- Renovate ignores `ghcr.io/bluefishforsale/*`.

**Home-grown images: push, then dispatch.** The repo's CI builds the image,
pushes it to `ghcr.io/bluefishforsale/<name>` under an immutable tag (the
commit sha), then sends a `repository_dispatch` to `homelab-kube` with the
image name and tag. A workflow there writes the tag into that app's values file
and commits it; Argo CD syncs. Same trigger as today, different target: the
dispatch updates a chart, it no longer runs a playbook.

- `:latest` is never what the cluster runs; the sha tag is, so a rollback is a
  revert of one commit.
- The dispatch workflow may only edit the values file of the app named in the
  payload, and only its image tag.
- Shape B static sites (built assets cloned onto a host and served by nginx)
  have no image today. Design call: each becomes a small nginx image built by
  its repo's CI and takes this same path.
- `docs/operations/deploy-pattern.md` gets the new pattern before the first W3
  cutover.

### Dead-man alert

Requirement (D17): when the cluster is down, terrac gets an email and an ntfy
push. Today Alertmanager, the exim smarthost and ntfy all run on ocean, and
ntfy and Alertmanager are scheduled to move into the cluster. A notification
path that runs on the thing that died sends nothing.

- A watchdog on agentbox (tier 0, already runs the Alertmanager receiver)
  probes the k3s API `/readyz`, ocean, and Alertmanager's own Watchdog
  heartbeat. It alerts only once a target has been down longer than an ocean
  reboot takes (ten minutes to start with), so the supervised monthly window
  does not page.
- It sends email through a relay that does not pass through ocean or the
  cluster (agentbox's own smarthost config to the same upstream relay).
- ntfy: design call, **ntfy moves to tier 0 (agentbox) and never into the
  cluster**. It comes out of wave W1. Otherwise the push half of D17 cannot
  work when it is needed. Moving the container is half of it. A phone reaches
  ntfy by name, and if that name still lands on ocean's nginx or ocean's
  `cloudflared`, the push dies with ocean anyway. So ntfy's name points at
  agentbox: a DNS record change through the pair gates for the LAN name, and
  for a public name a tunnel that terminates on agentbox, in
  `terraform-root`. This is the one exception to "no service DNS changes"
  (D10).
- It is built in phase 1, before the soak: the soak's gate is "no unplanned
  API outage" and something has to notice one. It is tested again (stop the
  API, get both notifications) before Alertmanager leaves Docker in phase 5.
  ntfy moves to agentbox at the same time; neither depends on the cluster.
- agentbox is on node005 and ocean, with the server on it, is on node006, so
  the watcher and the watched do not share a hypervisor.
- The other direction needs its own path. Today Alertmanager on ocean emails
  through ocean's exim (`files/ocean-prometheus/alertmanager.yml.j2:4`), so a
  dead node005 is reported. With ntfy and the only relay both on agentbox
  that would be lost: Alertmanager would watch node005 die and have no way to
  say so. So the exim smarthost stays on the ocean host and stays
  Alertmanager's email route, agentbox gets its own relay config for the
  watchdog, and each hypervisor reports the other's death. Both dying at once
  is still silent, and stays out of scope.
- agentbox is slimmed, not retired (D19). What stays on the VM is what has to
  work with the cluster down: this watchdog, ntfy, the remote-control
  sessions, and the break-glass apply role (coordination rule 6). What leaves
  for the cluster: the alert receiver (`agentbox-alert-receiver`) moves with
  Alertmanager in W6, and the media timers move as CronJobs with the service
  each one drives (`agentbox-media-refetch` and `agentbox-replace-reconcile`
  with the *arrs in W2, `agentbox-tdarr-drive` and `agentbox-x265-watch` with
  tdarr in W4). With the receiver in the cluster, alert pushes depend on the
  cluster, which is exactly the case the watchdog covers.

### Salt

- **Masterless (D7)**: no master, no `salt-minion` daemon. Every host runs
  `salt-call --local` against its own checkout of the Salt repo, with
  `file_client: local` and `file_roots` and `pillar_roots` pointing at that
  checkout. There is no tier 0 Salt VM, so nothing central has to be up for a
  host to be repaired, and the chicken-and-egg question does not arise. The
  minion `id` is set explicitly to the inventory name, never derived from the
  hostname.
- **Hosts**: every Linux host in the inventory, Proxmox hosts included. Not
  the Mac.
- **Repo**: new repo, `homelab-salt` (name is the builder's call).
- **Apply model**: no scheduled highstate. Changes apply through CI, per changed
  state, from the self-hosted runner, mirroring today's impact detection and
  serialization: for each impacted host the runner rsyncs the repo tree at the
  merged commit (an exact SHA, not a branch tip, so what applies is what was
  reviewed) to the host over SSH, connecting by IP from pillar rather than by
  name, and runs `salt-call --local state.apply <state>`, one host at a time
  under the per-host deploy lock ("One lock per host"). The host never fetches from GitHub, so it needs no
  deploy key and no working DNS to be repaired, which is what lets CI fix a
  DNS server whose resolver is broken. The SSH path is the one Ansible uses
  today; no new port or daemon. On every merge the runner also rsyncs the
  tree to the hosts it does not apply to, so a quiet host's checkout is
  current and its drift timer checks the reviewed tree.
- **Runners**: today there is one runner VM, `gh-runner-01` on node005
  (`inventories/production/hosts.ini:17`), and every Salt apply, k3s dispatch,
  DNS gate, Renovate run and image dispatch goes through it. The GitHub
  account is a user, not an organization, so a runner registers per
  repository: the VM already carries separate registrations (the
  `terraform-lan` ones for `terraform-root`), and `homelab-kube` and
  `homelab-salt` each need their own on both runner VMs. A second runner
  on node006 (D36) means losing node005 does not also lose the tool that
  repairs it. A runner never applies its own states or reboots itself from
  its own job (coordination rule 6): the other runner does it, or agentbox
  when only one is up.
- **Drift (coordination rule 8)**: a systemd timer on each host runs
  `state.apply test=True` against its checked-out commit and writes a
  node-exporter textfile metric (states that would change, failures, last run
  time, the checkout's SHA). Alerts: any would-change state for more than a
  day, a failed run, or a checkout that has differed from `master` for more
  than 7 days. A host cannot see `master`, so it only exports its own SHA;
  the runner exports `master`'s and the comparison is a Prometheus rule. No
  collector beyond Prometheus is needed.
- **No master features**: no mine, no reactor, no event bus, no
  `salt '*' cmd.run`. Cross-host data (the k3s server URL and join token, the
  NFS server address) lives in pillar. Cross-host ordering (k3s server before
  agents) is the CI workflow's order, not Salt orchestrate. Ad hoc fleet
  commands stay with `scripts/fleet-systemctl.sh` and friends.
- **Bootstrap (D29)**: one script, `bootstrap/bootstrap.sh` in `homelab-salt`,
  for every host. It installs the Salt onedir `.deb` pinned by version and
  sha256 and holds it, writes `/etc/salt/minion.d/masterless.conf` with the id
  it is given, generates the host's age key and prints the public half. terrac
  commits that public key and runs `sops updatekeys`; CI's next rsync brings
  the tree. Done by hand that puts terrac inside every VM rebuild, which is
  at odds with cattle VMs (D26) and with a new VM being one `terraform apply`
  away (D30). So a workflow takes the host's name as its only input, reads the
  public key from that host itself over SSH by IP, updates `.sops.yaml`, runs
  `sops updatekeys` with the CI key and opens the PR; terrac's part is the
  merge. It never takes a key as an input: a re-encrypted diff cannot be
  checked by eye, so a pasted key would hand that host's pillar to whoever
  dispatched the run. Bare metal (the Proxmox hosts) runs it by hand after install, since
  there is no cloud-init there; VMs run the same script from cloud-init
  user-data that `terraform-root` attaches. During the migration one Ansible
  play calls the script on existing hosts, so there is a single code path from
  day one. This is phase 0 work (D40). Salt's upstream package repo has moved
  before, so the script never fetches from it: the pinned `.deb` is mirrored
  to GCS and verified by hash.
  CI runs the script in a clean Debian container on every change to it.
- **Ownership**: a host concern is owned by exactly one of Ansible or Salt at a
  time. The PR that turns on a Salt state deletes the Ansible role or play that
  owned the same concern.
- **Hard limits carry over**: sshd, `authorized_keys`, the fleet key and
  port 22 firewall rules may be ported to Salt only by terrac, by hand. DNS
  and DHCP are not in this list; they are CI-applied with their own
  gates (D25, next section).
- **Secrets**: host secrets move from ansible-vault to Salt pillar encrypted
  with the same sops/age scheme. With a master, the master decides which host
  sees which pillar; masterless, every host holds the whole repo, so the
  encryption does that job instead. Pillar is split per host
  (`pillar/hosts/<id>.sls`) and per shared need (`pillar/shared/<name>.sls`,
  for example the k3s join token), and `.sops.yaml` creation rules encrypt each
  file to terrac's key, the CI key, and only the host keys that need it. Each
  host's age key is generated on the host at bootstrap (`/etc/salt/age.key`,
  root, `0400`) and only its public half is committed. A host therefore
  decrypts its own pillar and nothing else, and a lost host key means a rekey,
  not a fleet compromise. How pillar gets decrypted at render time (a sops
  renderer, an `ext_pillar`, or a wrapper that decrypts to tmpfs before
  `salt-call`) is the builder's call, settled in phase 0: the first
  secret-bearing state, the D28 timer's GCS key, needs it. Keys are per host,
  not pillar pushed by CI (D42).
  The vault cannot be archived until every key has a new home, including the
  cluster's age key and the Cloudflare Access email lists that `terraform-root`
  still reads from it.
- **Order**: base system first (packages, users, node-exporter, docker on the
  remaining Docker hosts), then the k3s install and upgrade (after phase 1's
  drill, so the playbook that survived three upgrades is the reference), then ocean's
  own host states (NFS server, zed, NVIDIA, the metrics timers), then Proxmox
  hosts, DNS and DHCP last, because the gates below are worth more once every
  other host has exercised the apply path. One condition on that order: the
  Salt k3s state does one install and one upgrade on the rehearsal VM
  before it touches the real cluster; the play it replaces earned its place
  with three.
- **Write once** (D40): the repo, the bootstrap and the first base
  states are phase 0 and 1 work, and the D28 timer, the patch pipeline (1b)
  and the watchdog are written in Salt from the start. Written in Ansible
  first, each would be built twice inside two quarters. Any new VM (the
  rehearsal VM, the second runner) is born under Salt. D3 keeps the k3s play in Ansible until
  it has passed the drill. One consequence: the D28 timer makes the two
  hypervisors the first hosts under Salt, bootstrapped by hand, so phase 0's
  Salt work is the repo, the bootstrap, pillar decryption and the deploy
  lock, not one state on one VM.

### DNS and DHCP under Salt

Salt owns DNS and DHCP end to end (D25), applied
by CI like everything else, not by hand. Today's Ansible play already does
this from CI, guarded by `serial: 1`, `any_errors_fatal` and a post-host
`dig` gate (`playbooks/individual/core/services/dns_ha_stack.yaml`); the Salt
version keeps those guards and adds more, since a resolver pair that is down
takes the runner that would fix it down too.

**Scope.** Everything `roles/dns_infrastructure` and its neighbours own today:

- PowerDNS authoritative (primary on dns02, AXFR secondary on dns01), the
  recursor, dnsdist, and the zone data and its import.
- Kea DHCP4 in hot-standby (primary dns02, standby dns01), Kea DHCP-DDNS, the
  control agent, every reservation and option, and the kea exporter.
- netboot.xyz and TFTP on the pair, and promtail on both.
- The fleet resolver config on every host (`fleet_resolvers.yaml` today: the
  netplan drop-in on cloud-init VMs, `/etc/resolv.conf` on the Proxmox hosts).
- pihole (192.168.1.9).

**One address table.** Host, IP, MAC, DHCP reservation and A/PTR record come
from a single pillar data file. Kea reservations and PowerDNS records both
render from it, so a host cannot get a reservation without a matching record
or the reverse. VM MACs are set in `terraform-root`; a CI check compares the
table to the MACs in `lan/proxmox-node00{5,6}-vms.tf` and fails on a mismatch.

**Who may change it.** Agents may author DNS and DHCP PRs. Only terrac merges
them. (The one exception is a service record written by ExternalDNS, fenced
as "Names" in Networking describes, D55.) A required review cannot enforce that here: agents open PRs from
terrac's own GitHub account, and an account cannot approve its own PR. So the
control sits in two other places. No auto-merge rule covers the DNS, DHCP and
resolver states, the address table, or the dns01, dns02 and pihole pillar,
so a PR on those paths waits for terrac to merge it by hand. And the apply
workflow for those paths runs in a GitHub environment that waits for
terrac's approval before any host is touched (the same mechanism D33 uses).
Environment approvals are free on a public repository; whether
`homelab-salt` is one is an open item in the PRD. Agents never run these states outside CI. terrac's
break-glass is `salt-call --local` on one host of the pair from the tree that
is already there, which works with GitHub, CI and DNS all down.

**Apply sequence (CI).** Each step gates the next; any failure halts the run,
leaves the untouched peer alone, and alerts through agentbox (ntfy and email).
No automatic rollback, per the standing homelab default; the fix is a forward
PR or terrac's break-glass.

1. *Offline validation on the runner*, before any host is touched: render
   every changed config and run each daemon's own config check against it in a
   container pinned to the hosts' versions (`kea-dhcp4 -t`, `kea-dhcp-ddns -t`,
   a PowerDNS zone check, dnsdist's config check; exact flags confirmed when
   porting).
2. *Pair pre-flight*: both hosts answer DNS, Kea HA reports a healthy
   hot-standby pair, and the secondary's SOA serials match the primary's. A
   pair that is already degraded is not applied to, except through a dispatch
   input only terrac uses.
3. *Standby first* (dns01). Every managed config file uses Salt's
   `file.managed` `check_cmd`, so a file that fails its daemon's check never
   replaces the working one on disk. Only the daemon whose config changed is
   restarted (`docker compose up -d <svc>` while the stack stays in compose),
   never the whole stack, which is how today's play restarts it.
4. *Host gate* on dns01, querying that host's IP directly so the peer cannot
   mask a failure: an internal A record, a PTR, an external name through the
   recursor, and Kea's status. Same 3-minute budget the Ansible gate grew after
   the 2026-09-09 slow restart.
5. *Primary* (dns02), then its host gate. Kea fails over to dns01 while the
   primary restarts; that is the hot-standby pair doing its job.
6. *Pair gate*: both answering, Kea HA back to normal hot-standby, SOA serials
   equal on both.
7. *Resolver config* on fleet hosts: one canary host first, then one host at a
   time with a `dig` gate each, and the runner last so a bad resolver change
   cannot cut off the run applying it.

A zone-only change applies to the primary alone and gates on the secondary's
serial catching up through NOTIFY and AXFR.

**Cutover from Ansible.** For at least two weeks before ownership moves, the
Salt DNS and DHCP states run on the pair in `test=True` only, and CI diffs
Salt's rendered configs against the files Ansible deployed. The flip happens
only once they are byte-identical, in one PR that deletes
`roles/dns_infrastructure`, `dns_ha_stack.yaml` and `fleet_resolvers.yaml`.
The first real Salt apply on the pair therefore changes nothing on disk.

**Rehearsal.** Before that flip, a throwaway pair (`dns-test01`, `dns-test02`)
plus a client VM, defined in `terraform-root` on a Proxmox bridge with no
uplink, so a test Kea can never hand out a lease on the LAN. Drills, all
through the real CI path: a full apply; a deliberately bad Kea config (CI
halts at step 1); a config that passes the check but breaks resolution (CI
halts at step 4, the primary untouched, the client still resolving and
renewing its lease); a killed primary (the standby serves leases); and a zone
change reaching the secondary. Results are recorded, then the test VMs are
deleted.

## Disaster recovery: rebuild from git

The test case is both hypervisor boot disks dying at once. The probe below
(read-only, 2026-11-14) is what decides what survives.

| | node005 | node006 |
|---|---|---|
| Boot disk | SATADOM-ML 64 GB (`sdf`): `pve-root`, an empty `local-lvm` | Samsung 870 EVO 2TB (`sda`): `pve-root` **and** `local-lvm` |
| VM disks | `vmpool-storage`, a ZFS mirror on two 860 EVO 1TB, separate from boot | `local-lvm` on the boot disk: ocean root (129G), dns02, llmeval (300G), template 9999 |
| Boot disk dies | Proxmox and `/etc/pve` lost; every VM disk survives | Proxmox, `/etc/pve`, every VM disk and the template lost; only `/data01` (HBA passthrough) survives |

The two nodes are not a Proxmox cluster (VMID 5000 exists on both), so
neither holds the other's VM configs. No vzdump or Proxmox Backup Server job
exists.

### What git cannot rebuild, and where each lives

| Thing | Home | Protection |
|---|---|---|
| terrac's age key | terrac's machines | Plus an offline **recovery** age key (D32), printed and on a USB stick in two places. Every sops file in every repo is also encrypted to it. A quarterly decrypt test proves it still works. |
| Terraform state | GCS, versioned | Already so. |
| DB dumps, k3s SQLite backup, Proxmox config DBs | `gs://homelab-db-backups`, versioned | Each has a freshness metric and alert. |
| Postgres WAL and base backups | `gs://homelab-pg-wal`, unversioned, pruned by CloudNativePG's retention policy | Archive-age and base-backup-age alerts, monthly scratch recovery (D35). |
| `/data01` | The HBA disks | RAIDZ2; imported by hand only. Not backed up off-site by this plan (see non-goals). |
| Images | GHCR | Rebuildable from each app repo's CI. |
| Salt `.deb`, Debian cloud image | GCS mirror, pinned by hash | Upstream URLs are not trusted to stay put. |

### Decisions that close the gaps

- **D26: VM root disks are cattle.** No VM-disk backups. Every VM is
  rebuildable from git, GCS and `/data01`, and the rebuild drill proves it.
  Backout: a VM that fails the drill gets a weekly vzdump until it passes.
- **node006's boot disk is an accepted single point.** It is the worst
  exposure in the table above: one SSD holds Proxmox and every VM disk on
  that node, and there is no free bay for a second, so nothing mirrors it and
  this plan adds no hardware. Its death costs a Proxmox reinstall plus a
  rebuild of ocean's root, dns02, llmeval and the second runner from git
  (steps 2 to 5 below). `/data01` and every service's state on it survive,
  since they are on the HBA. Two things follow. The k3s server's SQLite is on
  that disk and git cannot rebuild it, so it goes to GCS nightly (R2). And
  phase 0 lists whatever else on that disk is in none of git, GCS or
  `/data01`, and gives each a home, since it can no longer be assumed to
  outlive its host. The list covers every VM disk there, not only ocean's
  root: dns02 and llmeval's 300G are on it too. It is done before node006's
  PVE upgrade, which is the riskiest thing that disk goes through in this
  plan. Neither boot disk is mirrored; D28 is what makes a
  reinstall quick.
- **D28: the Proxmox config DB is backed up.** A nightly timer on each
  hypervisor takes `sqlite3 .backup` of `/var/lib/pve-cluster/config.db`,
  age-encrypts it to terrac's and the recovery key (it holds `/etc/pve/priv`),
  and uploads it to `gs://homelab-db-backups/pve/<node>/`. Freshness metric,
  alert past 48 hours. Restoring it onto a reinstall with the same hostname and
  IP brings back every VM config and `storage.cfg`, so VMs whose disks
  survived start as they were and `terraform-root`'s next plan is a no-op.
  This is the reattach path; letting Terraform recreate a VM over existing
  zvols is not one (its behaviour there is unverified, and the VMs carry
  `purge_on_destroy` and `delete_unreferenced_disks_on_destroy`). The timer
  is a Salt state from the start (D40).
- **D29: one bootstrap script** (in "Salt" above).
- **D30: the base image is stock.** No custom template build. Each node's base
  is the Debian genericcloud image, pinned by URL and sha512, mirrored to GCS,
  and downloaded onto the node by `terraform-root`. Everything else comes from
  cloud-init calling the bootstrap script, then Salt. A dead node's template
  is therefore one `terraform apply` away. The pin is the Debian major the
  fleet runs today; moving it is the later distro-lifecycle plan's business.
  (Provider resource names are confirmed when it is built.)
- **D31: stateful pods restore by an explicit Job.** Every app chart with a
  node-local database carries a backup CronJob (writing the same GCS object
  names `db-backup.sh` writes today) and a restore Job template that is never
  synced automatically: run on demand, dry-run by default, refusing a
  non-empty target unless told otherwise, the same contract as today's
  `db_restore.yaml`. The nightly restore test runs it against a scratch PVC.
  App state on ocean needs none of this for a host rebuild: it is a local PV
  on `/data01/services/<svc>` (Storage, above) and survives.
- **D35: Postgres runs under CloudNativePG with WAL archiving.** Today there
  are four Postgres containers. Three are in the nightly dump
  (`jellystat-db`, `mem0-postgres`, `archmap-postgres`,
  `files/db-backup/db-backup.sh:61`, which runs on whichever host holds the
  container), so a bad day loses up to 24 hours: jellystat's is on ocean,
  and mem0's and archmap's moved to harbor with their services. The fourth is
  ml-log-analyzer's, also on harbor (`files/ml-log-analyzer/docker-compose.yml.j2:55`),
  which is in no dump at all; it is added to `db-backup.sh` and
  `db-restore.sh` in phase 0 and is the first database to move, in phase 2,
  so the operator and the import are learned on the least critical one.
  archmap's and mem0's follow in the same phase, mem0 last. mem0 is why that matters: it is
  the agents' shared memory and changes all day. Each database becomes a
  CloudNativePG `Cluster` with `instances: 1`. No replicas, because each app
  runs one pod and its state is pinned to one node anyway, so a standby buys
  failover for a database whose only client is already down. A single-instance
  cluster blocks a node drain by default, because the operator's disruption
  budget protects the only instance; that would hang kured on harbor
  and ocean's monthly drain. So each cluster sets `enablePDB: false` and
  takes the outage a drain implies. The operator is
  a tier 1 platform app synced by Argo. WAL ships continuously to
  `gs://homelab-pg-wal` through the Barman Cloud plugin with
  `archive_timeout` at five minutes, plus a weekly base backup and a 14-day
  retention policy, which puts RPO at about five minutes with point-in-time
  recovery inside the window. The bucket and a service account that can only
  write to it live in `terraform-root`; the key reaches the cluster like any
  other cluster secret. The D31 nightly logical dump stays beside it, because
  a dump restores across Postgres majors and does not need the operator to be
  healthy. Restore is a new `Cluster` bootstrapped with `recovery` from the
  object store, optionally to a target time, run by hand like D31; a monthly
  job recovers mem0 into a scratch cluster and compares row counts. PGDATA
  stays node-local (a static local PV on ocean; on harbor `local-path`, or
  for mem0 a static local PV on the 870 EVO where its data is today). Cutover is not zero-copy, unlike the rest of ocean's state:
  CloudNativePG cannot adopt the Docker data directory, so each database moves
  in its window through `bootstrap.initdb.import` from the running container,
  which also allows a major-version bump on the way. The import needs the
  old database container running and its app stopped, so a Postgres cutover
  is its own sequence: stop the app containers, import, stop the database
  container, start the app pod. mem0 runs `ankane/pgvector`
  (`files/ocean-mem0/docker-compose.yml.j2:83`), so its `Cluster` needs an
  operand image that ships pgvector at the same or a newer version. If the
  uplink is down, archiving stalls and segments pile up in `pg_wal` (288 a
  day per database at the five-minute timeout, 16 MB each before
  compression), so R22 also alerts on `pg_wal` size. Plugin versus the
  in-tree `barmanObjectStore`, and the exact metric names behind the R22
  alerts, are confirmed when it is built.
- **D32: a recovery age key** (table above).
- **D33: the DNS pair gate has a rebuild mode.** The pair pre-flight refuses a
  degraded pair, which as written would block rebuilding a dead peer. A
  dispatch input `rebuild_peer=<dns01|dns02>`, behind a GitHub environment
  that only terrac can approve, applies to that host only. It requires the
  other peer to pass its host gate first and never touches it. Kea syncs
  leases from the survivor on start; the rebuilt PowerDNS renders zones from
  git, and its SOA serial must be at least the survivor's. Then the normal
  pair gate runs.
- **D34: Salt owns the hypervisor, passthrough included.** States for the
  Proxmox hosts cover apt repos, IOMMU kernel arguments (handled for
  whichever of GRUB or `proxmox-boot-tool` the host boots with),
  vfio-pci binding of the GPU and HBA with a softdep that runs before
  `mpt3sas` and `nvidia` can claim them, `storage.cfg` entries through
  `pvesm`, the D28 timer, NUT, and the exporters. On node006 the state also
  disables `zfs-import-scan` and `zfs-import-cache` for anything but the
  host's own pools: a reinstalled host that sees the HBA must never import
  `/data01` while ocean may. The bridges in `/etc/network/interfaces` are
  rendered by Salt, but like sshd a mistake there locks the host out, so CI
  only runs them `test=True`; terrac applies them by hand with the iDRAC
  console open.
- **Break-glass Terraform.** The `lan/` root normally applies from the
  self-hosted runners, which may be on the dead node. When they are down,
  terrac applies it from the laptop on the LAN against the same GCS state and
  lock. This is the one sanctioned local apply, and the next CI plan must be
  clean.

### Recovery order: both boot disks dead

1. **node005** (its VM disks survive). Replace the boot media; install
   Proxmox by hand with the same hostname and IP; run the bootstrap script;
   restore `config.db` from GCS (D28); `zpool import vmpool-storage`. DHCP is
   down as well as DNS at this point, so the laptop and the new host each
   get a static address and an outside resolver by hand, or the bootstrap
   cannot reach GCS. Start dns01
   first: DNS and DHCP return on one server (Kea standby in partner-down,
   PowerDNS serving its secondary copy). Then gh-runner-01, which carries
   every repo's runner registration, and CI is back. Then registry-cache-01, agentbox
   (ntfy, dead-man), pihole, netboot01, harbor. The household is
   without DNS only for this step, which is why it comes first.
2. **node006 host.** Install Proxmox; bootstrap; restore `config.db` (its VM
   configs now point at disks that no longer exist, so they are a reference
   for what to recreate, not a restore).
   The import guard (R21) is a Salt state, and Salt's tree only arrives once
   CI is back, so the first boot cannot rely on it: the disk shelf stays
   disconnected, or `mpt3sas` is blacklisted on the kernel command line,
   until that state has applied. Before anything touches the HBA, `zpool import` with no arguments must show
   `/data01` as last used by another system; the host never imports it.
   terrac applies the D34 network state by hand; CI applies the rest.
3. **node006 VMs** through CI: `terraform-root` downloads the base image
   (D30) and recreates whatever is missing.
4. **dns02** through the D33 rebuild mode, then the normal pair gate.
5. **ocean**: terrac imports `/data01` inside the VM; Salt brings NFS,
   NVIDIA and zed, then installs k3s; the nightly SQLite backup is restored
   (the phase 1 rehearsal); the `/data01` mount guard lets k3s start;
   harbor's agent reconnects; Argo reschedules the pods onto their local PV
   state.
6. **Everything else**: llmeval and any other VM from Terraform plus Salt;
   pods with node-local databases on lost disks restore through D31;
   Postgres recovers from its WAL archive to the last five minutes (D35).
7. **Verify**: drift metrics zero on every host, Argo all Synced and Healthy,
   the dead-man fires on a stopped API, `terraform-root` plans clean.

### Rehearsal

A rebuild that is never run is a guess, so it is a phase gate (phase 7) and a
yearly drill after that, on the isolated bridge the DNS test pair uses:
Proxmox nested in a VM, a small throwaway ZFS mirror standing in for
`vmpool-storage`, a test DNS pair and a test runner. The drill wipes the
nested host's boot disk and runs this section's order from git and GCS only,
timing each step.

### Single-site DNS

Both DNS servers and pihole are VMs on the two hypervisors, so losing both
hosts takes name resolution away from every device in the house until step 1
finishes. There is no hardware off the hypervisors to host a third resolver
(D43), so this risk is accepted and the recovery order is the mitigation.

## What stays in Ansible until Salt replaces it

The k3s install and upgrade play, `data01` export, ZFS read-only checks, DNS,
runners, agentbox. Proxmox VM definitions (CPU, RAM, disks, NICs, passthrough
devices) are not Ansible's or Salt's: they are `terraform-root`'s, and stay
there. Once Salt owns all of it
and the last service has left ocean's Docker, `homelab` is archived.

## Assumptions to verify

Claims this design leans on, and where each gets checked. A row moves to
"confirmed" with a date and a source, or the design changes.

| Claim | Status | Checked in |
|---|---|---|
| `local-path`'s stock StorageClass reclaims with `Delete` | Confirmed from docs, 2026-11-15 | Re-read on the cluster, phase 1 |
| A minor kubelet upgrade needs a drain first | Confirmed, upstream version skew policy, 2026-11-15 | |
| A single-instance CloudNativePG cluster blocks a node drain unless `enablePDB: false` | Confirmed from operator docs, 2026-11-15 | Drain drill on harbor, phase 2 |
| Cilium's L2 announcements need kube-proxy replacement, and are stable enough to run at the pinned Cilium version | From memory, unverified | Cilium docs, then the rehearsal cluster, phase 1 |
| A Gateway on a load-balancer address serves 80/443 without binding a host port or needing `NET_BIND_SERVICE` | From memory, unverified | The rehearsal cluster, phase 1 |
| `192.168.1.64/27` is unused | Sweep and repo read, 2026-11-19; a powered-off device would not show, and the UniFi client history was not read | UniFi client list, phase 0 |
| A record written through the PowerDNS API reaches both dns01 and dns02 | Unknown: this spec and `scripts/dns-drift-check.sh` disagree on how the pair replicates | Phase 0, before ExternalDNS |
| ExternalDNS's PowerDNS provider reads Gateway API routes and `DNSEndpoint` objects, and leaves records without its ownership TXT alone | From memory, unverified | ExternalDNS docs, then the rehearsal cluster, phase 1 |
| `app-template` 5.x is maintained (5.3.0 released October 2026) and its majors can force a Deployment to be recreated | Confirmed from the project's release page and a downstream chart's upgrade notes, 2026-11-19 | |
| `app-template` 5.x needs Kubernetes 1.31 and Helm 3.18 or newer | From a third-party listing, not the project's own pages | Reference chart render in CI, phase 0 |
| The Gateway sees a LAN client's address with `externalTrafficPolicy: Local` | From memory, unverified | The rehearsal cluster, phase 1 |
| A tunnel with one catch-all rule to the Gateway serves any hostname whose CNAME points at it, and a proxied CNAME moved between tunnels takes effect in seconds | From memory, unverified | The rehearsal cluster, phase 1 |
| Cilium creates an in-cluster Service for each Gateway that `cloudflared` can target by name | From memory, unverified | The rehearsal cluster, phase 1 |
| A record removed from Terraform state is left in place at Cloudflare, and ExternalDNS can then adopt it | From memory, unverified | One test record, phase 1 |
| k3s stores Secrets unencrypted in its database unless `--secrets-encryption` is set | From memory, unverified | k3s docs, phase 1; the nightly backup carries that database, so the flag goes on if so |
| Cilium's datapath and Docker's bridge and iptables rules coexist on one host through a reboot | Unverified | The rehearsal VM, phase 1, before k3s goes onto ocean |
| k3s's ServiceLB claims host ports 80 and 443 unless disabled, and k3s re-applies its bundled manifests on start | From memory, unverified | k3s docs and the rehearsal VM, phase 1 |
| A plain NetworkPolicy cannot allow traffic from Cilium's Gateway or from a hostNetwork pod | From memory, unverified | Cilium docs and the reference chart, phase 1 |
| Kubelet counts ZFS ARC as used memory | From memory, unverified | Upstream issues, then ocean's soak |
| `k3s-uninstall.sh` leaves Cilium's kernel state behind | From memory, unverified | Backout run on the rehearsal VM |
| A rebuilt node under its old name is refused without its node password file | From memory, unverified | Fresh-build restore on the rehearsal VM |
| k3s has no supported in-place downgrade | From memory, unverified | k3s docs, phase 1 |
| A pod's NFS volume on harbor shows ocean's `/data01/services`, not harbor's local disk | Reasoned from `data01-workers.md`, unverified | Phase 2 test pod |
| A GitHub account cannot approve its own PR, and environment approvals are available on a public repo under a user account | From memory, unverified | Phase 0, on `homelab-salt` |
| PVE 8 reached end of life in August 2026 | Confirmed, 2026-11-15 | |
| GitHub concurrency groups are scoped to one repository | Probable, no definitive source found | Phase 0, two repos, one group name |
| A local PV whose path is missing fails to mount instead of creating it | Unverified | The rehearsal VM, before k3s goes onto ocean |
| Argo CD polls git about every three minutes by default | Unverified | Phase 1 |
| Salt can decrypt sops pillar at render time, masterless | Unverified | Phase 0 |
| A PVE `config.db` restores across a major version | Unverified | Nested rehearsal host |
| Group read and write is enough to hardlink another uid's file | Unverified | Before the first R26 change |

## Left to the builder

- The rehearsal VM's name, IP (a free address in the .2-.50 infrastructure
  range, checked against inventory and DHCP reservations), vCPU and RAM.
- Chart versions, as long as each one's supported Kubernetes range is recorded
  in the upgrade log.
- Which three toy apps run the upgrade drill, as long as they cover ingress plus
  a cert, a local PV, and a sops secret.
- Where the nested Proxmox VM for the `config.db` restore test and the
  rebuild drill runs (node005's `vmpool-storage` is the only pool with room
  and a mirror under it).
- The cutover workflow's mechanics: how its merges are marked so
  `main-apply` skips them, and how its GitHub token is scoped.
- Salt repo layout.
