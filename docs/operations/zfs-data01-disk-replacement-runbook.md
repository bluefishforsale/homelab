# Runbook: Replacing a failed drive in the `data01` ZFS pool

**This is a coordinated, hand-run procedure. It is NOT a playbook and must never
be automated.** ZFS/storage changes carry data-loss risk (see the "ZFS / storage:
NEVER automate" invariant in `agents.md`). Run it deliberately, at the console,
with a clear head and a backout plan. Every step here is a human decision.

## Current state (why this is urgent)

`data01` is a single **RAIDZ2 vdev of 8 drives** on `ocean` (a VM on node006 with
the HBA passed through). RAIDZ2 tolerates **2** simultaneous drive failures.

As of the last check (`zpool status -v data01`):

- **2 drives FAULTED** — `wwn-0x5000c500b30c1db0`, `wwn-0x5000c500b2281882`
  ("too many errors").
- **1 more drive degrading** — `wwn-0x5000c500b38b622b`, ~14 read errors, still ONLINE.
- `errors: No known data errors` (no corruption yet).

With 2 drives faulted, **the pool has zero remaining parity**. If a third drive
fails — and one is already throwing errors — the pool is lost. Treat this as
act-soon, not someday.

## Golden rules

1. **The running pool state is gold.** Read and confirm before every action.
2. **Never** `zpool destroy`, `zpool create`, `zfs destroy`, `zfs rollback`, or a
   forced (`-f`) import/export on this pool. None of those belong in recovery.
3. **One drive at a time.** Replace, let it fully resilver, verify, then the next.
4. **Stop on surprise.** If a resilver errors, a new drive drops, or `zpool
   status` shows something you didn't expect, STOP and reassess. Do not push through.
5. **Backups first.** A resilver reads every surviving drive end to end; with no
   parity left, that stress can push the marginal third drive over. Confirm the
   irreplaceable data is backed up elsewhere BEFORE you start (see step 0).

## Step 0 — Before touching anything

- [ ] Confirm off-pool backups of anything irreplaceable exist and are current.
      If not, back up first — resilvering a zero-parity pool is the single most
      likely moment to lose data.
- [ ] Have the replacement drive(s) physically on hand, same size or larger.
- [ ] Snapshot the current state for the record:
      ```
      zpool status -v data01 | tee ~/data01-status-before.txt
      zpool list -v data01
      zpool events data01 | tail -50
      ```
- [ ] Map every `wwn-*` to a physical drive (serial + bay) so you pull the right
      one. Do NOT guess:
      ```
      # wwn -> /dev/sdX
      ls -l /dev/disk/by-id/ | grep wwn-0x5000c500b30c1db0
      # /dev/sdX -> serial + model (write the serial on paper)
      smartctl -i /dev/sdX
      # make the bay LED blink if the HBA/enclosure supports it
      ledctl locate=/dev/sdX     # ledctl locate_off=/dev/sdX to stop
      ```

## Step 1 — Decide clear vs replace

These faults are "too many errors" (persistent), not a transient blip, so this is
a **replace**, not a `zpool clear`. Only use `zpool clear data01 <wwn>` if you
have positively established the errors were transient (e.g. a reseated cable) —
which is not the case here.

## Step 2 — Replace the first drive

Do the **most-failed** drive first (`wwn-0x5000c500b2281882`, which has read +
write + checksum errors), so the worst offender stops dragging on the pool.

If the enclosure has a **free bay** (preferred — keeps the old drive readable):
```
# insert the new drive into the spare bay, find its id
ls -l /dev/disk/by-id/ | grep wwn        # identify the NEW drive's wwn/dev
zpool replace data01 wwn-0x5000c500b2281882 /dev/disk/by-id/wwn-<NEW>
```

If you must **swap in place** (no free bay):
```
zpool offline data01 wwn-0x5000c500b2281882   # take it offline first
# physically pull the drive you positively identified in step 0, insert the new one
zpool replace data01 wwn-0x5000c500b2281882 /dev/disk/by-id/wwn-<NEW>
```

Then **watch the resilver to completion**:
```
watch -n 30 'zpool status -v data01'
```
- Resilver time on a ~9TB drive at ~80% full can be many hours. Let it finish.
- If a **read error** appears on another drive during resilver, or the marginal
  third drive (`...b38b622b`) faults, STOP — you are now in a possible-data-loss
  window. Do not start a second replacement. Reassess (this is where the step-0
  backup saves you).

## Step 3 — Verify, then the second drive

- [ ] `zpool status -v data01` shows the replaced drive ONLINE, resilver complete,
      `errors: No known data errors`, and the vdev back to one-fault (or better).
- [ ] Only now repeat Step 2 for the second faulted drive
      (`wwn-0x5000c500b30c1db0`), full resilver, verify again.
- [ ] Watch `wwn-0x5000c500b38b622b` (the 14-read-error drive). If its error count
      keeps climbing, plan to replace it too — but one at a time, redundancy restored
      between each.

## Step 4 — Close out

- [ ] `zpool status -v data01` → `state: ONLINE`, no faulted devices, no errors.
- [ ] `zpool clear data01` to reset the (now-stale) error counters, only after the
      pool is healthy.
- [ ] Record the new drive serials/wwns; update any bay/serial inventory.
- [ ] Optional: `zpool scrub data01` to validate the whole pool end to end (this
      also stresses drives — do it when you can watch it, not before a trip).
- [ ] Save `zpool status -v data01 | tee ~/data01-status-after.txt`.

## Backout

There is no "undo" for a physical swap, but the safe fallbacks are:
- If a `zpool replace` was started and the new drive is bad, `zpool detach` the
  new drive and reinsert/`zpool online` the original if it is still readable.
- If the pool degrades further mid-procedure, STOP all drive operations, keep the
  pool imported read-mostly, and restore from the step-0 backup rather than
  forcing a resilver on a zero-parity pool.
- Never `zpool export -f` / `import -f` to "fix" a stuck state without
  understanding why — that is how a recoverable pool becomes an unrecoverable one.

## What NOT to do (ever, on this pool from automation)

`zpool create/destroy/add/remove/replace/attach/detach`, `zfs
create/destroy/set/mount/rollback`, or any `.mount` unit that targets `/data01`
belong to THIS runbook and the console, never to a committed playbook. CI enforces
that (`check-no-zfs-mutations.sh`); do not work around it.
