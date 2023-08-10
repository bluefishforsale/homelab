wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2

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

<EDIT CLOUDINIT USER PASSWORD>

qm clone 9999 614
qm set 614 --name kube614 --ipconfig0 ip=$(host kube614.home | awk '{print $NF}')/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 614 --hostpci0=42:00,pcie=on,rombar=off,x-vga=off
qm set 614 --cores 9
qm set 614 --memory 57344  # 56G
qm start 614

rbd ls
rbd  --image vm-9999-disk-0 info --pool ceph-lvm
rbd map --pool ceph-lvm vm-9999-disk-0



qm create 1001 --name tmp --net0 virtio,bridge=vmbr0
qm importdisk 1001 debian-10-openstack-amd64.qcow2 ceph-lvm