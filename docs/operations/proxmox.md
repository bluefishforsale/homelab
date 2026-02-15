# Proxmox Operations Guide

Comprehensive guide for Proxmox VE operations, maintenance, and homelab-specific setup.

**Hardware Reference:**

- **node006** (192.168.1.106): 40 cores, 680GB RAM, 64TB ZFS (8x 12TB HDD), RTX 3090 24GB, bonded 10G
- **node005** (192.168.1.105): 56 cores, 128GB RAM, 1TB SSD

**VMs from inventory:**

- ocean (192.168.1.143) - Media/AI server on node006
- dns01 (192.168.1.2) - BIND9 DNS on node005
- pihole (192.168.1.9) - DNS filtering on node005
- gitlab (192.168.1.5) - GitLab on node005
- gh-runner-01 (192.168.1.250) - GitHub runner on node005

---

## Quick Reference

### Daily Health Checks

```bash
pvecm status && pvecm nodes     # Cluster status
pveversion && uptime && df -h   # Node health
pvesh get /cluster/resources    # Resource usage
ceph -s                         # Ceph status (if enabled)
```

### VM/Container Commands

```bash
qm list                              # List VMs
qm start|stop|shutdown|status {vmid} # VM control
qm config {vmid}                     # View VM config
qm migrate {vmid} {node} --online    # Live migration

pct list                             # List containers
pct start|stop|enter {ctid}          # Container control
```

### Backup/Restore

```bash
vzdump {vmid} --storage {storage} --mode snapshot   # Backup VM
vzdump --all --storage {storage} --mode snapshot    # Backup all
qmrestore {backup-file} {vmid} --storage {storage}  # Restore
pvesm list {backup-storage}                         # List backups
```

---

## Initial Setup

### APT Repository Configuration (Ansible)

Use the playbook to automate repo setup and subscription warning removal:

```bash
# Configure repos and remove subscription warning on all Proxmox nodes
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/proxmox_repos.yaml

# Or target specific node
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/proxmox_repos.yaml -l node006
```

### Manual APT Configuration

If not using Ansible:

```bash
# Get GPG key
wget https://enterprise.proxmox.com/debian/proxmox-release-bookworm.gpg \
  -O /etc/apt/trusted.gpg.d/proxmox-release-bookworm.gpg

# Configure no-subscription repos
cat << EOF > /etc/apt/sources.list
deb http://ftp.debian.org/debian bookworm main contrib
deb http://ftp.debian.org/debian bookworm-updates main contrib
deb http://download.proxmox.com/debian/pve bookworm pve-no-subscription
deb http://security.debian.org/debian-security bookworm-security main contrib
EOF

# Disable enterprise repo
echo "# deb https://enterprise.proxmox.com/debian/pve bookworm pve-enterprise" \
  > /etc/apt/sources.list.d/pve-enterprise.list

apt-get update
```

### Remove Subscription Warning (Manual)

```bash
sed -Ezi.bak "s/(Ext.Msg.show\(\{\s+title: gettext\('No valid sub)/void\(\{ \/\/\1/g" \
  /usr/share/javascript/proxmox-widget-toolkit/proxmoxlib.js && systemctl restart pveproxy.service
```

### Install QEMU Guest Agent (Ansible)

```bash
# Install on all VMs
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/proxmox_qemu_agent.yaml
```

### Network Bonding (Dual 10GbE)

```bash
cat << EOF > /etc/network/interfaces
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
ping -c 2 192.168.1.1 && ping -c 2 1.1.1.1
```

---

## GPU/PCI Passthrough

### Enable IOMMU and VFIO

```bash
# Enable IOMMU in GRUB
sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT="quiet"/GRUB_CMDLINE_LINUX_DEFAULT="quiet intel_iommu=on iommu=pt"/g' /etc/default/grub
update-grub

# Load VFIO modules
cat << EOF >> /etc/modules
vfio
vfio_iommu_type1
vfio_pci
vfio_virqfd
EOF

# Optional: unsafe interrupts for certain hardware
echo "options vfio_iommu_type1 allow_unsafe_interrupts=1" > /etc/modprobe.d/iommu_unsafe_interrupts.conf
echo "options kvm ignore_msrs=1" > /etc/modprobe.d/kvm.conf

# Blacklist GPU drivers
cat << EOF >> /etc/modprobe.d/blacklist.conf
blacklist radeon
blacklist nouveau
blacklist nvidia
EOF

# Additional driver blacklists (for SAS passthrough)
echo "blacklist mpt3sas" > /etc/modprobe.d/blacklist-mpt3sas.conf
```

### Configure VFIO Device Binding

```bash
# Get PCI IDs
lspci -nn | grep -E "NVIDIA|SAS|NVMe"

# Configure VFIO (adjust IDs for your hardware)
# RTX 3090: 10de:2204 (GPU) + 10de:1aef (Audio)
# SAS2308: 1000:0087
cat << EOF > /etc/modprobe.d/vfio.conf
options vfio-pci ids=10de:2204,10de:1aef,1000:0087 disable_vga=1
softdep mpt3sas pre: vfio-pci
EOF

# Apply changes
update-initramfs -u
reboot
```

### Verify Passthrough

```bash
dmesg | grep -e DMAR -e IOMMU              # Check IOMMU enabled
find /sys/kernel/iommu_groups/ -type l | sort -V  # Check IOMMU groups
lspci -nnk | grep -A 3 "NVIDIA\|SAS"       # Verify vfio-pci driver
ls -la /dev/vfio/                          # Check VFIO devices
pvesh get /nodes/{node}/hardware/pci       # Available for passthrough
```

---

## Ceph Storage

### Installation

```bash
# Add Ceph repo
cat << EOF > /etc/apt/sources.list.d/ceph.list
deb http://download.proxmox.com/debian/ceph-reef bookworm no-subscription
EOF

# Install and initialize
pveceph install --version reef --repository no-subscription
pveceph init --network 192.168.1.0/24
pveceph mon create
pveceph mgr create
ceph -s
```

### Single-Node CRUSH Map Fix

For single-node setups, change failure domain from `host` to `osd`:

```bash
ceph osd getcrushmap -o current.crush
crushtool -d current.crush -o current.txt

# Edit current.txt: change "step chooseleaf firstn 0 type host" to "type osd"
# rule replicated_rule { ... step chooseleaf firstn 0 type osd ... }

crushtool -c current.txt -o new.crush
ceph osd setcrushmap -i new.crush
```

### Create OSDs with NVMe DB

```bash
# Write keyring
ceph auth get client.bootstrap-osd > /etc/pve/priv/ceph.client.bootstrap-osd.keyring

# Create 8 OSDs with shared NVMe for block.db
ceph-volume lvm batch /dev/sd{a,b,c,d,e,f,g,h} --db-devices /dev/nvme0n1 --yes

# Create pools
ceph osd pool create replicated-data-pool-name 64
ceph osd pool application enable replicated-data-pool-name rbd
pvesm add rbd ceph-lvm -pool replicated-data-pool-name
```

### CephFS Setup

```bash
ceph osd pool create cephfs_metadata_pool 32
ceph osd pool application enable cephfs_metadata_pool cephfs
ceph fs new cephfs cephfs_metadata_pool replicated-data-pool-name

# Reduce replication for single-node (optional)
ceph osd pool set cephfs_data size 2
ceph osd pool set cephfs_metadata size 2
```

### Ceph Operations

```bash
# Health and monitoring
ceph -s && ceph health detail
ceph -w                     # Real-time monitoring
ceph df && rados df         # Usage

# OSD management
ceph osd tree
ceph osd out|in|down {osd-id}

# Pool management
ceph osd lspools
ceph osd pool get {pool} all
ceph osd pool set-quota {pool} max_bytes 1TB

# RBD operations
rbd ls -l {pool}
rbd info {pool}/{image}
rbd snap create {pool}/{image}@{snap}
```

### Nuclear Option: Destroy All Ceph

```bash
pveceph mds destroy $HOSTNAME
pveceph fs destroy cephfs
pvesm remove rbd ceph-lvm -pool data
for pool in data cephfs_data cephfs_metadata; do pveceph pool destroy $pool; done
for osd in $(seq 0 7); do
  for step in stop down out purge destroy; do ceph osd $step $osd --force; done
done
lvdisplay | grep ceph | grep Name | awk '{print $3}' | xargs lvremove --yes
vgdisplay | grep 'VG Name' | grep ceph | awk '{print $3}' | xargs vgremove -y
for disk in a b c d e f g h; do wipefs -a /dev/sd${disk}; done
wipefs -a /dev/nvme0n1
pveceph mgr destroy $HOSTNAME
pveceph mon destroy $HOSTNAME
pveceph stop && pveceph purge
rm /etc/pve/ceph.conf
find /var/lib/ceph/ -mindepth 2 -delete
```

---

## VM Templates and Deployment

### Create Debian 12 Template (VMID 9999)

```bash
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2
curl https://github.com/bluefishforsale.keys > rsa.keys

qm create 9999 --name debian-12-generic-amd64 --net0 virtio,bridge=vmbr0
qm importdisk 9999 debian-12-generic-amd64.qcow2 local-lvm
qm set 9999 --net0 virtio,bridge=vmbr0,queues=64
qm set 9999 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-9999-disk-0
qm set 9999 --bios ovmf --machine q35
qm set 9999 --efidisk0 local-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=4M
qm set 9999 --boot order=scsi0
qm set 9999 --ide2 local-lvm:cloudinit
qm set 9999 --serial0 socket --vga serial0
qm set 9999 --sshkeys rsa.keys
qm set 9999 --cores 2 --memory 4096
qm set 9999 --agent enabled=1
qm set 9999 --hotplug network,disk
qm template 9999
```

### Homelab VMs

#### dns01 (VMID 2000)

```bash
qm clone 9999 2000
qm set 2000 --name dns01 --ipconfig0 ip=192.168.1.2/24,gw=192.168.1.1 --nameserver=1.1.1.1 --onboot 1
qm set 2000 --cores 1 --memory 1024
qm resize 2000 scsi0 +8G
qm start 2000
```

#### pihole (VMID 3000)

```bash
qm clone 9999 3000
qm set 3000 --name pihole --ipconfig0 ip=192.168.1.9/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 3000 --cores 1 --memory 1024
qm resize 3000 scsi0 +8G
qm start 3000
```

#### gitlab (VMID 4000)

```bash
qm clone 9999 4000
qm set 4000 --name gitlab --ipconfig0 ip=192.168.1.5/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 4000 --cores 16 --memory 32768
qm resize 4000 scsi0 +28G
qm start 4000
```

#### ocean (VMID 5000) - Primary Media/AI Server

```bash
qm clone 9999 5000
qm set 5000 --name ocean --ipconfig0 ip=192.168.1.143/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 5000 --cores 30 --memory 262144
qm resize 5000 scsi0 +126G
qm set 5000 --hostpci0=42:00,pcie=1,x-vga=1  # RTX 3090 GPU
qm set 5000 --hostpci1=02:00,pcie=1          # SAS Controller
qm start 5000
```

**Post-install ZFS import in ocean VM:**

```bash
sudo sed -i 's/^Components: main$/Components: main contrib non-free non-free-firmware/' /etc/apt/sources.list.d/debian.sources
sudo apt update && sudo apt install -y zfsutils-linux
sudo modprobe zfs && sudo zpool import -f data01
zpool status data01
```

#### metrics (VMID 6000)

```bash
qm clone 9999 6000
qm set 6000 --cores 16 --memory 16384
qm resize 6000 scsi0 +100G
qm start 6000
```

#### openclaw (VMID 7000)

```bash
qm clone 9999 7000
qm set 7000 --name openclaw --ipconfig0 ip=192.168.1.31/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 7000 --cores 4 --memory 4096
qm resize 7000 scsi0 +16G
qm start 7000
```

#### Kubernetes Cluster (VMIDs 501-513)

```bash
x=5
for y in 0 1; do
  for z in 1 2 3; do
    n="${x}${y}${z}"
    qm clone 9999 ${n}
    qm set ${n} --name kube${n} --ipconfig0 ip=$(host kube${n}.home | awk '{print $NF}')/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
    qm resize ${n} scsi0 +8G
    qm set ${n} --cores 8 --memory 8192
    [[ "$y" == "3" ]] && qm set ${n} --hostpci0=42:00,pcie=1  # GPU on kube*13
    qm start ${n}
  done
done

# Bulk operations
for x in 5; do for y in 0 1; do for z in 1 2 3; do qm start "$x$y$z"; done; done; done   # Start all
for x in 5; do for y in 0 1; do for z in 1 2 3; do qm stop "$x$y$z"; qm destroy "$x$y$z"; done; done; done  # Destroy all
```

---

## Performance Tuning

### VM Optimization

```bash
qm set {vmid} --cpu host,flags=+aes,+avx,+avx2           # CPU passthrough
qm set {vmid} --scsi0 {storage}:vm-{vmid}-disk-0,cache=writeback,iothread=1,discard=on,ssd=1
qm set {vmid} --net0 virtio,bridge=vmbr0
qm set {vmid} --balloon 0                                 # Disable ballooning
qm set {vmid} --numa 1                                    # Enable NUMA
```

### Ceph SSD Optimization

```bash
ceph tell osd.* injectargs '--osd_op_queue wpq'
ceph tell osd.* injectargs '--osd_op_queue_cut_off high'
```

---

## Troubleshooting

### VM Issues

```bash
qm config {vmid}                    # Check config
qm unlock {vmid}                    # Remove locks
pvesm status                        # Check storage
tail -f /var/log/syslog | grep {vmid}
```

### Network Issues

```bash
brctl show && ip link show vmbr0
cat /proc/net/dev | grep vmbr0      # Check dropped packets
systemctl restart networking        # Last resort
```

### Ceph Issues

```bash
ceph health detail
ceph osd tree
ceph osd scrub {osd-id}
ceph pg deep-scrub {pg-id}
```

---

## Monitoring

### Prometheus PVE Exporter

```bash
wget https://github.com/prometheus-pve/prometheus-pve-exporter/releases/download/v3.0.0/pve_exporter-3.0.0.pve
dpkg -i pve_exporter-3.0.0.pve

cat > /etc/pve-exporter/pve.yml << EOF
default:
  user: monitoring@pve
  password: your-password
  verify_ssl: false
EOF

systemctl enable --now pve-exporter
```

### Log Files

```bash
tail -f /var/log/syslog              # System
tail -f /var/log/pveproxy.log        # Web UI
tail -f /var/log/ceph/ceph.log       # Ceph
journalctl -u pvedaemon -f           # PVE daemon
```

---

## Security

### User Management

```bash
pvesh get /access/users
pvesh create /access/users --userid newuser@pve --password secretpass
pvesh set /access/acl --path /vms/{vmid} --users user@pve --roles PVEVMUser
pvesh create /access/users/{userid}/token/{tokenid}
```

### SSL/ACME

```bash
pvenode acme account register default mail@example.com
pvenode acme cert order --domains pve.example.com
```

---

## Emergency Procedures

### Node Failure

```bash
pvecm expected 1                    # If losing quorum
pvecm delnode {failed-node}
```

### Split-Brain Recovery

```bash
pvecm expected 1
systemctl stop pve-cluster
rm -rf /var/lib/pve-cluster/cfs_lock
systemctl start pve-cluster
```

### Ceph Emergency

```bash
ceph osd set noout && ceph osd set norebalance
# Fix issues
ceph osd unset noout && ceph osd unset norebalance
```

---

## Maintenance Schedule

| Frequency | Tasks |
|-----------|-------|
| Daily | Cluster health, resource usage, backup status |
| Weekly | `apt update`, storage review, clean old backups |
| Monthly | Security updates, certificates, capacity planning |
| Quarterly | Major updates, DR testing, security audit |
