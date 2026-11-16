# Storage: ZFS and NFS

`/data01` is the one thing in the fleet that can't be rebuilt from this repo, so
these two rules are invariants, not preferences.

## ZFS / storage: NEVER automate (data-safety invariant)

`/data01` (the ZFS pool) is the most critical infrastructure in the fleet; losing data
is the worst possible outcome. So storage is exempt from "merge = deploy":

- **No committed playbook may mutate ZFS.** Enforced by CI: `check-no-zfs-mutations.sh`
  (in the Validate Deploy Mapping job) fails the build on any mutating `zfs`/`zpool` verb
  (`create/destroy/set/mount/unmount/import/export/rollback/replace/...`), the
  `community.general.zfs|zpool` modules, or a `fstype: zfs` mount. Reads (`zpool list`,
  `zfs get`) and non-ZFS mounts (tmpfs) are fine.
- **Storage playbooks are dispatch-only.** The detector treats any `playbooks/**/*zfs*.yaml`
  as never-auto-apply (like terminalbench). Name a storage play `*zfs*.yaml` to inherit the
  guard; run it deliberately via `workflow_dispatch`.
- **The only in-repo ZFS play is read-only.** `playbooks/individual/ocean/data01_zfs.yaml`
  runs `become: false` (unprivileged — cannot mutate), timeout-bounds every command (a
  suspended pool blocks `zfs`/`zpool` forever), asserts `/data01` is mounted (fails if not),
  warns on non-ONLINE health, and records the live layout. It treats the running config as
  gold: it asserts reality is functional, never imposes config.
- **Real pool changes** (replacing a degraded disk, dataset work) are coordinated, hand-run,
  snapshot-first tasks with a backout plan — a runbook, not a committed auto-runnable play.
  Beware side effects: an unmount with open file handles makes services write into the empty
  mountpoint on root (invisible after remount), and property changes (`mountpoint`,
  `canmount`) silently trigger remounts.
- **Always identify a drive by WWN, never by `/dev/sdX`.** `zpool replace pool device
  [new-device]` takes an ordinary vdev path in *both* positions, so
  `/dev/disk/by-id/wwn-*` is always available and is the only form to use. `sdX` names are
  reassigned across reboots, so a command, an alert matcher or a silence keyed on one can
  silently come to mean a different physical disk.
  This is not hypothetical: of four `data01` replacements in August 2026, three passed a
  `wwn-` path and the 2026-08-18 one passed `ata-ST12000NM0127_ZJV3KB10`. That member is
  still imported under its ATA ID, and nothing detected the deviation because nothing was
  checking. A convention honoured three times in four reads as a convention that works.
  Both are stable by-id paths so no data was at risk, but anything matching on a `wwn-`
  prefix skips that disk. `disk_wwn_info` (`files/node-exporter/zpool-metrics.sh`) now maps
  kernel name -> wwn -> serial for every disk, so monitoring is no longer blind to it.
- **Every part swap gets a row in [`docs/operations/hardware-repair-log.md`](../operations/hardware-repair-log.md)**
  in the same change: part number, serial, WWN, date, vendor, order, price, and FARM vs SMART
  hours for used Seagate drives. It is the source of truth for hardware history; don't
  reconstruct that history from `zpool history`, Prometheus or email when the log can say it.
  Full record and the rename options: [`docs/operations/zfs-disk-replacement.md`](../operations/zfs-disk-replacement.md).

## Nothing with a write-ahead log goes over NFS

`/data01` is exported over NFS to worker hosts (architecture:
[`docs/architecture/data01-workers.md`](../architecture/data01-workers.md)),
and every service on ocean keeps its config and database under `/data01/services/<svc>`.
Those two facts together are a trap: run a service on a worker with its config dir on the
NFS mount and its database corrupts.

The rule is anything that keeps a WAL or journal, not just SQLite. These stores rely on
local-filesystem guarantees NFS doesn't give: POSIX byte-range locks that hold, `fsync`
ordering, and (for SQLite WAL mode) a shared-memory `-shm` index that is mmapped and only
works on one host. Over NFS the locks hold until a lease expires or a client stalls
mid-write, and then two writers share the file. SQLite (every *arr app, Plex, Jellyfin,
Grafana, ntfy), MySQL InnoDB, Postgres, Prometheus TSDB, Loki, etcd and Kafka logs are all
in scope. The *arr apps, Plex and Jellyfin document "no config on a network share" for
this reason.

- A service's state dir (anything with a database, WAL or journal) is always on a
  **local** disk on the host that runs it. On a worker that is the local disk mounted over
  `/data01/services`, never the NFS view of ocean's.
- Bulk media and download payloads are what NFS is for.
- Moving a service to a worker means copying its state to the worker's local disk, not
  pointing the container at ocean's copy over NFS. Two hosts never open one DB.
