# get ovmf packages
sudo apt-get install ovmf
ls -l /usr/share/ovmf/OVMF.fd
ls -l /usr/share/qemu/OVMF.fd


# using ceph LVM for VM disk image
wget https://cloud.debian.org/images/cloud/OpenStack/current-10/debian-10-openstack-amd64.qcow2
curl https://github.com/bluefishforsale.keys > rsa.key
qm create 9999 --name debian-10-openstack-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 9999 debian-10-openstack-amd64.qcow2 ceph-lvm
qm set 9999 --ide2 local-lvm:cloudinit
qm set 9999 --scsihw virtio-scsi-pci --scsi0 ceph-lvm:vm-9999-disk-0
qm set 9999 --boot order='scsi0'
qm set 9999 --serial0 socket --vga serial0
qm set 9999 --sshkeys rsa.key
qm template 9999


# make single test VM from the template, using efi and passthrough the GPU
curl https://github.com/bluefishforsale.keys > rsa.key
for x in 1 ; do
for y in 4 ; do
qm clone 9999 6${x}${y}
qm set 6${x}${y} --name kube6${x}${y} --ipconfig0 ip=$(host kube6${x}${y}.home | awk '{print $NF}')/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 6${x}${y} --sshkeys rsa.key
qm resize 6${x}${y} scsi0 +18G  # 21.47G
if [ $x = 0 ] ; then
    qm set 6${x}${y} --cores 4
    qm set 6${x}${y} --memory 8192  # 8G
else
    qm set 6${x}${y} --cores 9
    qm set 6${x}${y} --memory 57344  # 56G
    if [ $y = 4 ] ; then
        qm set 6${x}${y} --efidisk0 local-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=128K
        qm set 6${x}${y} --hotplug network,disk,usb
        qm set 6${x}${y} --bios ovmf
        qm set 6${x}${y} --machine q35
        qm set 6${x}${y} --hostpci0=42:00,pcie=on,rombar=off,x-vga=off
    fi
fi
done
done
for x in 1 ; do for y in 4 ; do  echo "6$x$y" ; done ; done | xargs -n1 qm start

