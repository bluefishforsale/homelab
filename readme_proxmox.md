# Do all this before loading the web ui for the first time
    - it will ask to install ceph
    - best to have the right apt repos in place first
## Apt repos
### non-subscriptions repos
  - https://pve.proxmox.com/wiki/Package_Repositories
  - /etc/apt/sources.list
    ```
    deb http://ftp.debian.org/debian bookworm main contrib
    deb http://ftp.debian.org/debian bookworm-updates main contrib

    # Proxmox VE pve-no-subscription repository provided by proxmox.com,
    # NOT recommended for production use
    deb http://download.proxmox.com/debian/pve bookworm pve-no-subscription

    # security updates
    deb http://security.debian.org/debian-security bookworm-security main contrib
    ```

  - File /etc/apt/sources.list.d/pve-enterprise.list
    - comment out this line
    ```
    deb https://enterprise.proxmox.com/debian/pve bookworm pve-enterprise

File /etc/apt/sources.list.d/ceph.list
    deb https://enterprise.proxmox.com/debian/ceph-quincy bookworm enterprise
    ```
### you can load the web UI now, it will ask you to install ceph....

# Back to the shell on the proxmox node
## create the OSDs
https://pve.proxmox.com/wiki/Deploy_Hyper-Converged_Ceph_Cluster


##### write the ceph-client keyring for the OSDS
```
ceph auth get client.bootstrap-osd > /etc/pve/priv/ceph.client.bootstrap-osd.keyring
```
##### make 8 OSD w/ blockdb on a shared NVME disk
```
ceph-volume lvm batch --report $(printf "/dev/sd%s " $(for x in a b c d e f g h ; do echo $x ; done) ) --db-devices /dev/nvme0n1 --yes
```

Makes an LVM disk layout like this
```
    NAME                                                                                                  MAJ:MIN RM   SIZE RO TYPE MOUNTPOINTS
    sda                                                                                                     8:0    0   3.6T  0 disk
    └─ceph--f8f47e82--d0f2--4c9e--b500--afef4fbbca5b-osd--block--6bcfd3bc--8d20--4e76--ad55--1a59d41adfdc 253:4    0   3.6T  0 lvm
    sdb                                                                                                     8:16   0   3.6T  0 disk
    └─ceph--a4877a35--1aab--493d--8996--52eab43dd24f-osd--block--5ca049bb--fa90--4a62--849f--649fc87c72cf 253:6    0   3.6T  0 lvm
    sdc                                                                                                     8:32   0   3.6T  0 disk
    └─ceph--c316ca9e--6ec0--464b--851e--33d85e4adef9-osd--block--184117d4--b621--471c--800c--f9bb859b83dc 253:8    0   3.6T  0 lvm
    sdd                                                                                                     8:48   0   3.6T  0 disk
    └─ceph--dc9a4e5f--f95e--47a4--9e04--8b48b4caf6d9-osd--block--f4d3869a--a5ab--43c6--905b--d6ee22d29075 253:10   0   3.6T  0 lvm
    sde                                                                                                     8:64   0   3.6T  0 disk
    └─ceph--8d5accd8--882c--450a--822e--79d858851102-osd--block--b7fce87a--99ba--43ec--9aa8--e14cc624aad2 253:12   0   3.6T  0 lvm
    sdf                                                                                                     8:80   0   3.6T  0 disk
    └─ceph--08611e94--54ee--44e0--bf52--aec5ec4eb383-osd--block--80e439e1--ad9f--43c4--9a5b--a225bbab080d 253:14   0   3.6T  0 lvm
    sdg                                                                                                     8:96   0   3.6T  0 disk
    └─ceph--5fe634a8--e06c--451b--b968--3f0aaaf55685-osd--block--c0ef721b--0761--4806--a573--178fbdb8b982 253:16   0   3.6T  0 lvm
    sdh                                                                                                     8:112  0   3.6T  0 disk
    └─ceph--f40bc882--ac6b--4018--be91--68a1d1a9e514-osd--block--72c302a0--29d6--4986--b0ae--845a114da7fe 253:18   0   3.6T  0 lvm
    sdi                                                                                                     8:128  0 238.5G  0 disk
    ├─sdi1                                                                                                  8:129  0  1007K  0 part
    ├─sdi2                                                                                                  8:130  0     1G  0 part /boot/efi
    └─sdi3                                                                                                  8:131  0   234G  0 part
    ├─pve-root                                                                                          253:0    0  70.5G  0 lvm  /
    ├─pve-data_tmeta                                                                                    253:1    0   1.5G  0 lvm
    │ └─pve-data                                                                                        253:3    0 144.6G  0 lvm
    └─pve-data_tdata                                                                                    253:2    0 144.6G  0 lvm
        └─pve-data                                                                                        253:3    0 144.6G  0 lvm
    sdj                                                                                                     8:144  1     0B  0 disk
    sr0                                                                                                    11:0    1  1024M  0 rom
    nvme0n1                                                                                               259:0    0 931.5G  0 disk
    ├─ceph--3e30faf6--6b44--4876--a3df--680dd52277f8-osd--db--53e87228--89cf--4964--835d--ebe1e10b7d2a    253:5    0 116.4G  0 lvm
    ├─ceph--3e30faf6--6b44--4876--a3df--680dd52277f8-osd--db--81bfb138--e288--474f--a0eb--641fa2589a0d    253:7    0 116.4G  0 lvm
    ├─ceph--3e30faf6--6b44--4876--a3df--680dd52277f8-osd--db--da08a281--ca92--4e59--b84a--602ee8dcf2ff    253:9    0 116.4G  0 lvm
    ├─ceph--3e30faf6--6b44--4876--a3df--680dd52277f8-osd--db--241b94ba--b15b--42df--8ea6--62b0891db953    253:11   0 116.4G  0 lvm
    ├─ceph--3e30faf6--6b44--4876--a3df--680dd52277f8-osd--db--2d62d50e--95d0--4152--9546--96abd156a44a    253:13   0 116.4G  0 lvm
    ├─ceph--3e30faf6--6b44--4876--a3df--680dd52277f8-osd--db--d65abacb--aac3--4a47--9b78--ec72c9097068    253:15   0 116.4G  0 lvm
    ├─ceph--3e30faf6--6b44--4876--a3df--680dd52277f8-osd--db--023150b8--4bcd--41f3--af4d--aafe7fdb57b0    253:17   0 116.4G  0 lvm
    └─ceph--3e30faf6--6b44--4876--a3df--680dd52277f8-osd--db--2a58aee2--a8ed--4c49--a067--d271538c55f2    253:19   0 116.4G  0 lvm
```

#### data pool and pve storage
```
pveceph pool create data -application rbd
pvesm add rbd ceph-lvm -pool data
```
## cephfs
```
pveceph mds create
pveceph fs create --pg_num 32 --add-storage
```


## Totally screwed? Need to start over?
  -  we got you
### Destroy all ceph and reset disks
```
    pveceph mds destroy $HOSTNAME
    pveceph fs destroy cephfs
    pvesm remove rbd ceph-lvm -pool data
    for pool in data cephfs_data cephfs_metadata  ; do  pveceph pool destroy $pool ; done
    # here's the list of OSD IDs - change to what range is on this metal
    for osd in `seq 0 7` ; do for step in stop down out purge destroy ; do ceph osd $step osd.$osd --force  ; done ; done
    vgdisplay | grep 'VG Name' | grep ceph | awk '{print $3}'  | xargs vgremove -y
    for disk in a b c d e f g h ; do wipefs -a /dev/sd${disk} ; done
    wipefs -a /dev/nvme0
    pveceph mgr destroy node006
    pveceph mon destroy node006
    pveceph stop
    pveceph purge
    rm /etc/pve/ceph.conf
    find /var/lib/ceph/ -mindepth 2 -delete
```

### if on a single node
- change the crush map domain to OSD
```
    ceph osd getcrushmap -o current.crush
    crushtool -d current.crush -o current.txt
    vi  current.txt
```

- change the host -> osd in the replicated_rule
```
    # rules
    rule replicated_rule {
            id 0
            type replicated
            step take default
            step chooseleaf firstn 0 type osd
            step emit
    }
```
- put the new map in
```
    crushtool -c current.txt -o new.crush
    ceph osd setcrushmap -i new.crush
```


##### Removing the no-subscription warning from the UI
https://johnscs.com/remove-proxmox51-subscription-notice/
```
cd /usr/share/javascript/proxmox-widget-toolkit
cp proxmoxlib.js proxmoxlib.js.bak
```




## Making VMs
### Getting the debian base img
  - adding the SSH keys at the start
```
    wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2
    wget -O  rsa.key https://github.com/bluefishforsale.keys
```

### Make VM temaplate
```
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
```

## make six VMs from the template
  - using cloud-init to set IP and onboot info
  - one of them (kube603) has GPU PCI-e passthrough
  - this requires DNS entries be in place for each VM to look up the IP
```
for x in 0 1 ; do
for y in 1 2 3 ; do

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
    if [ $y = 3 ] ; then
        # this sets up this VM specificallt for nvidia pci-e GPU passthrough at PCI-e address 42:00.*
        qm set 6${x}${y} --hostpci0=42:00,pcie=1
    fi
fi
done
done
```

## start VMS
```
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done | xargs -n1 qm start
```

## stop and destroy all VMs
```
for x in $(seq  0 1) ; do for y in $(seq 1 3) ; do  echo "6$x$y" ; done ; done |\
 while read id ; do qm stop $id ; qm destroy $id ; done
```



### Make OPNsense VM
```
qm create 1000 --name OPNsense-23.7-vga-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 1000 	OPNsense-23.7-vga-amd64.img ceph-lvm
qm set 1000 --ide2 ceph-lvm:cloudinit
qm set 1000 --scsihw virtio-scsi-pci --scsi0 ceph-lvm:vm-1000-disk-0
qm set 1000 --boot order='scsi0'
qm set 1000 --serial0 socket --vga serial0
qm set 1000 --sshkeys rsa.key
qm set 1000 --ipconfig0 ip=192.168.1.4/24,gw=192.168.1.1 --nameserver=192.168.1.2
qm set 1000 --hotplug network,disk
qm set 1000 --bios ovmf
qm set 1000 --machine q35
qm set 1000 --efidisk0 ceph-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=1M
qm set 1000 --cores 4
qm set 1000 --memory 4096
qm set 1000 --agent enabled=1
qm resize 1000 scsi0 +6G
```

### Pihole VM
```
qm create 3000 --name pihole --net0 virtio,bridge=vmbr0
qm importdisk 3000 	debian-12-generic-amd64.qcow2 ceph-lvm
qm set 3000 --ide2 ceph-lvm:cloudinit
qm set 3000 --scsihw virtio-scsi-pci --scsi0 ceph-lvm:vm-3000-disk-0
qm set 3000 --boot order='scsi0'
qm set 3000 --serial0 socket --vga serial0
qm set 3000 --sshkeys rsa.key
qm set 3000 --ipconfig0 ip=192.168.1.9/24,gw=192.168.1.1 --nameserver=192.168.1.2
qm set 3000 --hotplug network,disk
qm set 3000 --bios ovmf
qm set 3000 --machine q35
qm set 3000 --efidisk0 ceph-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=1M
qm set 3000 --cores 2
qm set 3000 --memory 2048
qm set 3000 --agent enabled=1
qm resize 3000 scsi0 +6G
```

### dns01 VM
```
qm create 2000 --name dns01 --net0 virtio,bridge=vmbr0
qm importdisk 2000 	debian-12-generic-amd64.qcow2 ceph-lvm
qm set 2000 --ide2 ceph-lvm:cloudinit
qm set 2000 --scsihw virtio-scsi-pci --scsi0 ceph-lvm:vm-2000-disk-0
qm set 2000 --boot order='scsi0'
qm set 2000 --serial0 socket --vga serial0
qm set 2000 --sshkeys rsa.key
qm set 2000 --ipconfig0 ip=192.168.1.3/24,gw=192.168.1.1 --nameserver=1.1.1.1
qm set 2000 --hotplug network,disk
qm set 2000 --bios ovmf
qm set 2000 --machine q35
qm set 2000 --efidisk0 ceph-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=1M
qm set 2000 --cores 2
qm set 2000 --memory 2048
qm set 2000 --agent enabled=1
qm resize 2000 scsi0 +6G
```