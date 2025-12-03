# ZFS Pool Disk Replacement Guide

Procedures for replacing failed or failing disks in ZFS pools. Applies to both HDD and NVMe drives.

**Pool Reference:**

- **data01**: 64TB RAIDZ2 pool on ocean VM (8x 12TB HDD via SAS passthrough)

---

## Pre-Flight Checks

### Assess Current State

```bash
# Full disk layout
lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT,MODEL,SERIAL

# Disk identifiers (use these for ZFS operations, not /dev/sdX)
ls -l /dev/disk/by-id/ | grep -v part

# Current pool status
zpool status
zpool status -v  # Verbose with error details

# Pool health history
zpool history data01 | tail -20

# Check for pending errors
zpool status -x  # Shows only pools with problems
```

### Identify Problem Disk

```bash
# Check SMART data for all disks
for disk in /dev/sd?; do
  echo "=== $disk ==="
  smartctl -H $disk 2>/dev/null | grep -E "SMART|result"
done

# Detailed SMART for specific disk
smartctl -a /dev/sdX

# Key SMART attributes to check
smartctl -A /dev/sdX | grep -E "Reallocated|Current_Pending|Offline_Uncorrectable|UDMA_CRC"

# Check dmesg for disk errors
dmesg | grep -iE "sd[a-z]|error|fail|i/o" | tail -50

# ZFS I/O error counts
zpool status data01 | grep -A 20 "NAME.*STATE"
```

---

## Planned Disk Replacement

Use this procedure when proactively replacing a disk (upgrade, preventive replacement).

### 1. Identify Disk to Replace

```bash
# Get the disk ID from zpool status
zpool status data01

# Example output shows disk by ID:
#   scsi-SATA_WDC_WD120EFBX_XXXXXXXX  ONLINE  0  0  0

# Verify physical disk mapping
ls -l /dev/disk/by-id/scsi-SATA_WDC_WD120EFBX_XXXXXXXX
```

### 2. Offline the Disk

```bash
# Gracefully offline the disk (pool continues in degraded state)
zpool offline data01 scsi-SATA_WDC_WD120EFBX_XXXXXXXX

# Verify status
zpool status data01
# Should show: DEGRADED, with disk marked OFFLINE
```

### 3. Physically Replace the Disk

```bash
# Power down disk (if hot-swap not supported)
# Or simply pull the disk from the bay

# Insert new disk

# Force SCSI rescan (for SAS/SATA)
echo "- - -" | sudo tee /sys/class/scsi_host/host*/scan

# For NVMe drives
echo 1 | sudo tee /sys/class/nvme/nvme*/rescan

# Verify new disk detected
lsblk
ls -l /dev/disk/by-id/ | grep -v part
```

### 4. Replace in ZFS Pool

```bash
# Get new disk ID
NEW_DISK=$(ls -l /dev/disk/by-id/ | grep -v part | grep "NEW_SERIAL" | awk '{print $9}')

# Replace old disk with new disk
zpool replace data01 scsi-SATA_WDC_WD120EFBX_OLD_SERIAL /dev/disk/by-id/scsi-SATA_WDC_WD120EFBX_NEW_SERIAL

# Monitor resilvering progress
watch -n 5 'zpool status data01 | grep -A 5 "scan:"'

# Or use zpool iostat for I/O during resilver
zpool iostat data01 5
```

### 5. Monitor Resilver Progress

```bash
# Resilver status
zpool status data01

# Example output during resilver:
#   scan: resilver in progress since Sat Nov 29 12:00:00 2025
#         2.50T scanned at 150M/s, 1.20T resilvered at 120M/s, 45.00% done
#         0 days 03:24:15 to go

# Estimated completion
zpool status data01 | grep -E "scan:|done|to go"
```

---

## Emergency Disk Replacement (Failed Disk)

Use when a disk has already failed and pool is degraded.

### 1. Assess Damage

```bash
# Check pool state
zpool status -v data01

# Look for:
#   state: DEGRADED
#   status: One or more devices has been removed by the administrator.
#   action: Online the device using 'zpool online' or replace the device

# Check for data errors
zpool status data01 | grep -E "CKSUM|READ|WRITE"
```

### 2. If Disk is Still Visible but Failing

```bash
# Force offline
zpool offline -f data01 scsi-SATA_WDC_WD120EFBX_FAILING_SERIAL

# Then proceed with replacement steps above
```

### 3. If Disk Already Gone (Removed/Failed)

```bash
# Pool will show UNAVAIL or REMOVED for the disk
# Proceed directly to physical replacement and rescan

# After inserting new disk and rescanning:
zpool replace data01 scsi-SATA_WDC_WD120EFBX_OLD_SERIAL /dev/disk/by-id/scsi-SATA_WDC_WD120EFBX_NEW_SERIAL
```

---

## Post-Replacement Verification

```bash
# Wait for resilver to complete
zpool status data01 | grep "scan:"
# Should show: scan: resilvered ... with 0 errors

# Verify pool health
zpool status data01
# Should show: state: ONLINE

# Clear any historical errors
zpool clear data01

# Run a scrub to verify data integrity
zpool scrub data01

# Monitor scrub progress
zpool status data01 | grep "scan:"
```

---

## Troubleshooting

### Disk Not Detected After Insert

```bash
# Rescan all SCSI hosts
for host in /sys/class/scsi_host/host*; do
  echo "- - -" > $host/scan
done

# Check dmesg for detection
dmesg | tail -20

# If SAS controller, may need:
echo 1 > /sys/class/scsi_device/*/device/rescan
```

### Resilver Stuck or Slow

```bash
# Check I/O wait
iostat -x 5

# Reduce other I/O during resilver
# ZFS prioritizes resilver, but heavy workloads can slow it

# Check for disk errors during resilver
dmesg | tail -50
```

### Replace Command Fails

```bash
# If old disk ID not recognized, use GUID
zpool status data01  # Note the GUID from the status output

# Replace using GUID
zpool replace data01 GUID /dev/disk/by-id/NEW_DISK_ID

# Or detach and attach approach
zpool detach data01 OLD_DISK_ID  # Only for mirrors!
zpool attach data01 EXISTING_DISK /dev/disk/by-id/NEW_DISK_ID
```

---

## Useful Commands Reference

| Command | Purpose |
|---------|---------|
| `zpool status` | Pool health and disk status |
| `zpool status -v` | Verbose with error files |
| `zpool status -x` | Only show problem pools |
| `zpool iostat 5` | I/O statistics every 5 seconds |
| `zpool history` | Pool operation history |
| `zpool clear` | Clear error counters |
| `zpool scrub` | Verify data integrity |
| `zpool online` | Bring disk online |
| `zpool offline` | Take disk offline |
| `zpool replace` | Replace disk |
| `zpool resilver` | Restart resilver (rare) |
| `smartctl -a` | Disk SMART data |
| `lsblk` | Block device layout |

---

## Safety Notes

- **RAIDZ2 can survive 2 disk failures** - don't panic on single disk failure
- **Always use `/dev/disk/by-id/`** - device names (/dev/sdX) can change on reboot
- **Resilver time depends on pool size and usage** - 12TB disk may take 12-24 hours
- **Don't replace multiple disks simultaneously** - wait for resilver to complete
- **Keep spare disks** - have replacement drives ready for emergencies
