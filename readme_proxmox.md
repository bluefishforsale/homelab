# make cloud-init VM template
wget https://cloud.debian.org/images/cloud/OpenStack/current-10/debian-10-openstack-amd64.qcow2
qm create 9999 --name debian-10-openstack-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 9999 debian-10-openstack-amd64.qcow2 local-lvm
qm set 9999 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-9999-disk-0
qm set 9999 --ide2 local-lvm:cloudinit
qm set 9999 --boot c --bootdisk scsi0
qm set 9999 --serial0 socket --vga serial0
qm template 9999


qm clone 9999 666 --name test-vm
< fix cloud-init settings, disk, etc in UI >
< start vm >
< console shell >

sudo apt update
sudo apt full-upgrade
sudo apt install qemu-guest-agent


for x in 0 1 ; do
for y in 1 2 3 ; do
qm clone 666 6${x}${y} --name kube6${x}${y}
done
done