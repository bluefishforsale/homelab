# node005 docker host with /data01 over NFS (design)

Status: draft for review, 2026-10-17. Nothing here is built yet.

Working name for the VM is `harbor` (open decision D3). Everything below says
"harbor" so the spec reads concretely; swap the name once picked.

## 1. Goal

A repeatable pattern for **data01 worker hosts**: Docker VMs that run compose
services the same way ocean does (same `/data01/...` paths, same compose-project
layout under `/data01/services/<svc>`, same image refresh, same monitoring, same
deploy path through CI), with `/data01` arriving over NFS from ocean instead of
from local disks. The disk array stays on ocean; ocean is the storage server and
the workers operate on it.

harbor on node005 is the first instance. More hosts get the same shape as they
are added, so nothing in this spec may be harbor-specific where a host group
would do: the export list, the sentinel, the alerts and the inventory group all
key on membership, not on one name or IP.

What the workers are for (D2, answered 2026-10-19): **CPU and I/O offload for
transcoding and downloading.** tdarr CPU workers first, nzbget later (section
5.1), then other services that can move, or that can run as more than one
instance.

## 2. Sanity check (read this first)

The idea is sound in shape and I'd build it, but three of the assumptions in
the one-line version don't hold, and each one changes the design.

### 2.1 node005 does not have the RAM free today

Measured this session:

| | node005 | node006 |
|---|---|---|
| Physical RAM | 125 GiB | 660 GiB |
| `free` available | 22 GiB | 290 GiB |
| Threads | 56 (2x E5-2690 v4) | 40 (ocean holds 30 vCPU) |
| VMs | dns01 2G, pihole 1G, registry-cache (4000) 2G, gh-runner-01 64G, registry-cache-01 (7000) 2G, agentbox 12G | dns02 2G, ocean 357G |
| ZFS ARC | 61.7 GiB of a 62.9 GiB max (default half of RAM) | n/a |

Two things eat node005: the ARC on `vmpool-storage` (942 GB pool, 30% used,
backing VM disks only) and gh-runner-01, which is allocated 64 GiB and was
using 3 GiB inside the guest (`free -g`: 62 total, 3 used, 59 available).

So node005 is the right host for CPU (lots of idle threads), node006 is the
right host for RAM. A useful harbor (24 to 32 GiB) only fits on node005 if
we reclaim memory first. Cheapest options, in order of how much I like them:

1. Cap node005's ARC at 32 GiB (`options zfs zfs_arc_max=34359738368` in
   `/etc/modprobe.d/zfs.conf`, plus a live write to
   `/sys/module/zfs/parameters/zfs_arc_max`). Frees about 30 GiB. That pool
   only backs VM disks, so 32 GiB of ARC is still generous. Backout: delete
   the line, write 0 to the parameter. This is a module tunable, not a pool or
   dataset mutation, so it doesn't trip `check-no-zfs-mutations.sh`, but in the
   spirit of the storage rule I'd still hand-apply it the first time.
2. Shrink gh-runner-01 from 64 GiB to 16 GiB. Frees 48 GiB of allocation.
   Runner changes are manual (a runner can't redeploy itself), and it needs a
   VM restart, so schedule it when no CI is running.
3. VM 4000 `registry-cache` is running on node005 but is not in the
   inventory (the inventory's registry cache is 7000 `registry-cache-01`).
   Looks like a leftover. Not in scope here: noted as a backlog item, not
   acted on.

Measured 90-day peaks (node-exporter, `MemTotal - min_over_time(MemAvailable[90d])`,
no swap use and zero OOM kills on any of them) against what each VM holds:

| VM | Allocated | 90d peak used | Host RSS now | Proposed |
|---|---|---|---|---|
| gh-runner-01 (5000) | 64 GiB | 3.97 GiB | 32.5 GiB | 12 GiB |
| agentbox (8000) | 12 GiB | 3.4 GiB | 11.6 GiB | 8 GiB |
| dns01 (2000) | 2 GiB | 1.19 GiB | 2.0 GiB | unchanged (DNS) |
| registry-cache-01 (7000) | 2 GiB | 1.19 GiB | 2.0 GiB | unchanged |
| pihole (3000) | 1 GiB | 0.85 GiB | 1.0 GiB | unchanged |
| registry-cache (4000) | 2 GiB | no exporter | 2.0 GiB | backlog; not in repo, IP .40, no SSH key |

node005's ARC hit rate is effectively 100%, but 53 GiB of the 62 GiB is MRU
(read once), and the hot MFU set is 11 GiB. Most of it double-caches VM disk
blocks the guests already cache themselves, so a 32 GiB cap keeps the hot set
three times over.

After ARC 32 GiB, runner 12 GiB and agentbox 8 GiB: available goes from about
22 GiB to about 77 GiB, and committed worst case (every VM full plus ARC at
cap) drops to about 59 GiB of 125.8, which leaves room for a harbor of up to
48 GiB with about 14 GiB of slack.

Decision D1 picks which.

### 2.2 "Looks like it's on ocean" holds for media, not for service state

On ocean, every service keeps its config and its databases under
`/data01/services/<svc>`, which is on the ZFS pool. 28 service dirs hold
SQLite or database files right now (sonarr, radarr, prowlarr, overseerr,
bazarr, homeassistant, grafana, jellyfin, ntfy, open-webui, mem0, mysql and
more).

SQLite over NFS is the classic way to corrupt a database: its locking relies
on POSIX byte-range locks behaving like a local filesystem, and over NFS they
mostly do, until a lease expires or a client hangs mid-write. The *arr apps
and Plex all document "don't put the config dir on a network share" for this
reason.

So harbor cannot take `/data01/services` over NFS. The design keeps the paths
identical anyway: NFS-mount ocean's `/data01` at `/data01`, then mount a local
disk over `/data01/services`. Containers see the same paths as on ocean; bulk
data goes over the wire, state stays local. Section 4.3 covers the ordering
hazard this creates.

Consequence: service state on harbor is not on data01, so it loses ZFS
redundancy and snapshots, and it's not reached by the existing DB backup
(section 4.7).

### 2.3 It doesn't add resilience for anything that touches /data01

`/data01` lives inside the ocean VM (the HBA is PCI-passed through as
`hostpci1: 02:00`). If ocean is down, the NFS export is down, and every harbor
container reading `/data01` hangs on a hard mount. Harbor is only more
available than ocean for services that never touch `/data01` outside their
own local state dir.

That's fine if the goal is capacity, CPU offload, or a second place to run
stateless or local-state services. It's the wrong tool if the goal is
"media keeps working when ocean is down." D2 is answered as offload (section
1), which this fits.

"HA instances" therefore means two different things here, and only one of them
is real for data01 work:

- **Redundant workers.** Several tdarr nodes across hosts, any one of which can
  die while the rest keep pulling jobs. That is real redundancy, but it ends at
  ocean: the tdarr server and the export both live there.
- **Services that never touch data01.** These can run a second instance on a
  worker and genuinely survive an ocean outage (the DNS pair is the existing
  example of this shape).

Anything whose job is reading or writing `/data01` is exactly as available as
ocean, however many copies of it run. D11 picks which of the two is in scope.

### 2.4 ocean is hard-coded all over the repo

Moving any service off ocean is more than changing where a container runs:

- 39 playbooks under `playbooks/individual/ocean/` say `hosts: ocean`.
- `vars/vars_cloudflared.yaml` points ingress at `http://192.168.1.143:<port>`
  (30 references).
- `files/ocean-prometheus/prometheus.yml.j2` has 40 references to ocean's
  address or name.
- DNS: `*.home` is a wildcard A record to 192.168.1.143 (ocean nginx), in
  `files/dns-stack/import-zones.sh.j2:84`.
- `files/db-backup/db-backup.sh` runs on ocean (`playbooks/operations/backup/db_backup.yaml:16`,
  `hosts: ocean`) and backs up via `docker exec`, so it only sees ocean's
  containers.

Phase 1 doesn't fix any of that wholesale. It builds the host and moves one
canary. Phase 2 (section 7) is where per-service host placement becomes a
first-class variable.

### 2.5 Things that are fine

- Network: both hypervisors are on `bond0`, 802.3ad over 2x 10G (measured).
  A single NFS TCP flow hashes onto one link, so 10G per flow; `nconnect=4`
  spreads it. Media streaming and *arr imports are nowhere near that.
- CPU: node005's v4 Xeons are newer than ocean's v2 and have AVX2; with
  `cpu: host` (same as agentbox) anything Bun-based or AVX2-hungry works.
- The VM creation pattern already exists three times
  (`create_agentbox_vm.yaml`, `create_runner_vm.yaml`,
  `create_registry_cache_vm.yaml`), so this is a copy, not an invention.
- Monitoring mostly comes for free: a host in `[vms]` gets node-exporter,
  cadvisor and promtail from the fleet loops.

## 3. Anti-goals

These are out of scope. If a later step needs one, it's a new spec.

- No automatic failover. A worker does not take over when ocean dies; any
  redundancy is a second instance named per service (D11).
- No GPU work on harbor: no Plex/Jellyfin hardware transcode, no llamacpp,
  no comfyui, no frigate.
- No ZFS changes of any kind on data01. No `zfs set sharenfs`, no new
  datasets, no property changes. The export is `/etc/exports` on ocean's
  kernel NFS server.
- No SQLite, MySQL, Postgres, Prometheus TSDB or Loki chunks on NFS. Ever.
- No moving the monitoring stack (Prometheus, Loki, Grafana, Alertmanager).
  Monitoring that depends on NFS can't report the NFS outage.
- No Kubernetes, Swarm, Nomad or any scheduler. Placement is a variable in
  the repo, decided by a human.
- No exporting data01 to anything outside the `[data01_workers]` inventory
  group. Adding a host to the group is the only way it gets the export.
- No bulk migration in phase 1. Exactly one canary service moves.
- No changes to ocean's existing paths, compose files, or service layout.
- No decommissioning of anything on node005 (VM 4000 stays a backlog item).
- No auto-rollback (standing default: halt and alert).

## 4. Design

### 4.1 The VM

New playbook `playbooks/individual/infrastructure/create_harbor_vm.yaml`,
cloned from `create_agentbox_vm.yaml` with these values:

| Setting | Value | Why |
|---|---|---|
| Host | node005 | the ask |
| VMID | 6000 | free on node005 (in use: 2000, 3000, 4000, 5000, 7000, 8000, 9999) |
| Template | 9999 (Debian 12 cloud-init) | same as every other VM |
| CPU | `host` | AVX2 passthrough, see kvm64 note in the agentbox play |
| Cores | 16 | node005 has 56 threads, existing VMs hold about 28 |
| Memory | 24 GiB (D1) | fits only after memory is reclaimed |
| Root disk | grow to about 64G | OS + `/var/lib/docker` |
| Second disk | 200G on `vmpool-storage`, ext4, mounted at `/data01/services` | local service state (2.2) |
| `onboot` | 1 | |
| IP | 192.168.1.32 static (D3) | next to agentbox .31; verify free |
| NIC | virtio on `vmbr0`, `queues=8` | same bridge as everything else |

Two disks rather than one, so service state can be snapshotted, resized,
or backed up independently of the OS, and so the overmount is a real
separate filesystem the boot ordering can check.

### 4.2 NFS server on ocean

New playbook `playbooks/individual/ocean/nfs_export_data01.yaml`, `hosts: ocean`.

- Installs `nfs-kernel-server` (not present today; `exportfs: command not
  found` on ocean).
- Manages one file, `/etc/exports.d/data01-workers.exports`, templated with
  one client entry per host in `[data01_workers]`, so a new worker gets the
  export from an inventory change and nothing else:

  ```
  /data01  192.168.1.32(rw,sync,no_subtree_check,root_squash,sec=sys,fsid=1)
  ```

  (one such client spec per worker on the same line).

- One export of the whole `/data01`, not separate exports for `media`,
  `complete`, `incoming`. The *arr apps import by hardlink from
  `/data01/complete` into `/data01/media`, and hardlinks fail with `EXDEV`
  across two NFS mounts even when they're the same filesystem on the server.
  One mount keeps hardlinks working.
- No `crossmnt`, so the `data01/docker` child dataset (and its pile of legacy
  per-layer datasets) isn't traversed.
- `exportfs -ra` as a handler, never a restart of anything else on ocean.
- NFSv4.2 only (`/etc/nfs.conf`: `vers3=n`), so there's one TCP port (2049)
  and no rpcbind/mountd dance.
- ocean's INPUT policy is ACCEPT with only a fail2ban sshd chain (measured),
  so the export's host list is the only access control. Acceptable on the
  home LAN; noted.

The existing `playbooks/individual/infrastructure/nfs_server.yaml` is a
landmine: `hosts: all`, exports `/srv/nfs4` to `*` read-write, mode 0777, and
restarts nfs-server. If anyone runs it, every host becomes a world-writable NFS
server. It's not this work, but the new playbook should not resemble it, and
deleting or fencing it belongs on the backlog (D9).

### 4.3 NFS client and mount ordering on harbor

Mount options (fstab via `ansible.posix.mount`):

```
ocean.home:/data01  /data01  nfs4  nfsvers=4.2,hard,proto=tcp,nconnect=4,rsize=1048576,wsize=1048576,noatime,_netdev,x-systemd.mount-timeout=30  0 0
/dev/disk/by-id/<scsi1 id>-part1  /data01/services  ext4  defaults,noatime,x-systemd.requires=data01.mount  0 2
```

Mount by stable id for the local disk, same reason drives go by WWN.

`hard`, not `soft`: a soft mount returns EIO mid-write and the app writes a
half file and moves on; hard makes processes wait in D state until ocean
returns. Waiting is the honest failure.

The hazard: the local disk overmounts a directory that also exists on the
NFS side, ocean's live `/data01/services`. If the local mount fails or comes
up in the wrong order, harbor's containers write straight into ocean's
service dirs, into ocean's live sonarr DB, over NFS. That's the worst case
in this whole design, so it's guarded three ways:

1. `docker.service` drop-in: `RequiresMountsFor=/data01 /data01/services`.
2. A sentinel: the local disk carries `/data01/services/.worker-local`
   (written once by the playbook on the raw filesystem). A docker
   `ExecStartPre=/usr/bin/test -f /data01/services/.worker-local` refuses to
   start dockerd if the path resolves to ocean's dir instead. Same file name
   on every worker, so the guard is part of the pattern, not of harbor.
3. ocean's `/data01/services` never gets that file (asserted by the NFS
   playbook on ocean: fail if it exists).

NOTE: if the sentinel check is ever dropped, a failed local mount silently
points harbor's containers at ocean's live databases.

Boot order across nodes can't be expressed in Proxmox, so if node005 boots
before ocean, the NFS mount retries until `x-systemd.mount-timeout`, docker
fails its dependency, and `Restart=on-failure` plus a timer retries. Harbor
sits idle rather than half-up.

### 4.4 Identity

Containers writing to NFS must use the same numeric uid/gid as on ocean:
`terrac` 1000:1002 and `media` 1001:1001 (measured on ocean). The base system
play creates users; harbor must end up with identical uid/gid for both,
asserted by the play. `root_squash` means a container running as root that
writes to `/data01` becomes `nobody` and gets permission errors, which is
what we want: it forces PUID/PGID to be set explicitly. (D5 if you'd rather
`no_root_squash`.)

### 4.5 Docker and image refresh

Same as ocean, via the existing plays with `--limit harbor`:

- `01_base_system` (users, keys, resolvers).
- Docker CE. `docker_ce.yaml` is high blast radius (state: latest, dockerd
  bounce per host), so harbor's first run uses `--limit harbor` only. No
  NVIDIA packages (`nvidia_gpu` unset).
- `daemon.json` with the registry mirror at registry-cache-01, same as
  other VMs.
- `docker-refresh@<project>` timers come from `docker compose ls`, so they
  appear as soon as a project runs.

### 4.6 Monitoring

- Inventory: add harbor to `[vms]` (picks up node-exporter, cadvisor,
  promtail) and a new `[docker_hosts]` group containing ocean and harbor, for
  plays that should target "anywhere compose runs." `[data01_workers]` is the
  subset that mounts the export (harbor now, every later worker), and the
  alerts below match on it rather than on `harbor` by name.
- node-exporter on harbor gets `--collector.mountstats` and `--collector.nfs`
  so NFS latency and retransmits are visible.
- Alerts (in `files/ocean-prometheus/alert_rules.yml.j2`):
  - `HarborData01Missing`: `node_filesystem_size_bytes{instance=~"harbor.*",mountpoint="/data01",fstype="nfs4"}` absent for 5m.
  - `HarborServicesNotLocal`: `/data01/services` fstype is not `ext4` (the overmount failed).
  - `HarborNfsRetransmits`: rate of RPC retransmits above a threshold for 10m.
- Exact metric names verified against the live exporter before the rules
  merge (prior lesson: `unifi_*`, not `unpoller_*`).
- `scripts/exporter-scrape-check.sh` and `scripts/prom.sh targets` show
  harbor's targets up.

### 4.7 Backups

Harbor's service state is on a single virtual disk on node005, not on data01.
Two pieces:

- DB dumps: `db-backup.sh` runs on ocean through `docker exec`, so it can't
  see harbor's containers. Phase 1 runs the same play against harbor
  (`hosts: docker_hosts`), with the GCS object path keyed by host so the two
  don't overwrite each other. `db-restore.sh` gets the same treatment.
- Config dirs: a nightly systemd timer on harbor rsyncs `/data01/services/`
  (excluding live DB files, which the dump covers) to
  `/data01/backups/harbor/services/` over NFS. That lands harbor's state on
  ZFS, where it's under data01's redundancy.

### 4.8 DNS and ingress

- `harbor.home` A 192.168.1.32 and PTR, in `files/dns-stack/import-zones.sh.j2`
  (both DNS servers, per the DNS rollout rules).
- The `*.home` wildcard stays on ocean. A service that moves to harbor gets an
  explicit record, or is reached through ocean's nginx proxying to harbor.
  Explicit records are simpler; D7.
- cloudflared ingress for a moved service points at `192.168.1.32:<port>`.

### 4.9 How a playbook targets harbor

Phase 1 uses the smallest change that works: an ocean service playbook that is
allowed to move changes `hosts: ocean` to `hosts: "{{ service_host | default('ocean') }}"`,
and the canary sets `service_host: harbor` in its own vars file. One line per
playbook, only for services that actually move, and the impact detector maps
the vars file to the playbook as it does today. Phase 2 (section 7) replaces
this with a registry.

## 5. Canary

One service moves in phase 1 to prove the whole path: NFS read and write,
uid mapping, hardlinks, local state, backups, monitoring, ingress.

Recommended canary: a **tdarr node** (CPU transcode worker). It reads and writes
`/data01/media` and `/data01/transcode`, has no database of its own (the tdarr
server keeps that on ocean), is designed to run remotely with shared paths, and
puts node005's idle 56 threads to work, which is a real benefit rather than a
demo. Failure costs nothing: the server just stops handing it jobs.

Fallback canary if you'd rather start without NFS writes: `ntfy` or `karma`
(local state only, no `/data01` traffic). That proves the host and monitoring
but not the NFS path. D6.

### 5.1 nzbget (after the canary, not phase 1)

nzbget is the second named workload, and it fits the overmount design better
than it first looks. Measured on ocean 2026-10-19:

- Its heavy I/O (article writes, par2 repair, unrar) happens in
  `InterDir=/scratch/intermediate` and `TempDir=/scratch/tmp`, and `/scratch` is
  `/data01/services/nzbget/scratch`. On a worker that path is the local disk,
  so the random-write work stays local and only the finished move to
  `DestDir` (`/data01/incoming/...`) crosses NFS, as one sequential copy.
- Its queue and config are under `/data01/services/nzbget` too, so they stay
  local, which they must.

Three things block moving it:

1. It is a bare `docker run` systemd unit (`files/ocean-nzbget/nzbget.service.j2`),
   not a compose project, so it has no `docker-refresh@` timer today. Converting
   it to compose is a prerequisite on ocean, its own PR, before any move.
2. `scratch/intermediate` holds 724G, 633 GiB of it in 109 of 115 dirs older
   than 7 days: failed or abandoned downloads that nothing cleans up. node005's
   `vmpool-storage` is 942 GB in total, so the worker's local disk can't hold
   that. Retention (or a cleanup of what's there) has to land first. D10.
3. sonarr and radarr talk to nzbget by URL, so its address moves with it
   (placement registry, section 7).

## 6. Acceptance criteria (phase 1)

1. `qm config 6000` on node005 shows `cpu: host`, the agreed memory, two
   disks, `onboot: 1`; node005 `free -g` available stays above 8 GiB after
   harbor starts.
2. `findmnt /data01` on harbor shows `nfs4` from `ocean.home:/data01` with
   `vers=4.2,hard`; `findmnt /data01/services` shows `ext4` from the local
   disk.
3. With the local disk unmounted, `systemctl start docker` fails on the
   sentinel check (tested once by hand, then remounted).
4. As uid 1000, `touch /data01/transcode/harbor-test` on harbor shows owner
   `terrac` on ocean; as root it shows `nobody` (root_squash).
5. `ln /data01/complete/x /data01/media/x` succeeds on harbor (hardlink across
   dirs on the one export).
6. Rebooting harbor brings both mounts and the canary back without touching
   anything by hand.
7. Rebooting ocean (or stopping nfs-server for 2 minutes): harbor's canary
   stalls, `HarborData01Missing` or the retransmit alert fires, and it
   recovers on its own once the export returns.
8. Prometheus shows node-exporter, cadvisor and promtail targets for harbor
   up; Loki has harbor's logs.
9. The canary runs as a compose project with a healthcheck, and a
   `docker-refresh@<project>` timer exists for it.
10. The db-backup run for harbor writes its object to GCS (or reports "no
    databases" cleanly for a DB-less canary), and the nightly rsync lands in
    `/data01/backups/harbor/services/`.
11. CI: `make validate`, Validate Deploy Mapping (ownership coverage for the
    new playbooks and vars), and `check-no-zfs-mutations.sh` all green.
12. No change on ocean except the `nfs-kernel-server` package, `/etc/nfs.conf`
    and `/etc/exports.d/data01-workers.exports`.

## 7. Phase 2 (not built now, written down so phase 1 doesn't paint into a corner)

With more than one worker coming, phase 2 is the expected next step, not a
maybe: phase 1's `service_host` one-liner is fine for one canary, but a third
host makes hand-edited addresses in cloudflared, Prometheus and DNS the thing
that breaks.

- `vars/vars_service_hosts.yaml`: a placement registry next to
  `vars_service_ports.yaml`, mapping service to host. cloudflared ingress,
  Prometheus targets and DNS records template from it instead of hard-coding
  192.168.1.143. Impact detection maps a placement change to that service's
  playbook, cloudflared and prometheus.
- Candidate list for moves, by fit:
  - Good: tdarr nodes, exporters that don't scrape ocean-local things
    (unpoller, ndt-speedtest, cloudflare-exporter), ntfy, karma, homepage,
    n8n, paia and other small web apps with local state.
  - Maybe: *arr apps (local DB, NFS media, hardlinks work), but they lose
    inotify for writes made on ocean's side, so they rely on scheduled
    scans. Fine for sonarr/radarr; check before moving.
  - Never: plex, jellyfin, llamacpp, comfyui, frigate (GPU); prometheus,
    loki, grafana, alertmanager (monitoring must not depend on NFS); mysql;
    zfs-exporter, smart-exporter, node-exporter for ocean (host-local).

## 8. Rollout order

Each step is its own PR (one change per PR, squash merge).

1. Reclaim memory on node005 (D1). Hand-applied for the ARC tunable and the
   runner, then captured in the repo so the next apply is a no-op.
2. `create_harbor_vm.yaml` + inventory entry + DNS record. Merge auto-runs
   the create play on node005.
3. Base system + docker on harbor (`--limit harbor` dispatch).
4. NFS export on ocean. Additive: new package, new export file, no restart of
   any running service.
5. NFS client, local disk, sentinel and docker ordering on harbor; acceptance
   criteria 2 through 7.
6. Monitoring and alert rules; criterion 8.
7. Backups (db-backup on `docker_hosts`, rsync timer); criterion 10.
8. Canary service; criteria 9 and the rest.

## 9. Backout

Harbor is additive, so backing out is removal in reverse:

- Stop the canary on harbor, revert its `service_host` (it redeploys to
  ocean with its state copied back from `/data01/backups/harbor/services/`).
- Remove `/etc/exports.d/data01-workers.exports`, `exportfs -ra`. ocean's services
  never depended on the NFS server, so this is invisible to them. Removing
  the package is optional.
- `qm stop 6000`; destroying it is a destructive step I'd confirm with you
  at the time.
- Revert the ARC cap (delete the modprobe line, write 0) and the runner
  memory if you want them back.

Nothing in this design changes data01 itself, so there's no data01 backout.

## 10. Open decisions (the grill list)

Each one changes what gets built. Defaults in brackets are what I'd do.

- **D1 RAM source.** ARC cap, runner shrink, agentbox trim? [all three: ARC to 32 GiB,
  runner to 12 GiB, agentbox to 8 GiB, harbor at up to 48 GiB.]
- **D2 What is harbor for?** Answered 2026-10-19: CPU and I/O offload for
  transcoding and downloading, as the first of several data01 workers.
- **D3 Name, VMID, IP, and the convention for the next ones.** [harbor, 6000,
  192.168.1.32 after confirming it's free in DHCP and DNS; later workers get
  the next free VMID on their node and an IP from a reserved block.]
- **D4 Export scope.** Whole `/data01` (hardlinks work, ocean's
  `/data01/services` visible but overmounted) or subtrees only (safer, but
  breaks *arr hardlinking). [whole, with the sentinel guard.]
- **D5 Squash policy.** `root_squash` (forces explicit PUID/PGID) or
  `no_root_squash` (fewer surprises, root on harbor is root on data01).
  [root_squash.]
- **D6 Canary.** tdarr node or a no-NFS service first. [tdarr node.]
- **D7 Reaching moved services on .home.** Explicit DNS records or proxy
  through ocean's nginx. [explicit records.]
- **D8 Read-only mounts.** Should harbor get `/data01` read-write, or
  read-only plus specific read-write subtrees? Read-only shrinks the blast
  radius of a bad container a lot but conflicts with D4's single mount.
  [read-write, revisit if anything other than media workers moves.]
- **D9 The old `nfs_server.yaml`.** Fence it now in this work, or backlog it.
  [backlog it, separate PR.]
- **D10 nzbget intermediate retention.** 633 GiB of stale intermediate dirs:
  clean once by hand and add a retention timer, or move `InterDir` to NFS on
  the worker and give up the local-I/O win. [clean plus retention; the local
  scratch is most of the point of moving nzbget.]
- **D11 What "HA" means here.** Redundant workers across hosts (ends at ocean),
  second instances of services that never touch data01 (survives ocean), or
  both. [both, named per service, never for anything whose job is data01.]
