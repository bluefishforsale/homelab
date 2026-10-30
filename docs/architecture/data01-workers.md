# data01 workers (harbor)

Docker hosts other than ocean that see ocean's `/data01` pool over NFS and keep
their own service state on a local disk. They exist so work that only needs the
media (transcodes, downloads) can leave ocean without leaving the pool. Today the
only one is **harbor**, a VM on node005. The rule they live under is in
[`agents.md`](../../agents.md#nothing-with-a-write-ahead-log-goes-over-nfs):
nothing with a WAL crosses NFS.

Membership is the `[data01_workers]` inventory group. Adding a host there gets it
the export, the mounts, the alerts, and a tdarr node; nothing else names harbor
except its VM play and NZBGet.

## Playbooks

| Playbook | Host | What it owns |
|---|---|---|
| `infrastructure/create_harbor_vm.yaml` | node005 | VM 6000 at 192.168.1.32: CPU `host`, 24 cores (`cpuunits` 200), 32 GiB, `virtio-scsi-single` with an iothread per disk. scsi0 OS (~64G), scsi1 200G state disk on `vmpool-storage`, scsi2 the passed-through 870 EVO 2TB (`wwn-0x5002538f54b047b7`) as NZBGet scratch. Disk and CPU changes only apply to a stopped VM. |
| `ocean/nfs_export_data01.yaml` | ocean | NFSv4.2 only (`vers3=n`). One export of all of `/data01` to each worker's IP: `rw,sync,no_subtree_check,root_squash,sec=sys,fsid=1`, no `crossmnt`. Netboot hosts get `/data01/complete` only, `all_squash` to media, `subtree_check`. Refuses an empty client list. Never touches ZFS or restarts anything but nfs-server. |
| `infrastructure/data01_worker.yaml` | ocean, then workers (`serial: 1`) | Asserts ids match ocean, formats the state disk if blank, migrates existing state onto it on first mount, mounts NFS at `/data01` and the state disk over `/data01/services`, installs the docker guards and a retry timer. |
| `workers/tdarr_node.yaml` | `data01_workers` | CPU tdarr node joined to ocean's tdarr server. |
| `workers/nzbget.yaml` | ocean, then harbor | NZBGet compose project with scratch on the 870 EVO; only finished files cross NFS into `/data01/incoming`. Retires the old ocean units. Prune timer. ocean's nginx proxies it at `192.168.1.32:8901`. |
| `infrastructure/db_restore_test.yaml` | harbor | Nightly restore test of the DB backups ([`db-restore.md`](../operations/db-restore.md)). |

## Mounts

```
192.168.1.143:/data01  /data01           nfs4  nfsvers=4.2,hard,proto=tcp,nconnect=4,rsize=1048576,wsize=1048576,noatime,_netdev,x-systemd.mount-timeout=30
<scsi1 by-id>          /data01/services  ext4  defaults,noatime,_netdev,x-systemd.requires=data01.mount
```

- One export of the whole pool, because the *arr apps import by hardlink from
  `/data01/complete` to `/data01/media` and a hardlink across two NFS mounts
  fails with `EXDEV`.
- `hard`, not `soft`: a stalled ocean parks processes in D state instead of
  handing them EIO mid-write.
- `_netdev` on the local disk is deliberate. Without it, requiring the NFS mount
  from a `local-fs.target` mount is an ordering cycle and systemd drops the NFS
  mount at boot.
- Server by static IP, not `ocean.home`: `*.home` names have flapped under DDNS
  collisions.

## The overmount guard

The local disk mounts over `/data01/services`, which also exists on the NFS side
as ocean's live service dirs. If the local mount is missing, a worker's
containers write into ocean's databases over NFS. Three guards, all required:

1. `docker.service` has `RequiresMountsFor=/data01 /data01/services`.
2. docker's `ExecStartPre` refuses to start unless `/data01/services/.worker-local`
   exists. Only the local disk carries that file.
3. The first play asserts ocean never has it.

NOTE: drop any one and a failed local mount points a worker at ocean's live DBs.

If node005 boots before ocean, the NFS mount times out, docker fails its
dependency, and `data01-worker-retry.timer` retries until ocean is back.

## Identity

NFS `sec=sys` matches by number, so every worker must have the same uid/gid as
ocean for each `system_users` entry (terrac 2000, media 2100 since #702). Both
plays assert it. `root_squash` means a container writing as root becomes
`nobody`, so services set PUID/PGID explicitly.

## Monitoring

Workers are in `[vms]`, so node-exporter, cadvisor and promtail cover them.
Alerts in `files/ocean-prometheus/alert_rules.yml.j2` match `[data01_workers]`,
not harbor by name: `Data01WorkerNfsMissing`, `Data01WorkerStateDiskMissing`,
`Data01WorkerNfsRetransmits`. DNS is `harbor.home` A and PTR in
`files/dns-stack/import-zones.sh.j2`; a moved service keeps its `<svc>.home`
name and ocean's nginx proxies to the worker.

## Not built

- **Backups of worker state.** `db-backup.sh` runs on ocean through
  `docker exec` and can't see a worker's containers, and there is no rsync of
  `/data01/services` back onto the pool. Today that state is NZBGet's and the
  tdarr node's config, both rebuildable; a service with a real database must not
  move to a worker until this exists.
- **A host registry.** A service picks its host by sitting in `workers/` with
  `hosts:` set to `harbor` or `data01_workers`. Fine at one worker.

## Backout

Stop the worker's services and move them back to ocean's playbooks. Removing a
host from `[data01_workers]` drops it from the export on the next
`nfs_export_data01.yaml` run. Nothing on the pool changes either way.
