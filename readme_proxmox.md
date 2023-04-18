# make cloud-init VM template
curl https://github.com/bluefishforsale.keys > rsa.pub
wget https://cloud.debian.org/images/cloud/OpenStack/current-10/debian-10-openstack-amd64.qcow2
qm create 9999 --name debian-10-openstack-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 9999 debian-10-openstack-amd64.qcow2 local-lvm
qm set 9999 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-9999-disk-0
qm set 9999 --ide2 local-lvm:cloudinit
qm set 9999 --boot c --bootdisk scsi0
qm set 9999 --serial0 socket --vga serial0
qm set 9999 --sshkeys rsa.key
qm template 9999

# make six VMs from the template, using cloud-init to set IP and onboot info
curl https://github.com/bluefishforsale.keys > rsa.pub
for x in 0 1 ; do
for y in 1 2 3 ; do
qm clone 9999 6${x}${y}
qm set 6${x}${y} --name kube6${x}${y} --ipconfig0 ip=$(host kube6${x}${y}.home | awk '{print $NF}')/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 6${x}${y} --sshkeys rsa.pub
qm set 6${x}${y} --cores 40
qm set 6${x}${y} --memory 65536  # 64G
qm resize 6${x}${y} scsi0 +8G  #10G total
done
done
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm start


# stop and destroy
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm stop
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm destroy


# using ceph LVM
qm create 8888 --name debian-10-openstack-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 8888 debian-10-openstack-amd64.qcow2 ceph-lvm
qm set 8888 --scsihw virtio-scsi-pci --scsi0 ceph-lvm:vm-8888-disk-0
qm set 8888 --ide2 local-lvm:cloudinit
qm set 8888 --boot c --bootdisk scsi0
qm set 8888 --serial0 socket --vga serial0
qm set 8888 --sshkeys rsa.key
qm template 8888

# make six VMs from the template, using cloud-init to set IP and onboot info
curl https://github.com/bluefishforsale.keys > rsa.pub
for x in 0 1 ; do
for y in 1 2 3 ; do
qm clone 8888 6${x}${y}
qm set 6${x}${y} --name kube6${x}${y} --ipconfig0 ip=$(host kube6${x}${y}.home | awk '{print $NF}')/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 6${x}${y} --sshkeys rsa.pub
qm set 6${x}${y} --cores 40
qm set 6${x}${y} --memory 65536  # 64G
qm resize 6${x}${y} scsi0 +8G  #20G total
done
done
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm start
