# stop and destroy
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done |\
 while read id ; do qm stop $id ; qm destroy $id ; done

# using ceph LVM for VM disk images
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

for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm start



