# Install proxmox and setup ceph

- this document is hard coded for a specific setup
- read through, and change things to fit your setup
- machine has 8 HDD, and 1 NVME on `/dev/sd{a..h}` and `/dev/nvme0n1`
- boot disk is `/dev/sdi`
- ssh-keys stored in `github` and loaded during VM creation using github username
- software setup explained in [ansible readme](ansible//readme.md)
- if you changed things, consult the ansible inventory and make your changes consistent
- [ansible inventory](ansible/inventory.ini)

## networking interfaces to bond

- https://pve.proxmox.com/pve-docs/chapter-sysadmin.html#sysadmin_network_bond
- network: 192.168.1.0/2
- gateway: 191.168.1.1
- dual 10GbE as eno1 eno2 named interfaces

```bash
ip address add 192.168.1.106/24 dev eno1
ip route add default via 192.168.1.1via

cat << EOF > /etc/network/interfaces.new
auto lo
iface lo inet loopback

iface eno1 inet manual

iface eno2 inet manual

auto bond0
iface bond0 inet manual
      bond-slaves eno1 eno2
      bond-miimon 100
      bond-mode 802.3ad
      bond-xmit-hash-policy Layer3+4

auto vmbr0
iface vmbr0 inet static
        address  192.168.1.106/24
        gateway  192.168.1.1
        bridge-ports bond0
        bridge-stp off
        bridge-fd 0
EOF
ifreload -a
ping 192.168.1.1
ping 1.1.1.1
```

### GPU passthrough IOMMU

```bash
sed  /etc/default/grub  -i -e  's/GRUB_CMDLINE_LINUX_DEFAULT="quiet"/GRUB_CMDLINE_LINUX_DEFAULT="quiet intel_iommu=on"/g'
update-grub

cat << EOF >> /etc/modules
vfio
vfio_iommu_type1
vfio_pci
vfio_virqfd
EOF

echo "options vfio_iommu_type1 allow_unsafe_interrupts=1" > /etc/modprobe.d/iommu_unsafe_interrupts.conf
echo "options kvm ignore_msrs=1" > /etc/modprobe.d/kvm.conf
echo "blacklist radeon" >> /etc/modprobe.d/blacklist.conf
echo "blacklist nouveau" >> /etc/modprobe.d/blacklist.conf
echo "blacklist nvidia" >> /etc/modprobe.d/blacklist.conf

# add the PCI ID to the modprobe vfio config
echo "options vfio-pci ids=$(lspci | grep NVIDIA | awk '{print $1}' | xargs | sed -e 's/ /,/g') disable_vga=1"> /etc/modprobe.d/vfio.conf
```

## Do all this before loading the web UI for the first time

- because it will ask to install ceph
- it's best to have the right apt repos in place first

## Apt repos

### first get GPG key

```bash
wget https://enterprise.proxmox.com/debian/proxmox-release-bookworm.gpg -O /etc/apt/trusted.gpg.d/proxmox-release-bookworm.gpg
```

### non-subscriptions repos

- https://pve.proxmox.com/wiki/Package_Repositories
- bookworm (debian 12)
- security updates

```bash
cat << EOF > /etc/apt/sources.list
deb http://ftp.debian.org/debian bookworm main contrib
deb http://ftp.debian.org/debian bookworm-updates main contrib
deb http://download.proxmox.com/debian/pve bookworm pve-no-subscription
deb http://security.debian.org/debian-security bookworm-security main contrib
EOF
```

### comment out the enterprise repo

```bash
cat << EOF > /etc/apt/sources.list.d/pve-enterprise.list
# deb https://enterprise.proxmox.com/debian/pve bookworm pve-enterprise
EOF

apt-get update
```


### Removing the no-subscription warning from the UI

- https://johnscs.com/remove-proxmox51-subscription-notice/

```bash
cd /usr/share/javascript/proxmox-widget-toolkit
cp proxmoxlib.js proxmoxlib.js.bak
```

- https://johnscs.com/remove-proxmox51-subscription-notice/

```bash
sed -Ezi.bak "s/(Ext.Msg.show\(\{\s+title: gettext\('No valid sub)/void\(\{ \/\/\1/g" /usr/share/javascript/proxmox-widget-toolkit/proxmoxlib.js && systemctl restart pveproxy.service
```

### ceph reef  repo

```bash
cat << EOF > /etc/apt/sources.list.d/ceph.list
deb http://download.proxmox.com/debian/ceph-reef bookworm no-subscription
EOF
```



### install ceph

```bash
pveceph install --version reef --repository no-subscription
pveceph init --network 192.168.1.0/24
pveceph mon create
pveceph mgr create
ceph -s
```

### update crush map domain from  host to osd

- if on a single node
- change the crush map domain from host to OSD

```bash
ceph osd getcrushmap -o current.crush
crushtool -d current.crush -o current.txt
vi  current.txt
```

### change the host -> osd in the replicated_rule

- don't forget to revert when going to multiple nodes

#### what the rules should look like

```bash
rule replicated_rule {
        id 0
        type replicated
        step take default
        step chooseleaf firstn 0 type osd
        step emit
}
```

### put the new map in

```bash
crushtool -c current.txt -o new.crush
ceph osd setcrushmap -i new.crush
```

## create the OSDs

https://pve.proxmox.com/wiki/Deploy_Hyper-Converged_Ceph_Cluster

##### write the ceph-client keyring for the OSDS

```bash
ceph auth get client.bootstrap-osd > /etc/pve/priv/ceph.client.bootstrap-osd.keyring
```

### make 8 OSD w/ blockdb on a shared NVME disk

- create ceph OSD lvm PV, VG, and LV  LVMs
- creates and starts ceph OSD services and mounts the disks
- erasure coded pool

```bash
ceph-volume lvm batch --report $(printf "/dev/sd%s " $(for x in a b c d e f g h ; do echo $x ; done) ) --db-devices /dev/nvme0n1 --yes
ceph-volume lvm batch $(printf "/dev/sd%s " $(for x in a b c d e f g h ; do echo $x ; done) ) --db-devices /dev/nvme0n1 --yes
ceph osd erasure-code-profile set ec-profile-name k=4 m=2 crush-failure-domain=host
ceph osd pool create ec-data-pool-name 64 64 erasure ec-profile-name
ceph osd pool application enable ec-data-pool-name rbd
ceph osd pool create replicated-data-pool-name 64
ceph osd pool application enable replicated-data-pool-name rbd
ceph osd pool create replicated-data-pool-name 64
ceph osd pool application enable replicated-data-pool-name rbd
ceph osd tier add ec-data-pool-name replicated-data-pool-name
ceph osd tier cache-mode replicated-data-pool-name writeback
ceph osd tier set-overlay ec-data-pool-name replicated-data-pool-name
pvesm add rbd ceph-lvm -pool replicated-data-pool-name
```

### cephfs

```bash
ceph osd pool create cephfs_metadata_pool 32
ceph osd pool application enable cephfs_metadata_pool cephfs
ceph fs new cephfs cephfs_metadata_pool replicated-data-pool-name


# decrease replication factor from 3->2
ceph osd pool set cephfs_data size 2
ceph osd pool set cephfs_metadata size 2
```


# Totally screwed? Need to start over?
  -  we got you

## Destroy all ceph and reset disks
```bash
pveceph mds destroy $HOSTNAME
pveceph fs destroy cephfs
pvesm remove rbd ceph-lvm -pool data
for pool in data cephfs_data cephfs_metadata  ; do  pveceph pool destroy $pool ; done
# here's the list of OSD IDs - change to what range is on this metal
for osd in `seq 0 7` ; do for step in stop down out purge destroy ; do ceph osd $step $osd --force  ; done ; done
lvdisplay | grep ceph | grep Name  | awk '{print $3}' | xargs lvremove --yes
vgdisplay | grep 'VG Name' | grep ceph | awk '{print $3}'  | xargs vgremove -y
for disk in a b c d e f g h ; do wipefs -a /dev/sd${disk} ; done
wipefs -a /dev/nvme0n1
pveceph mgr destroy node006
pveceph mon destroy node006
pveceph stop
pveceph purge
rm /etc/pve/ceph.conf
find /var/lib/ceph/ -mindepth 2 -delete
```


### Make VM temaplate
## get SSH keys

wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2
curl https://github.com/bluefishforsale.keys > rsa.keys

```bash
qm create 9999 --name debian-12-generic-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 9999 debian-12-generic-amd64.qcow2 local-lvm
qm set 9999 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-9999-disk-0
qm set 9999 --bios ovmf
qm set 9999 --machine q35
qm set 9999 --efidisk0 local-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=4M
qm set 9999 --boot order=scsi0
qm set 9999 --ide2 local-lvm:cloudinit
qm set 9999 --serial0 socket --vga serial0
qm set 9999 --sshkeys rsa.keys
qm set 9999 --ciuser terrac
qm set 9999 --cipassword 2brak4u2
qm set 9999 --cores 2
qm set 9999 --memory 4096
qm set 9999 --agent enabled=1
qm set 9999 --hotplug network,disk
qm template 9999
```

#  just in case a root account is needed and keys don't work
qm set 9999 --ciuser debian
qm set 9999 --cipassword admin



### dns01 VM

```bash
qm clone 9999 2000
qm set 2000 --name dns01 --ipconfig0 ip=192.168.1.2/24,gw=192.168.1.1 --nameserver=1.1.1.1 --onboot 1
qm set 2000 --cores 1
qm set 2000 --memory 1024
qm resize 2000 scsi0 +8G
qm start 2000
```

### Pihole VM

```bash
qm clone 9999 3000
qm set 3000 --name pihole --ipconfig0 ip=192.168.1.9/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 3000 --cores 1
qm set 3000 --memory 1024
qm resize 3000 scsi0 +8G
qm start 3000
```

### Gitlab VM

```bash
qm clone 9999 4000
qm set 4000 --name gitlab --ipconfig0 ip=192.168.1.5/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 4000 --cores 16
qm set 4000 --memory 32768
qm resize 4000 scsi0 +28G
qm start 4000
```

# OCEAN VM
## passes through specific PCIE addresses for GPU and SAS controller

### Pre-requisite: Enable IOMMU and VFIO on Proxmox Host

Before creating the VM, configure hardware passthrough:

```bash
# 1. Enable IOMMU in GRUB
 nano /etc/default/grub
# Change: GRUB_CMDLINE_LINUX_DEFAULT="quiet"
# To:     GRUB_CMDLINE_LINUX_DEFAULT="quiet intel_iommu=on iommu=pt"
 update-grub

# 2. Load VFIO modules
 tee -a /etc/modules <<EOF
vfio
vfio_iommu_type1
vfio_pci
vfio_virqfd
EOF

# 3. Blacklist drivers
echo "blacklist nouveau" |  tee /etc/modprobe.d/blacklist-nouveau.conf
echo "blacklist mpt3sas" |  tee /etc/modprobe.d/blacklist-mpt3sas.conf

# 4. Get PCI IDs and configure VFIO
lspci -nn -s 42:00  # RTX 3090 GPU + Audio
lspci -nn -s 02:00  # SAS controller
lspci -nn -s 05:00  # NVMe SSD

# Find NVMe PCI address (if needed)
lspci | grep -i nvme
ls -la /sys/block/nvme0n1  # Shows PCI path in symlink

# Configure VFIO to claim devices (use actual IDs from lspci -nn output)
# RTX 3090: 10de:2204 (GPU) + 10de:1aef (Audio)
# SAS2308: 1000:0087
# Intel NVMe 660P: 8086:f1a8 (NOTE: Has MSI-X passthrough issues, skip for now)
echo "options vfio-pci ids=10de:2204,10de:1aef,1000:0087" |  tee /etc/modprobe.d/vfio.conf
echo "softdep mpt3sas pre: vfio-pci" |  tee -a /etc/modprobe.d/vfio.conf
# NOTE: NVMe passthrough disabled due to MSI-X capability errors
# echo "softdep nvme pre: vfio-pci" |  tee -a /etc/modprobe.d/vfio.conf

# 5. Update initramfs and reboot
 update-initramfs -u
 reboot
```

### Verify VFIO Configuration (After Reboot)

```bash
# Verify IOMMU enabled
dmesg | grep -e DMAR -e IOMMU

# Verify VFIO claimed GPU and SAS controller
lspci -k -s 42:00.0 | grep "Kernel driver"  # Should show: vfio-pci
lspci -k -s 42:00.1 | grep "Kernel driver"  # Should show: vfio-pci
lspci -k -s 02:00.0 | grep "Kernel driver"  # Should show: vfio-pci (NOT mpt3sas)

# NVMe should still use nvme driver (not passed through)
lspci -k -s 05:00.0 | grep "Kernel driver"  # Should show: nvme

# Check VFIO devices available
ls -la /dev/vfio/

# Note: After VFIO claims devices, they will NOT be visible to Proxmox:
# - ZFS disks (sda-sdh) will disappear from host - only in ocean VM
# - NVMe (nvme0n1) remains on Proxmox host (not passed through due to MSI-X issues)
```

### Create Ocean VM

```bash
qm clone 9999 5000
qm set 5000 --name ocean --ipconfig0 ip=192.168.1.143/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm resize 5000 scsi0 +126G  # 128GB total boot disk
qm set 5000 --cores 30      # 30 cores for VM
qm set 5000 --memory 262144 # 256GB RAM
qm set 5000 --hostpci0=42:00,pcie=1,x-vga=1  # RTX 3090 GPU (includes audio)
qm set 5000 --hostpci1=02:00,pcie=1          # SAS Controller + 8 ZFS disks (sda-sdh)
# NOTE: NVMe passthrough has MSI-X capability issues - skip for now
# qm set 5000 --hostpci2=05:00,pcie=1,rombar=0  # Intel NVMe 660P (1.9TB) - FAILS
qm start 5000
```

### Post-Install: Import ZFS Pool in VM

```bash
# SSH to ocean VM
ssh terrac@192.168.1.143

# Enable contrib and non-free repos for ZFS (Debian 12 DEB822 format)
sudo sed -i 's/^Components: main$/Components: main contrib non-free non-free-firmware/' /etc/apt/sources.list.d/debian.sources

# Update and install ZFS utilities
sudo apt update
sudo apt install -y zfsutils-linux

# Load ZFS kernel module
sudo modprobe zfs

# Verify module loaded
lsmod | grep zfs

# Import ZFS pool
sudo zpool import -f data01

# Verify pool status
zpool status data01

# Check resilver progress (if applicable)
watch -n 10 'zpool status data01 | grep -A 3 scan'

# Verify all disks are visible
lsblk
# Should show: 8x 10.9TB ZFS disks (sda-sdh) + 128GB boot disk
# Note: NVMe (nvme0n1) is NOT passed through due to MSI-X issues
```

## make six kube VMs from the template

- using cloud-init to set IP and onboot info
- one of them (kube013) has GPU PCI-e passthrough
- this requires DNS entries be in place for each VM to look up the IP

```bash
x=5
for y in 0 1 ; do
    for z in 1 2 3 ; do
        n="${x}${y}${z}"
        qm clone 9999 ${n}
        qm set ${n} --name kube${n} --ipconfig0 ip=$(host kube${n}.home | awk '{print $NF}')/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
        qm resize ${n} scsi0 +8G  # 10G
        if [ $x = 0 ] ; then
            qm set ${n} --cores 4
            qm set ${n} --memory 2048  # 8G
        else
            qm set ${n} --cores 8
            qm set ${n} --memory 8192  # 56G
            if [ $y = 3 ] ; then
                # this sets up this VM specifically for nvidia GPU passthrough at PCI-e address 42:00.*
                qm set ${n} --hostpci0=42:00,pcie=1
            fi
        fi
        qm start "$x$y$z"
    done
done
```

## start kube VMS

```bash
for x in 5 ; do for y in $(seq  0 1) ; do for z in $(seq 1 3) ; do  qm start "$x$y$z" ; done ; done ; done
```

## stop and destroy all kube VMS

```bash
for x in 5 ; do for y in $(seq  0 1) ; do for z in $(seq 1 3) ; do qm stop "$x$y$z" ; qm destroy "$x$y$z" ; done ; done ; done
```



