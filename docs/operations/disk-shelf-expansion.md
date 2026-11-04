# Disk shelf expansion for data01

This exists because data01 is nearly full and the R720 is out of drive bays. It is the
plan and the shopping list for adding external SAS shelves to node006 and growing the
pool from them, written before any money is spent so the purchase and the
`zpool add` happen once.

## Goal

Add 12 to 24 more 3.5" bays, run them as RAIDZ2, and grow `data01` on ocean with them.
That means fast enough for scrubs and resilvers, cheap on the used market, and not
loud or hot enough to be a problem in the rack. Rack space (U15 and up is open) and
power budget are already covered.

## What exists today

Everything below was read live on 2026-11-04.

`data01` is a single 8-wide RAIDZ2 of 12 TB Exos X14 SATA drives (`ST12000NM0127`).
`zpool list` reports `87.2T SIZE, 76.1T ALLOC, 11.2T FREE`, capacity `87%`, fragmentation
`20%`. The last scrub took `1 days 06:59:04` with 0 errors, and the pool is `ONLINE`
with no errors on any member. Past 80% full, ZFS allocation slows and fragmentation
climbs faster, so this is already the tight end.

ocean runs `zfs-2.1.11-1+deb12u1` on Debian 12. That matters: RAIDZ expansion (growing
an existing raidz vdev one disk at a time) landed in OpenZFS 2.3, so on this version the
only way to grow `data01` is to add a new top-level vdev.

The disks reach ocean through PCI passthrough on node006 (Proxmox `pve-manager/8.4.0`,
kernel `6.8.12-9-pve`). The internal HBA is a SAS2308 at `02:00.0` (`[1000:0087]`), link
`8GT/s x8`, alone in IOMMU group 40. It is bound to vfio-pci by device ID in
`/etc/modprobe.d` (`options vfio-pci ids=10de:2204,10de:1aef,1000:0087`, see
[proxmox.md](proxmox.md)) and attached to VM 5000 as `hostpci1=02:00,pcie=1`. The RTX
3090 is `hostpci0` in IOMMU group 14.

node006 slot usage from `dmidecode -t slot`: PCI4 (x16) holds the 3090, PCI7 holds an
Intel 660P NVMe (`05:00.0`), and PCI1, PCI2, PCI3, PCI5 and PCI6 (all x8 electrical,
PCIe 3) are free. The internal SAS2308 sits in the integrated storage slot, not one of
the seven. The 3090 is a thick card, so check that PCI5 is physically clear before
counting it.

## The shelves have to hang off node006

The first idea was one shelf per metal. That doesn't work if the disks are going into
`data01`: ocean is a VM on node006, and passthrough only hands a guest devices from its
own host. A shelf cabled to node005 can never be a member of `data01`. It could only be
a separate pool on node005 (a decent `zfs send` replica target, and a real backup for a
pool that recently ran with zero parity left), but that's a different project with a
different goal. This plan puts both shelves on node006.

## Hardware

### Shelf

| Shelf | Bays | Link | Noise and heat | Used price (unverified) | Notes |
|---|---|---|---|---|---|
| Dell MD1200 | 12 LFF, 2U | SAS2 6 Gb, SFF-8088 | Loud for the first minute at power-on, then settles to moderate | $150-250 | Recommended. Dell rails, SATA and SAS both fine, cheap caddies |
| Supermicro CSE-846 JBOD, SQ PSUs, BPN-SAS3-846EL1 | 24 LFF, 4U | SAS3 12 Gb, SFF-8644 | Quietest option, more so with a fan swap | $400-700 | Pick this if quiet matters more than cost. Needs a CSE-PTJBOD-CB2 power board |
| NetApp DS212C | 12 LFF, 2U | SAS3 12 Gb, SFF-8644 | Moderate | $200-350 | Fine, but SAS3 buys nothing for 12 spinners |
| NetApp DS4246 | 24 LFF, 4U | SAS2 6 Gb, QSFP | Moderate to loud, about 150 W empty with 4 PSUs | $150-300, often sold without caddies | Cheap per bay, but heavy and hot, and needs QSFP cables |
| NetApp DS4243 | 24 LFF | 3 Gb | Loud | Cheap | Avoid. 3 Gb link, and SATA needs interposers |
| EMC KTN-STL3 | 15 LFF | 6 Gb | Very loud | Cheap | Avoid |

The pick is two MD1200s. A 6 Gb x4 wide port carries about 2.2 GB/s, and 10 to 12 HDDs at
about 250 MB/s each top out near 2.5 to 3 GB/s, but only on outer tracks and only during
a scrub or resilver. Clients get at most 1.1 GB/s over 10GbE anyway, so SAS2 isn't the
bottleneck that matters. Two 12-bay shelves on two separate HBA ports also give each
shelf its own link, which beats one 24-bay SAS2 shelf squeezed through a single 2.2 GB/s
port. If the budget stretches and the noise in the room matters, the Supermicro 846 is
the better box. Run it with a 9300-8e and everything else in this doc still applies.

Set the MD1200 to unified mode, not split mode, and cable to the EMM's In port. One EMM
per shelf is enough, since a second EMM only adds a path for dual-ported SAS drives and
does nothing for SATA.

### HBA

One LSI 9207-8e (SAS2308, IT mode, firmware P20.00.07.00), about $30-50 used, into a free
x8 slot on node006. It has two external SFF-8088 ports, so each shelf gets its own port
and nothing is daisy-chained. PCIe 3 x8 is about 7.8 GB/s, well above two shelves' 4.4.

Matching the internal chip is the main reason to choose it. The 9207-8e reports the same
`1000:0087` ID, so the existing vfio-pci binding claims it at boot with no modprobe
change. A 9300-8e is `1000:0097`, so choosing the Supermicro path means adding that ID to
the `vfio-pci ids=` line and rebuilding the initramfs.

A second HBA (one per shelf) doesn't buy availability: with vdevs confined to a shelf,
losing either HBA still takes out a whole vdev and suspends the pool. Buy a spare
9207-8e for the drawer instead.

### Cables, caddies, power

For each MD1200: one SFF-8088 to SFF-8088 cable (1 m, about $15), twelve Dell F238F /
KG1CH caddies (about $5-8 each), and two C13 power cords, one per PSU, on separate UPS
outlets if possible. Expect about 130-150 W at idle per shelf with 12 drives (about 500
BTU/hr) and about 250 W during spin-up.

### Drives

Recertified Exos, SAS rather than SATA when the price is close. The SAS phy metrics
(`files/node-exporter/sas-phy-metrics.sh`) exist because of a correlated-checksum
incident that pointed at the physical layer, and SATA behind an expander (tunnelled over
STP) is the weaker path there. SAS drives also report richer error counters. The new
vdev doesn't have to match the existing 12 TB drives, so buy whatever size has the best
$/TB on the day, 16 to 20 TB being the likely range. Prices in 2026 move enough that this
doc doesn't quote them.

## Pool layout

ZFS stripes data across all top-level vdevs and needs every one of them present, so the
enclosure becomes a failure domain. If a RAIDZ2 vdev has more than two disks in one
shelf, losing that shelf (both PSUs, the EMM, the cable, the HBA port) faults the vdev
and suspends the whole pool. RAIDZ2 can't be spread at two-or-fewer disks per enclosure
without four or more enclosures, so the rule here is the opposite: each new vdev lives
entirely inside one shelf. A shelf outage then suspends the pool (an outage, with
`failmode=wait` holding I/O) rather than corrupting it, since the disks come back intact
when power or cabling returns and `zpool clear` resumes. Dual PSUs per shelf are what
make that rare.

Per 12-bay shelf, the recommended layout is one 10-wide RAIDZ2 vdev with 2 bays left
empty. The empty bays are deliberate: the disk replacement runbook
([zfs-data01-disk-replacement-runbook.md](zfs-data01-disk-replacement-runbook.md))
prefers replacing into a free bay so the outgoing disk stays readable through the
resilver, and the R720's internal bays have none. 10-wide RAIDZ2 is 80% efficient, a
little better than the existing 8-wide's 75%, without the resilver time of 12-wide.
Mixed vdev widths in one pool are fine.

Rough usable space per shelf, before ZFS overhead:

| Drive | 10-wide RAIDZ2, data disks x size |
|---|---|
| 16 TB | 8 x 16 = 128 TB (about 116 TiB) |
| 20 TB | 8 x 20 = 160 TB (about 145 TiB) |

Existing data stays where it is, because ZFS doesn't rebalance. New writes favour the
emptier vdev, so for a while most I/O lands on the new shelf. That's expected and evens
out over time.

### Why not RAIDZ expansion

Upgrading ocean to OpenZFS 2.3 (bookworm-backports or Debian 13) and widening `raidz2-0`
with shelf disks is possible, and it's the wrong move here. A widened vdev would span the
internal backplane and a shelf, so losing the shelf would take more than two members of
one vdev. Expansion also keeps old data at the old parity ratio until rewritten, runs
for days on a vdev this full, and caps every new disk at the 12 TB of the smallest
member. Upgrading ZFS on the pool's host just to do it adds risk to the one thing that
must not break.

## Procedure

Per [agents.md](../../agents.md), nothing here is a playbook. ZFS changes are hand-run and
snapshot-first, and the VM's PCI config is set by hand with `qm set`. Every step before
`zpool add` can be undone; `zpool add` can't.

1. Burn in the drives first, somewhere other than the production pool if possible: a
   long SMART test and one full write pass on each disk. Recertified drives that are
   going to die early usually do it here.
2. Schedule a window. Installing the HBA means powering off node006, which takes ocean
   and every service on it down.
3. Shut down ocean, then node006. Install the 9207-8e in a free x8 slot (PCI1-3 or PCI6,
   PCI5 only if the 3090 clears it). Rack and cable the shelves, power them on and give
   the fans a minute to settle before node006 boots, so the HBA sees the disks at
   enumeration.
4. On node006, confirm the new card is claimed by vfio-pci and isolated:
   `lspci -nnk | grep -A3 1000:0087` should show `Kernel driver in use: vfio-pci` for both
   controllers, and `find /sys/kernel/iommu_groups -name '0000:<bus>:00.0'` should land
   in a group with nothing else in it.
5. Attach it to ocean: `qm set 5000 --hostpci2=<bus>:00,pcie=1`. Record the line in
   [proxmox.md](proxmox.md) next to `hostpci0` and `hostpci1`.
6. Boot ocean. Check that the new disks show up (`lsscsi`, `ls -l /dev/disk/by-id/wwn-*`),
   check `smartctl -a` on each, and map every WWN to a shelf and bay before going further.
7. Snapshot whatever is worth snapshotting, then do a dry run that prints the resulting
   layout without changing anything, using WWN paths only:
   `zpool add -n data01 raidz2 /dev/disk/by-id/wwn-... (x10)`.
   Read the output. If it shows the new vdev as anything other than `raidz2`, stop.
8. Run the same command without `-n`. Then `zpool status data01` should show `raidz2-0`
   and `raidz2-1`, both `ONLINE`.
9. Repeat 7-8 for the second shelf, once the first has run cleanly for a while.

## Monitoring

A shelf that's not monitored doesn't count as done. Confirm the SMART exporter and the
SAS phy metrics pick up the new disks and the new phys (`scripts/prom.sh`), and that
`disk_wwn_info` maps the new WWNs. The MD1200 reports PSU, fan and temperature state over
SES, which `sg_ses` can read from ocean since ocean owns the HBA. Wiring that into the
node-exporter textfile collector is follow-up work, not a blocker for the purchase.

## Backout

Up to step 7, backing out is mechanical: `qm set 5000 --delete hostpci2`, power down,
pull the card, and nothing on the pool has changed. After `zpool add` there's no backout
for the vdev itself. On 2.1.11 a raidz top-level vdev can't be removed, and the only way
to undo it is to copy everything to a new pool. That's why the dry run, the WWN mapping
and the burn-in come first. If there's any doubt about a shelf, run it as a separate
pool for a few weeks before committing it to `data01`.

A failed shelf after the add is an outage, not a data-loss event, as long as each vdev
stays inside one enclosure: fix the shelf or move its disks to a spare one, power up,
`zpool clear data01`.
