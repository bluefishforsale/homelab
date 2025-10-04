# 💾 ZFS Operations Guide

## Overview

This guide covers ZFS storage management, maintenance, and troubleshooting for your homelab environment.

## 📋 Daily Operations

### Pool Status Monitoring
```bash
# Check pool health
zpool status
zpool status -v  # Verbose output with error details

# Pool I/O statistics
zpool iostat
zpool iostat -v 5  # Live stats every 5 seconds

# Pool history
zpool history
zpool events    # Recent events and errors
```

### Dataset Management
```bash
# List all datasets
zfs list
zfs list -t all  # Include snapshots

# Dataset properties
zfs get all tank/dataset
zfs get compression,dedup tank/dataset

# Create dataset
zfs create tank/new-dataset
zfs create -o compression=lz4 tank/compressed-dataset

# Set dataset properties
zfs set compression=lz4 tank/dataset
zfs set quota=100G tank/dataset
zfs set recordsize=1M tank/large-files  # For large files/VMs
```

### Snapshot Operations
```bash
# Create snapshot
zfs snapshot tank/dataset@$(date +%Y%m%d-%H%M)
zfs snapshot -r tank/dataset@backup-snapshot  # Recursive

# List snapshots
zfs list -t snapshot
zfs list -t snapshot tank/dataset

# Rollback to snapshot
zfs rollback tank/dataset@backup-snapshot

# Clone from snapshot
zfs clone tank/dataset@snapshot tank/clone

# Destroy snapshot
zfs destroy tank/dataset@old-snapshot
zfs destroy -r tank/dataset@recursive-snapshot  # Recursive
```

## 🔧 Pool Management

### Pool Creation
```bash
# Create basic pool
zpool create tank /dev/sdb

# Create mirrored pool  
zpool create tank mirror /dev/sdb /dev/sdc

# Create RAIDZ pool
zpool create tank raidz /dev/sdb /dev/sdc /dev/sdd
zpool create tank raidz2 /dev/sd{b,c,d,e,f}  # RAIDZ2 (dual parity)

# Create pool with cache/log devices
zpool create tank raidz /dev/sd{b,c,d} \
  cache /dev/nvme0n1p1 \
  log /dev/nvme0n1p2
```

### Pool Expansion
```bash
# Add vdev to existing pool
zpool add tank mirror /dev/sde /dev/sdf

# Add cache device
zpool add tank cache /dev/nvme1n1

# Add log device  
zpool add tank log /dev/nvme1n1p1

# Replace disk (resilver)
zpool replace tank /dev/sdb /dev/sdg
```

### Pool Properties
```bash
# Set pool properties
zpool set autoexpand=on tank
zpool set autoreplace=on tank
zpool set failmode=continue tank

# Pool features
zpool get all tank
zpool upgrade tank  # Upgrade pool features
```

## 🚀 Performance Optimization

### Compression Settings
```bash
# Enable compression (saves space and can improve performance)
zfs set compression=lz4 tank
zfs set compression=gzip-6 tank/archive  # Higher compression for archives

# Check compression ratio
zfs get compressratio tank
zfs list -o name,used,available,compressratio
```

### ARC (Adaptive Replacement Cache) Tuning
```bash
# Check ARC statistics
arcstat
arc_summary

# Tune ARC size (add to /etc/modprobe.d/zfs.conf)
echo 'options zfs zfs_arc_max=17179869184' >> /etc/modprobe.d/zfs.conf  # 16GB max
echo 'options zfs zfs_arc_min=4294967296' >> /etc/modprobe.d/zfs.conf   # 4GB min

# Apply immediately (or reboot)
echo 17179869184 > /sys/module/zfs/parameters/zfs_arc_max
```

### Record Size Optimization
```bash
# Database workloads (small random I/O)
zfs set recordsize=8K tank/database

# Virtual machines (default is good)
zfs set recordsize=64K tank/vms  # Default, but explicit

# Large sequential files (media, backups)
zfs set recordsize=1M tank/media
zfs set recordsize=1M tank/backups
```

### Deduplication (Use Carefully)
```bash
# Enable deduplication (requires lots of RAM)
zfs set dedup=on tank/dedup-dataset

# Check deduplication ratio
zpool list  # Shows DEDUP column
zdb -DD tank  # Detailed dedup statistics

# Disable deduplication for new data
zfs set dedup=off tank/dataset  # Existing deduped data remains
```

## 📊 Monitoring & Maintenance

### Health Monitoring
```bash
# Pool health check
zpool status -x  # Show only unhealthy pools

# Scrub operations (data integrity check)
zpool scrub tank
zpool status tank  # Check scrub progress

# Schedule automatic scrubs (cron)
echo "0 2 * * 0 root /sbin/zpool scrub tank" >> /etc/crontab
```

### Capacity Management
```bash
# Check space usage
df -h /tank  # Basic usage
zfs list -o space  # Detailed space breakdown

# Find large files/datasets
du -sh /tank/* | sort -h
zfs list -s used  # Sort datasets by usage

# Clean up snapshots
zfs list -t snapshot -s creation | tail -20  # Oldest snapshots
# Remove old snapshots (careful!)
zfs destroy tank/dataset@old-snapshot
```

### Performance Monitoring
```bash
# Real-time I/O stats
zpool iostat -v 2

# Historical performance
sar -d 1 60  # Disk I/O statistics

# ZFS-specific monitoring
zpool events -f  # Follow events in real-time
```

## 🔄 Backup & Replication

### Send/Receive Operations
```bash
# Send dataset to file
zfs send tank/dataset@snapshot > backup.zfs
zfs send -i tank/dataset@old-snap tank/dataset@new-snap > incremental.zfs

# Send to remote system
zfs send tank/dataset@snapshot | ssh remote-host zfs receive backup/dataset

# Resume interrupted send/receive
zfs send -t token  # Use token from failed receive
```

### Automated Backup Scripts
```bash
#!/bin/bash
# /usr/local/bin/zfs-backup.sh

DATASET="tank/important"
SNAPSHOT_NAME=$(date +%Y%m%d-%H%M)
REMOTE_HOST="backup-server"
REMOTE_DATASET="backup/$(hostname)"

# Create snapshot
zfs snapshot "${DATASET}@${SNAPSHOT_NAME}"

# Find previous snapshot for incremental
PREV_SNAP=$(zfs list -t snapshot -o name -s creation "${DATASET}" | tail -2 | head -1)

# Send incremental backup
if [ -n "$PREV_SNAP" ]; then
    zfs send -i "$PREV_SNAP" "${DATASET}@${SNAPSHOT_NAME}" | \
        ssh "$REMOTE_HOST" zfs receive -F "$REMOTE_DATASET"
else
    zfs send "${DATASET}@${SNAPSHOT_NAME}" | \
        ssh "$REMOTE_HOST" zfs receive "$REMOTE_DATASET"
fi

# Cleanup old local snapshots (keep last 7 days)
zfs list -t snapshot -o name -s creation "$DATASET" | grep "@" | head -n -7 | \
    xargs -r -n1 zfs destroy
```

## 🚨 Troubleshooting

### Common Issues

#### Pool Import/Export
```bash
# Export pool (for maintenance/moving)
zpool export tank

# Import pool
zpool import tank

# Force import (if previous host crashed)
zpool import -f tank

# Import with different name
zpool import tank tank-backup

# Scan for importable pools
zpool import -a  # Import all found pools
```

#### Disk Failures
```bash
# Check for disk errors
zpool status -x

# Replace failed disk
zpool replace tank /dev/old-disk /dev/new-disk

# Remove failed disk (if redundancy allows)
zpool remove tank /dev/failed-disk

# Clear errors after replacement
zpool clear tank
```

#### Performance Issues
```bash
# Check for fragmentation
zpool list  # Check FRAG column

# Defragment (only on newer ZFS versions)
zpool trim tank  # If pool supports TRIM

# Check for memory pressure
free -h
arcstat | grep "memory pressure"

# Adjust ARC size if needed
echo $((8*1024*1024*1024)) > /sys/module/zfs/parameters/zfs_arc_max
```

#### Snapshot Issues
```bash
# Clean up hold references
zfs holds tank/dataset@snapshot
zfs release hold-tag tank/dataset@snapshot

# Fix dataset busy errors
lsof +D /tank/dataset  # Find processes using dataset
fuser -v /tank/dataset  # Alternative method
```

### Recovery Procedures

#### Pool Recovery
```bash
# If pool won't import due to missing devices
zpool import -m tank  # Import with missing devices
zpool remove tank /dev/missing-disk

# Recover from backup labels
zdb -l /dev/disk  # Check ZFS labels
zpool import -D /dev/disk-directory
```

#### Data Recovery
```bash
# Recover from snapshot
zfs rollback tank/dataset@good-snapshot

# Clone for investigation
zfs clone tank/dataset@snapshot tank/recovery-clone

# Mount specific snapshot (read-only)
mount -t zfs tank/dataset@snapshot /mnt/recovery
```

## 🔧 Advanced Configuration

### ZFS with Virtualization
```bash
# Optimize for VM storage
zfs create -o recordsize=64K \
           -o compression=lz4 \
           -o sync=disabled \
           -o logbias=throughput \
           tank/vms

# VM disk image management
zfs create -V 50G tank/vms/vm-001  # Create 50GB zvol
zfs set refreservation=50G tank/vms/vm-001  # Guarantee space

# Snapshot before changes
zfs snapshot tank/vms/vm-001@before-update
# Clone for testing
zfs clone tank/vms/vm-001@before-update tank/vms/vm-001-test
```

### ZFS Encryption (ZFS on Linux 0.8+)
```bash
# Create encrypted dataset
zfs create -o encryption=on \
           -o keyformat=passphrase \
           -o keylocation=prompt \
           tank/encrypted

# Load encryption key
zfs load-key tank/encrypted

# Mount encrypted dataset
zfs mount tank/encrypted

# Change encryption key
zfs change-key tank/encrypted
```

### ZFS Delegation
```bash
# Allow user to manage specific dataset
zfs allow user1 mount,snapshot,send,receive tank/user1

# Remove permissions
zfs unallow user1 tank/user1

# Show current permissions
zfs allow tank/user1
```

## 📈 Capacity Planning

### Growth Monitoring
```bash
# Track pool growth over time
zfs get available,used,compressratio tank
zpool iostat -T d  # Timestamped I/O stats

# Estimate future capacity needs
zfs list -o name,used,available,refer,compressratio
```

### Expansion Planning
```bash
# Calculate expansion options
zdb -C tank | grep asize  # Current raw capacity

# RAIDZ expansion (requires ZFS 2.2+)
zpool attach tank raidz-device new-device

# Traditional expansion (add vdev)
zpool add tank raidz /dev/new-disk1 /dev/new-disk2 /dev/new-disk3
```

## 🔐 Security Best Practices

### Access Controls
```bash
# Set appropriate permissions
chmod 750 /tank/sensitive
chown user:group /tank/dataset

# Use ZFS datasets for isolation
zfs create tank/users/alice
zfs set quota=100G tank/users/alice
zfs allow alice mount,snapshot tank/users/alice
```

### Audit Trail
```bash
# Enable ZFS event logging
zpool events -f | logger -t zfs-events

# Monitor for unauthorized access
zfs get mounted,mountpoint tank/sensitive
```

## 📅 Maintenance Schedule

### Daily Tasks
- [ ] Check pool status (`zpool status`)
- [ ] Monitor capacity (`zfs list`)
- [ ] Review ZFS events (`zpool events`)

### Weekly Tasks  
- [ ] Run scrub (`zpool scrub`)
- [ ] Clean old snapshots
- [ ] Review performance metrics
- [ ] Check backup integrity

### Monthly Tasks
- [ ] Full capacity analysis
- [ ] Performance tuning review
- [ ] Security audit
- [ ] Disaster recovery testing

### Quarterly Tasks
- [ ] ZFS software updates
- [ ] Hardware health check
- [ ] Backup strategy review
- [ ] Documentation updates

This ZFS operations guide provides comprehensive coverage of storage management tasks while maintaining the idempotency and automation principles essential to your homelab environment.
