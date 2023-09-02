# Do all tis beofre loading the web ui
    - it will ask to install ceph
    - best to have the right apt repos in place first
## non-subscriptions repos
https://pve.proxmox.com/wiki/Package_Repositories
/etc/apt/sources.list
    deb http://ftp.debian.org/debian bookworm main contrib
    deb http://ftp.debian.org/debian bookworm-updates main contrib

    # Proxmox VE pve-no-subscription repository provided by proxmox.com,
    # NOT recommended for production use
    deb http://download.proxmox.com/debian/pve bookworm pve-no-subscription

    # security updates
    deb http://security.debian.org/debian-security bookworm-security main contrib

File /etc/apt/sources.list.d/pve-enterprise.list
comment out this line
    deb https://enterprise.proxmox.com/debian/pve bookworm pve-enterprise

File /etc/apt/sources.list.d/ceph.list
    deb https://enterprise.proxmox.com/debian/ceph-quincy bookworm enterprise

## create the OSDs
https://pve.proxmox.com/wiki/Deploy_Hyper-Converged_Ceph_Cluster

ceph auth get client.bootstrap-osd > /etc/pve/priv/ceph.client.bootstrap-osd.keyring
id=0
for disk in a b c d e f g h ; do
    # pveceph osd create /dev/sd${disk} -db_dev /dev/nvme0n1 -db_dev_size 110
    ceph-volume lvm create --bluestore --osd-id $id --data /dev/sd${disk} --block.db /dev/nvme0n1 --block.db-size 110
    let "id=id+1"
done
## data pool and pve storage
pveceph pool create data -application rbd
pvesm add rbd ceph-lvm -pool data
## cephfs
pveceph mds create
pveceph fs create --pg_num 32 --add-storage


# destroy all ceph and reset disks
pveceph mds destroy node006
pveceph fs destroy cephfs
for osd in `seq 0 7` ; do for step in stop down out purge destroy ; do ceph osd $step osd.$osd --force  ; done ; done
vgdisplay | grep 'VG Name' | grep ceph | awk '{print $3}'  | xargs vgremove -y
for disk in a b c d e f g h ; do wipefs -a /dev/sd${disk} ; done
wipefs -a /dev/nvme0n1
pveceph mgr destroy node006
pveceph mon destroy node006
pveceph stop
pveceph purge
rm /etc/pve/ceph.conf
find /var/lib/ceph/ -mindepth 2 -delete



# Make VM temaplate
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2
wget -O  rsa.key https://github.com/bluefishforsale.keys

qm create 9999 --name debian-12-generic-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 9999 	debian-12-generic-amd64.qcow2 ceph-lvm
qm set 9999 --ide2 ceph-lvm:cloudinit
qm set 9999 --scsihw virtio-scsi-pci --scsi0 ceph-lvm:vm-9999-disk-0
qm set 9999 --boot order='scsi0'
qm set 9999 --serial0 socket --vga serial0
qm set 9999 --sshkeys rsa.key
qm set 9999 --hotplug network,disk
qm set 9999 --bios ovmf
qm set 9999 --machine q35
qm set 9999 --efidisk0 ceph-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=1M
qm set 9999 --cores 2
qm set 9999 --memory 4096
qm set 9999 --agent enabled=1
qm template 9999

# make six VMs from the template, using cloud-init to set IP and onboot info
wget -O  rsa.key https://github.com/bluefishforsale.keys
for x in 0 1 ; do
for y in 1 2 3 ; do

qm clone 9999 6${x}${y}
qm set 6${x}${y} --name kube6${x}${y} --ipconfig0 ip=$(host kube6${x}${y}.home | awk '{print $NF}')/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 6${x}${y} --sshkeys rsa.key
# qm set 6${x}${y} --efidisk0 ceph-lvm:6${x}${y}-disk,efitype=4m
qm resize 6${x}${y} scsi0 +18G  # 21.47G
if [ $x = 0 ] ; then
    qm set 6${x}${y} --cores 4
    qm set 6${x}${y} --memory 8192  # 8G
else
    qm set 6${x}${y} --cores 9
    qm set 6${x}${y} --memory 57344  # 56G
    if [ $y = 3 ] ; then
        # this sets up this VM specificallt for nvidia pci-e GPU passthrough at PCI-e address 42:00.*
        qm set 6${x}${y} --hostpci0=42:00,pcie=1
    fi
fi
done
done

# start VMS
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm start

# stop and destroy
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done |\
 while read id ; do qm stop $id ; qm destroy $id ; done



