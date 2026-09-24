#!/usr/bin/env bash
# Helper script to extract device-to-wwn mappings for ZFS devices
# This script is meant to be run manually or as part of a maintenance routine
# to understand how devices are mapped in the system

echo "=== Device to WWN mapping information ==="
echo

# List all devices in /dev/disk/by-id/ with their targets
echo "Available devices in /dev/disk/by-id/:"
ls -la /dev/disk/by-id/ 2>/dev/null | grep -v "QEMU" | head -20
echo

# Show kernel device names and their associated WWNs via lsblk
echo "Device information (kernel names, WWNs, and serials):"
lsblk -o NAME,WWN,SERIAL 2>/dev/null | grep -v "NAME"
echo

# Show available ZFS pools and their devices
if command -v zpool >/dev/null 2>&1; then
  echo "ZFS pools and devices:"
  zpool status -v 2>/dev/null | grep -E "NAME|STATE|/dev/" | head -20
  echo
else
  echo "zpool command not available - skipping ZFS info"
  echo
fi

# Show how to manually create the mapping
echo "=== Manual mapping approach ==="
echo "To manually map a device like 'sdf' to WWN:"
echo "1. Find the symlink: find /dev/disk/by-id/ -lname '*sdf'"
echo "2. Extract WWN from symlink name"
echo "3. Get serial: lsblk -o SERIAL -n -r /dev/sdf"
echo