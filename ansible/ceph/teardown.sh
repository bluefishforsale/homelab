#!/usr/bin/env bash
DISK="/dev/${1}"

# Zap the disk to a fresh, usable state (zap-all is important, b/c MBR has to be clean)

# You will have to run this step for all disks.
sgdisk --zap-all $DISK

# Clean hdds with dd
dd if=/dev/zero of="$DISK" bs=1M count=100 oflag=direct,dsync

# Clean disks such as ssd with blkdiscard instead of dd
blkdiscard $DISK

CEPH_ID="$(lsblk ${DISK} | grep ceph | awk '{print $1}'  | sed -e 's/└─//g')"
# These steps only have to be run once on each node
# If rook sets up osds using ceph-volume, teardown leaves some devices mapped that lock the disks.
find /dev/mapper -type b -name "${CEPH_ID}" -exec dmsetup remove {} +
find /dev/ -type b -name "${CEPH_ID}" -delete

# ceph-volume setup can leave ceph-<UUID> directories in /dev and /dev/mapper (unnecessary clutter)
rm -rf /dev/${CEPH_ID}
rm -rf /dev/mapper/${CEPH_ID}

# Inform the OS of partition table changes
partprobe $DISK