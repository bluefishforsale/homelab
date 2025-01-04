# get current layout before doing any operations
lsblk
ls -l /dev/disk/by-id
zpool status

# pull a specific disk
## if the slot is unknown, pull any disk
### get the ID of the pulled disk
zpool status

##  offline the disk
zpool offline <pool> <disk-id>

# insert replacement disk
# force a rescan
echo "- - -" | sudo tee /sys/class/scsi_host/host*/scan
echo 1 | sudo tee /sys/class/nvme/nvme*/rescan

# zfs replace the disk
zpool replace <pool> <old-disk-id> <new-disk-id>

# wait for pool health to be GOOD
zpool status