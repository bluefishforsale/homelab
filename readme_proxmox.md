# stop and destroy
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm stop
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm destroy

# using ceph LVM for VM disk image
wget https://cloud.debian.org/images/cloud/OpenStack/current-10/debian-10-openstack-amd64.qcow2
curl https://github.com/bluefishforsale.keys > rsa.key
qm create 8888 --name debian-10-openstack-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 8888 debian-10-openstack-amd64.qcow2 ceph-lvm
qm set 8888 --bios seabios
qm set 8888 --machine i440fx
qm set 8888 --ide2 local-lvm:cloudinit
qm set 8888 --scsihw virtio-scsi-pci --scsi0 ceph-lvm:vm-8888-disk-0
qm set 8888 --boot order='scsi0'
qm set 8888 --serial0 socket --vga serial0
qm set 8888 --sshkeys rsa.key
# qm set 8888 --efidisk0 ceph-lvm:0
qm template 8888

# make six VMs from the template, using cloud-init to set IP and onboot info
curl https://github.com/bluefishforsale.keys > rsa.key
for x in 0 1 ; do
for y in 1 2 3 ; do
qm clone 8888 6${x}${y}
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
    if [ $y = 4 ] ; then
        # this sets up this VM specificallt for nvidia pci-e GPU passthrough at PCI-e address 42:00.*
        qm set 6${x}${y} --machine q35
        qm set 6${x}${y} --hostpci0=42:00,pcie=1
    fi
fi
done
done
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm start
