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
| node005 | Proxmox 8.1, 56 threads, 125 GiB RAM (92 used, RAM upgrade planned). VM disks on `vmpool-storage` (ZFS on SATA/NVMe SSDs). Runs dns01, pihole, netboot01, gh-runner-01, harbor, registry-cache-01, agentbox. |
| node006 | Proxmox 8.4, 40 threads, 660 GiB RAM (498 used, ocean holds 366 GiB). VM disks on `local-lvm` (870 EVO 2TB). Runs dns02, ocean, llmeval. |
| ocean | Debian 12, kernel 6.1, Docker 29.6, cgroup v2, ~60 containers. RTX 3090 (driver 610.43) and the SAS HBA passed through; owns the `data01` pool (87 TiB raw, 72% used) and exports it over NFSv4.2. nginx on 80/443 is the front door for every service. Docker sets `iptables -P FORWARD DROP`. |
| GPU today | Already shared: llama-server 19.6 GiB and tdarr-ffmpeg 376 MiB on the one card, via Docker. |
| "harbor" VM | Not Harbor. A `data01_workers` host: nzbget, a CPU tdarr-node, NFS `/data01` with a local state disk over `/data01/services` (`docs/architecture/data01-workers.md`). |
| registry-cache-01 | Pull-through caches for docker.io, ghcr, gcr, nvcr, gitlab. |
| `homelab-kube` repo | 2021-era kubeadm cluster config, 479 commits. Has an app-of-apps chart (`charts/argocd-applications`), sops-secrets-operator (GPG), cert-manager, MetalLB, Cilium, ingress-nginx, cloudflare tunnel ingress, kube-prometheus-stack, promtail, and charts for plex, tdarr, sonarr, radarr, lidarr, tautulli, overseerr, nzbget. |

Defects in `homelab-kube` to fix before reuse:

- `ocean-nfs-pvc.yaml` gives the 18T `/data01` PV `persistentVolumeReclaimPolicy: Delete`. Must be `Retain`.
- `charts/argocd-applications/values.yaml` sync policy has `prune: true` with `allowEmpty: true`: an empty render prunes everything. Must be `allowEmpty: false`.
- sops via GPG, promtail (end of life), chart dependencies four years stale.

A note on etcd: the original reason for avoiding it was spinning disks. VM disks
are on consumer SSDs, not spinning disks. Consumer SSDs without power-loss
protection still give poor fsync latency under ZFS sync writes, so the decision
stands; the reason is fsync latency, not rotation.

## Architecture

### Three tiers

| Tier | What | Runs on | Managed by |
|---|---|---|---|
| 0: break-glass | DNS pair, DHCP, Proxmox, ZFS and NFS on the ocean host, Salt master, registry-cache, k3s server, gh-runner, agentbox, the external dead-man alert | Plain VMs and hosts | Ansible now, Salt later |
| 1: platform | Argo CD, cert-manager, secrets operator, ingress controller, NVIDIA device plugin, NFS and local storage classes, kube-prometheus-stack, Alloy | The cluster | Argo CD |
| 2: apps | Every service that runs as a container today | The cluster | Argo CD |

Rule: anything you need to repair the cluster lives in tier 0. Nothing in tier 0
may depend on the cluster being up.

### One cluster, not a management cluster plus a workload cluster

Decided (design call, see PRD D2). A second, management cluster doubles the
upgrade and maintenance load for one operator on two hypervisors, and it does
not protect against the failure that matters (the cluster is down and you
cannot reach anything), which tier 0 covers. Upgrade risk is controlled with a
throwaway rehearsal cluster built from the same playbook, not a permanent second
cluster.

### Cluster shape

| Node | Host | Role | Size (initial) |
|---|---|---|---|
| `k3s-server01` | new VM on node006 | k3s server, tainted `NoSchedule` for workloads | 4 vCPU, 8 GiB, 40 GiB disk on `local-lvm` |
| `k3s-agent01` | new VM on node005 | agent; replaces the harbor VM's workloads | 16 vCPU, 32 GiB, OS disk plus a local state disk on `vmpool-storage` |
| `ocean` | existing VM on node006 | agent; GPU and `/data01` node, tainted so only workloads that tolerate it land there | unchanged |

This keeps "one k8s VM per metal" for workloads, plus a small server VM. The
server is not on ocean so that ocean reboots (driver updates, kernel) do not take
the API down. While the server is down, running pods keep running; only
scheduling and reconciliation stop.

- **Distribution**: k3s. **Datastore**: embedded SQLite via kine (single
  server, no `--cluster-init`, no etcd). HA control plane is an anti-goal for
  2027 (see PRD).
- **Version pinning**: one variable, `k3s_version`, in `vars/vars_k3s.yaml`.
  Install and upgrade plays are dispatch-only (added to the never-auto-apply
  list in `detect-impacted-playbooks.sh`). An upgrade is a PR that bumps the
  variable plus a deliberate dispatch, never a side effect of a merge.
- **Backup**: nightly `sqlite3 .backup` of `server/db/state.db`, plus the
  server token and `server/tls/`, to GCS beside the DB dumps. Restore is
  rehearsed in phase 1.
- **Images**: k3s `registries.yaml` mirrors docker.io, ghcr.io, gcr.io and
  nvcr.io through registry-cache-01.

### Networking

- **CNI**: open question for terrac (PRD Q1). Recommendation: Cilium, because
  "advanced network control" is one of the reasons for doing this, and swapping
  the CNI later means rebuilding the cluster. The fallback is k3s's default
  flannel plus its built-in NetworkPolicy controller. Decided before phase 1,
  because the upgrade drill must run on the CNI that ships.
- **No new IP ranges, no DNS changes.** Load-balancer IP pools would have to be
  carved out of DHCP, which lives with the DNS pair (an agent hard limit). So:
  - During migration, ocean's nginx stays the front door. Each service cutover
    repoints one nginx upstream from the Docker container to the k8s service
    (NodePort or ingress on a node IP).
  - At the end, the ingress controller takes ports 80/443 on ocean
    (`hostNetwork`), after the Docker nginx retires. Ocean's IP stays the front
    door. No DNS record changes.
- **hostNetwork pods** where the app needs the LAN: Home Assistant (discovery),
  Plex (GDM discovery, real client IPs for local-network detection).
- **Ports lockdown**: a default-deny NetworkPolicy per app namespace, with
  explicit allows. `vars/vars_service_ports.yaml` entries retire with their
  service.

### Storage

The `agents.md` rule "Nothing with a write-ahead log goes over NFS" carries over
unchanged. SQLite and Postgres state (the *arrs, Plex, Tautulli, Bazarr,
Grafana's MySQL, Prometheus TSDB, Loki) must be on a node-local disk.

| Kind | Where | How |
|---|---|---|
| Media (read/write, hardlinks) | `/data01` | One static NFS PV of the whole pool, `ReadWriteMany`, `Retain`, mounted on `k3s-agent01`. On ocean, a `hostPath` of `/data01` (no NFS loopback). One mount of the whole pool so *arr hardlinks do not hit `EXDEV`. |
| App state on ocean | `/data01/services/<svc>` | `hostPath` (or a static local PV) pinned to ocean, pointing at the same directory the Docker container uses today. Zero data copy at cutover. |
| App state on k3s-agent01 | its local state disk | `local-path` provisioner on the local disk. |

### GPU

- The 3090 is passed through to ocean, and a PCI device can only be passed to
  one VM. So no GPU workload can move until ocean is a cluster node. This is
  why ocean joins in phase 3, not at the end (PRD D4).
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

Ocean runs Docker and a k3s agent (its own containerd) together for the whole of
phase 4. Known risks: Docker's `FORWARD DROP` policy against pod traffic, two
container runtimes each needing the NVIDIA runtime configured, and host port
collisions. Mitigations:

1. Prove the exact coexistence on `k3s-agent01` first (install Docker there,
   run a container and pods, verify pod-to-pod, pod-to-service and NodePort
   traffic, then reboot and verify again).
2. Join ocean in a maintenance window, with a written backout (`k3s-agent-uninstall.sh`, restart docker).
3. Fallback if coexistence cannot be made clean: k3s's `--docker` mode on ocean
   only (one runtime, cri-dockerd), revisited once Docker is empty.

### Cutover of one service (the unit of migration)

1. Chart and values in `homelab-kube`, Application added to the app-of-apps but
   scaled to 0.
2. Secrets moved: added to sops in `homelab-kube`. Removed from the vault in the
   same PR that retires the Ansible play (never present in both once the play is
   gone; see PRD R7).
3. Window: stop the Docker container (`docker compose down` via the retire
   play), scale the Deployment to 1, repoint the nginx upstream, verify with the
   service's health check and blackbox probe.
4. Retire: the Ansible play, its `files/<svc>`, its port entry and its vault
   keys are removed in one `homelab` PR.

Never run the container and the pod at the same time against the same state
directory. Downtime per service is the stop-to-healthy window (target under 5
minutes), not zero.

### Ocean to k8s node: the full strategy

Ocean becomes a cluster node in two moves. The first is in place and additive
(no OS upgrade): a k3s agent joins next to Docker so services can move one at a
time. The second is the first **passthrough swap** (below): once Docker on ocean
is empty, a clean Debian 13 VM takes over the GPU, the HBA and the pool, and
from then on ocean is a k8s-only node rebuilt, never upgraded in place.

**Step 1: prepare (no downtime).**

- Prove Docker and k3s coexistence on `k3s-agent01` (see above).
- Inventory every ocean container into a migration table: class (stateless,
  WAL-bearing state, GPU, hostNetwork, platform), state path, ports, inbound
  dependants, external repo that deploys it. This table is the wave plan.
- Node labels and taint chosen up front, so pods pin to a role, not a host
  name: label `homelab.role/data01=local`, `homelab.role/gpu=rtx3090`; taint
  `homelab.role/ocean=true:NoSchedule`. Workloads that belong on ocean tolerate
  the taint and select on the labels. This is what lets pods follow ocean to a
  replacement VM in step 4 with no chart changes.

**Step 2: join (one maintenance window, about 30 minutes).**

1. Hold `docker-ce`, `containerd.io`, NVIDIA driver and ZFS packages
   (`apt-mark hold`) for the duration of the overlap.
2. Configure `nvidia-container-runtime` for k3s's containerd (Docker's config
   stays as is).
3. Install the k3s agent at the cluster's pinned version, with the labels and
   taint above.
4. Verify: every Docker service's blackbox probe is still green, Docker's
   `FORWARD DROP` has not broken pod traffic, then reboot ocean and verify both
   again.
5. Backout: `k3s-agent-uninstall.sh`, restart Docker, re-verify probes.

**Step 3: GPU gate.** Device plugin with time-slicing; run an NVENC transcode
pod and a CUDA pod while llama.cpp still runs in Docker. Record the NVENC
session cap. This is the go/no-go for the GPU half of the plan.

**Step 4: waves (phase 4).** One service per cutover, per the cutover procedure
above. Waves, lowest risk first:

| Wave | Services | Notes |
|---|---|---|
| W1 | exporters, homepage, ntfy, languagetool, karma, pushgateway, blackbox, mtr, ndt | stateless; learn the nginx flip |
| W2 | sonarr, radarr, lidarr, prowlarr, bazarr, overseerr, tautulli, kometa | WAL state via `hostPath` on ocean; hardlinks need the one whole-pool mount |
| W3 | archmap, photonic_inventory, my_ta_jose, paia, etrading_boy, blog_saetnere_com_wp | external repos; each repo switches its deploy pattern in the same cutover |
| W4 | tdarr server and nodes, llama.cpp, open-webui, mem0 stack, plex | GPU; plex in quiet hours; mem0 is the agents' shared memory, so cut it over with no agent run in flight |
| W5 | homeassistant | hostNetwork |
| W6 | prometheus, loki, grafana, alertmanager | this is phase 5, after the external dead-man alert exists |
| W7 | nginx | ingress controller takes 80/443 on ocean via hostNetwork; certbot retires to cert-manager |

At the end of W7, Docker on ocean is empty and is removed.

**Step 5: first passthrough swap.** Ocean is rebuilt as a clean Debian 13 k8s
node using the passthrough swap procedure below. This is also ocean's OS major
upgrade: it is on Debian 12, which left regular security support in June 2026
and is on LTS until June 2028.

### OS lifecycle: patch continuously, never upgrade a major in place

Two different things, handled differently:

**Patches (packages, kernels, CVE fixes): continuous, automated, rolling.**

- `unattended-upgrades` for the Debian security origin on every VM. It
  installs; it does not reboot.
- Held packages on the passthrough VMs and anything that builds a kernel
  module: NVIDIA driver, ZFS (`zfs-dkms`, `zfsutils-linux`), the kernel itself.
  Those move in the supervised monthly window, and the window checks
  `dkms status` shows both the NVIDIA and ZFS modules built for the new kernel
  **before** rebooting. A ZFS module that failed to build means a pool that
  does not import on boot.
- Reboots on k8s nodes: kured, in a weekly window that does not overlap the
  Sunday 08:00 to 14:00 Pacific image refresh, one node at a time, drain first.
  Ocean is in the rota; its reboot is a few minutes of downtime for the GPU and
  local-state pods, and that is accepted inside the window.
- Reboots on tier 0 VMs: one at a time in the same window, driven by the
  patch play (Ansible now, Salt later). **The DNS pair is excluded from any
  automation an agent writes or runs**; terrac patches it, never both at once.
- Proxmox hosts: monthly patch window, hand-run. A hypervisor reboot takes
  every VM on it down (no shared storage, and passthrough VMs cannot
  live-migrate), so it is scheduled. PVE major upgrades are in place by
  necessity (bare metal) and get their own runbook, one node at a time.
- Visibility: a node-exporter textfile metric for pending security updates and
  `reboot-required`; alert when security updates are pending more than 7 days
  or a reboot is pending more than 14 days. Falling behind becomes an alert,
  not a discovery.
- Container images: Renovate (see platform table) covers CVEs inside images.

**OS majors: replace the VM, never `do-release-upgrade` in place.** Every VM
moves to a new Debian major within 6 months of its release, and always before
the old major's regular security support ends. The Proxmox template (VMID
9999, Debian 12 today) is rebuilt for each new major and point release. Three
replacement patterns:

| VM kind | Examples | Pattern |
|---|---|---|
| Plain k8s agent | future agents without passthrough | Blue/green: new VM joins, old one is cordoned, drained, removed. Zero app downtime for anything with a second replica or no local state. |
| k3s server | `k3s-server01` | New VM built and patched, old one stopped, the nightly SQLite backup (plus token and `server/tls`) restored onto the new one, which takes the old IP and MAC. API down for minutes, workloads keep running. This is the same restore rehearsed in phase 1. |
| Passthrough VM | ocean (GPU plus HBA plus pool), `k3s-agent01` (passed-through scratch disk) | The passthrough swap, below. |
| Tier 0 VM | Salt master, registry-cache, runners, agentbox | Rebuilt from config (Ansible now, Salt later), cut over by IP. DNS pair: terrac only. |

### Passthrough swap procedure

Applies to any VM that owns hardware only one VM can hold at a time. Written
for ocean, the hardest case; `k3s-agent01` is the same without the pool and GPU.

**Before the window (no downtime, can take days):**

1. Create the new VM (new VMID, for example 5001) from the current template,
   on node006, with a temporary IP and **small RAM** (node006 cannot run two
   366 GiB VMs). No `hostpci` devices yet.
2. Apply all host config and patches: base system, NVIDIA driver at the same
   or newer version, ZFS at the same or newer version than the pool's feature
   flags need, NFS server config, node-exporter, k3s agent installed at the
   pinned version but **not started**. The GPU and pool cannot be tested yet;
   check `dkms status`, `modinfo nvidia`, `modinfo zfs`.
3. Copy the old VM's `net0` MAC to the new VM's config (applied at the swap),
   so DHCP reservations and ARP do not change. Set the target RAM and CPU so
   the VM boots at full size after the swap.
4. Written checklist signed off: versions, held packages, labels, taint.

**The window (target under 30 minutes of ocean-hosted downtime):**

1. Cordon and drain the old node. Scale down pods on other nodes that hold the
   `/data01` NFS mount (tdarr-node, nzbget) so nothing parks in D state.
2. Hand-run, per the ZFS rules in `agents.md`: `zfs snapshot -r` of
   `data01/services`, then `zpool export data01`. **Not in any committed play.**
3. Shut down the old VM.
4. On node006: remove `hostpci*` from the old VM, add them to the new one;
   give the new VM the final IP (192.168.1.143) and the old MAC; give the old
   VM a dead network config and `onboot: 0`. A dispatch-only play may do this
   Proxmox step; the ZFS steps stay manual.
5. Start the new VM. Check `nvidia-smi`. Hand-run `zpool import data01`
   **without `-f`**; if it asks for `-f`, stop and find out why (hostid, an
   unclean export) before forcing. Never `zpool upgrade` in the window. Start
   the NFS server; NFS clients recover on their own (`hard` mounts).
6. Delete the old Node object and its `ocean.node-password.k3s` secret in
   `kube-system`, then start the k3s agent. The node rejoins as `ocean`, so
   monitoring names do not change, with the same labels and taint.
7. Uncordon. Pods reschedule onto it by label. Verify GPU pods, every blackbox
   probe, and the NFS-dependent pods scaled back up.

**Backout (any time until the old VM is deleted):** stop the new VM, hand-run
`zpool export`, move `hostpci*` and the MAC back, start the old VM, rejoin.
The old VM is kept stopped and untouched for 14 days, then deleted.

### Platform components

| Concern | Today | Target |
|---|---|---|
| Deploy | `main-apply` runs a playbook | Argo CD app-of-apps from `homelab-kube` `master`. Auto-sync; `prune: true`, `allowEmpty: false`. |
| External repo deploys | `repository_dispatch` to `homelab`, playbook pulls `:latest` | CI commits a tag bump to `homelab-kube` (or Argo CD Image Updater writes back). `docs/operations/deploy-pattern.md` gets a k8s section. |
| Weekly image refresh | `docker-refresh@<project>` timers | Renovate on `homelab-kube`, against pinned tags. |
| Secrets | ansible-vault | SOPS with age, decrypted in-cluster by sops-secrets-operator (already in `homelab-kube`, switched from GPG to age). The age private key is the one bootstrap secret; it stays in the vault and the bootstrap play installs it. |
| Public cert | certbot on ocean, Cloudflare DNS-01 | cert-manager `ClusterIssuer` for Let's Encrypt, Cloudflare DNS-01, same token. |
| Internal TLS | `homelab-ca` (spec 2026-10-14) | cert-manager `CA` issuer backed by the same root. The CA key handling rules from that spec apply unchanged. |
| Metrics | Prometheus on ocean, file-based scrape config | Phases 1 to 4: ocean's Prometheus also scrapes the cluster (kube-state-metrics, kubelet). Phase 5: kube-prometheus-stack in cluster, TSDB on ocean local disk, ServiceMonitors per app. |
| Logs | Loki plus promtail | Loki in cluster plus Alloy (promtail is end of life). |
| Dead-man alert | none outside ocean | An Alertmanager Watchdog to something outside both the cluster and ocean. Open question (PRD Q3). |
| DB backups | `files/db-backup/db-backup.sh` | Per-database CronJobs writing the same GCS object names, so `db-restore.sh` keeps working. Both remain authoritative until the last database moves. |

### Salt

- **Master**: a small VM, `salt01`, on node005. Tier 0. Not in the cluster:
  the master must be able to repair the hosts the cluster runs on, so it cannot
  depend on them. This settles the chicken-and-egg question.
- **Minions**: every Linux host in the inventory, Proxmox hosts included. Not
  the Mac.
- **Repo**: new repo, `homelab-salt` (name is the builder's call).
- **Apply model**: no scheduled highstate. Changes apply through CI, per changed
  state, from the self-hosted runner, mirroring today's impact detection and
  serialization. A scheduled `state.apply test=True` reports drift (coordination
  rule 8).
- **Ownership**: a host concern is owned by exactly one of Ansible or Salt at a
  time. The PR that turns on a Salt state deletes the Ansible role or play that
  owned the same concern.
- **Hard limits carry over**: DNS, sshd, `authorized_keys`, the fleet key and
  port 22 firewall rules may be ported to Salt only by terrac, by hand.
- **Order**: base system first (packages, users, node-exporter, docker on the
  remaining Docker hosts), then the k3s install and upgrade (after phase 3, so
  the playbook that survived three upgrades is the reference), then Proxmox
  hosts, DNS last.

## What stays in Ansible until Salt replaces it

VM creation (`create_*_vm.yaml`), the k3s install and upgrade play, `data01`
export, ZFS read-only checks, DNS, runners, agentbox. Once Salt owns all of it
and the last service has left ocean's Docker, `homelab` is archived.

## Left to the builder

- VM names, IPs (free addresses in the .2-.50 infrastructure range, checked
  against inventory and DHCP reservations), exact vCPU and RAM numbers.
- Chart versions, as long as each one's supported Kubernetes range is recorded
  in the upgrade log.
- Which three toy apps run the upgrade drill, as long as they cover ingress plus
  a cert, an NFS PV, and a sops secret.
- Salt repo layout.
