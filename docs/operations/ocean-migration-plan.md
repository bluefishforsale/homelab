# Ocean Server Migration: Ubuntu 20.04 to Proxmox/Debian

Completed migration of ocean from Ubuntu 20.04 baremetal to a Debian 12 VM on Proxmox.

---

## Quick Reference

| Component | Value |
|-----------|-------|
| Proxmox Host | node006 (192.168.1.106) |
| Ocean VM | 192.168.1.143 (VM on node006) |
| VM ID | 5000 |
| Template ID | 9999 |
| GPU | NVIDIA RTX 3090 (PCI 42:00) - passed through |
| SAS Controller | LSI SAS2308 (PCI 02:00) - passed through |
| ZFS Pool | data01 (8x 12TB raidz2) |
| VM Specs | 30 cores, 256GB RAM, 128GB boot disk |

---

## Migration Summary

**Completed:**

1. Proxmox VE installed on node006 (192.168.1.106)
2. IOMMU and VFIO configured for hardware passthrough
3. Debian 12 cloud-init template created (VM ID 9999)
4. Ocean VM created with GPU and SAS passthrough
5. ZFS pool data01 imported in VM
6. Docker services deployed via Ansible

**Hardware Passthrough:**

- GPU: NVIDIA RTX 3090 at PCI 42:00.0 (+ audio at 42:00.1)
- SAS Controller: Broadcom LSI SAS2308 at PCI 02:00.0 (8x disks)
- NVMe: NOT passed through (MSI-X issues) - remains on Proxmox host

## Architecture

### Storage: ZFS Pool (data01)

- 8x 12TB disks in raidz2 configuration
- Connected to SAS controller at PCI 02:00.0
- Controller passed through to VM for native ZFS performance

### Network

- Proxmox host (node006): 192.168.1.106
- Ocean VM: 192.168.1.143

### Hardware Passthrough

- GPU: NVIDIA RTX 3090 at PCI 42:00.0
- SAS Controller: PCI 02:00.0 (all ZFS disks)
- NVMe: NOT passed through (remains on Proxmox host)

---

## Step-by-Step Migration Guide

### ✅ COMPLETED: Proxmox Installation and VFIO Configuration
- Proxmox VE installed on node006 (192.168.1.106)
- SSH access configured (terrac user with keys, passwordless )
- Network bond0 configured (eth0+eth1)
- Old Ubuntu boot disk available via USB adapter if needed
- IOMMU enabled: `intel_iommu=on iommu=pt`
- VFIO modules loaded: vfio, vfio_iommu_type1, vfio_pci, vfio_virqfd
- Drivers blacklisted: nouveau, mpt3sas, nvme
- VFIO PCI IDs configured: 10de:2204, 10de:1aef, 1000:0087, 8086:f1a8

---

### Phase 1: Create Cloud-Init Template (On node006)

#### Step 1.1: Download Debian 12 Cloud Image
```bash
# SSH to node006 as terrac
ssh terrac@192.168.1.106

# Download Debian 12 cloud image
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2

# Download your SSH keys
curl https://github.com/bluefishforsale.keys > rsa.keys
```

#### Step 1.2: Create Template VM 9999
```bash
# Create VM shell
qm create 9999 --name debian-12-generic-amd64 --net0 virtio,bridge=vmbr0,queues=128

# Import disk to Proxmox storage (using local-lvm)
qm importdisk 9999 debian-12-generic-amd64.qcow2 local-lvm

# Configure VM hardware
qm set 9999 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-9999-disk-0
qm set 9999 --bios ovmf
qm set 9999 --machine q35
qm set 9999 --efidisk0 local-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=4M
qm set 9999 --boot order=scsi0
qm set 9999 --ide2 local-lvm:cloudinit
qm set 9999 --serial0 socket --vga serial0

# Configure cloud-init
qm set 9999 --sshkeys rsa.keys
qm set 9999 --hotplug network,disk
qm set 9999 --cores 2
qm set 9999 --memory 4096
qm set 9999 --agent enabled=1

# Convert to template
qm template 9999
```

---

### Phase 2: Verify VFIO Configuration (After Reboot)

#### Step 2.1: Verify IOMMU and VFIO Devices
```bash
# SSH to node006
ssh terrac@192.168.1.106

# Verify IOMMU is enabled
dmesg | grep -e DMAR -e IOMMU | head -20
# Should show: "DMAR: IOMMU enabled"

# Verify VFIO claimed GPU (both functions)
lspci -k -s 42:00.0 | grep "Kernel driver"
# MUST show: Kernel driver in use: vfio-pci
lspci -k -s 42:00.1 | grep "Kernel driver"
# MUST show: Kernel driver in use: vfio-pci

# Verify VFIO claimed SAS controller (NOT mpt3sas!)
lspci -k -s 02:00.0 | grep "Kernel driver"
# MUST show: Kernel driver in use: vfio-pci

# Verify VFIO claimed NVMe (NOT nvme!)
lspci -k -s 05:00.0 | grep "Kernel driver"
# MUST show: Kernel driver in use: vfio-pci

# Check VFIO devices are available
ls -la /dev/vfio/
# Should show multiple devices

# IMPORTANT: Verify disks and NVMe are NO LONGER visible to Proxmox host
lsblk
# You should NOT see:
# - sda through sdh (ZFS disks) - now controlled by VFIO
# - nvme0n1 (Intel NVMe) - now controlled by VFIO
# You should ONLY see:
# - sdk (Proxmox boot disk)
# - Potentially sdi (old Ubuntu disk via USB)
```

**⚠️ CRITICAL CHECK:** If any device still shows the wrong driver (mpt3sas, nvme, nouveau), STOP and troubleshoot before proceeding.

---

### Phase 3: Create Ocean VM

#### Step 3.1: Clone VM from Template

```bash
# SSH to node006
ssh terrac@192.168.1.106

# Clone from template 9999 to VM ID 5000
qm clone 9999 5000

# Configure ocean VM with DIRECT production IP (no temporary IP needed)
qm set 5000 --name ocean \
  --ipconfig0 ip=192.168.1.143/24,gw=192.168.1.1 \
  --nameserver=192.168.1.2 \
  --onboot 1

# Ensure multiqueue is configured for optimal network performance
qm set 5000 --net0 virtio,bridge=vmbr0,queues=128

# Resize disk to 128GB total (template is small, add the difference)
qm resize 5000 scsi0 +126G

# Set CPU and Memory (ocean specs: 30 cores, 256GB RAM)
qm set 5000 --cores 30
qm set 5000 --memory 262144  # 256GB in MB

# GPU Passthrough - NVIDIA RTX 3090 (includes both GPU and audio)
# Pass through entire function (42:00.0 and 42:00.1)
qm set 5000 --hostpci0=42:00,pcie=1,x-vga=1

# SAS Controller Passthrough - ALL ZFS disks at PCI address 02:00
# This gives VM direct control of all 8 ZFS disks (sda-sdh) for native ZFS performance
qm set 5000 --hostpci1=02:00,pcie=1

# NOTE: NVMe passthrough skipped - MSI-X capability issues prevent successful passthrough
# NVMe remains on Proxmox host, accessible via other methods if needed
```

#### Step 3.2: Start VM and Access
```bash
# Start ocean VM
qm start 5000

# Monitor boot progress
qm status 5000
# Wait ~30-60 seconds for cloud-init to complete

# Verify GPU is visible in VM
lspci | grep -i nvidia
# Expected: 
# 42:00.0 VGA compatible controller: NVIDIA Corporation GA102 [GeForce RTX 3090]
# 42:00.1 Audio device: NVIDIA Corporation GA102 High Definition Audio Controller

# Verify SAS controller and disks are visible in VM
lspci | grep -i sas
# Expected: 02:00.0 Serial Attached SCSI controller

# Check disks are present (will have different device names in VM)
lsblk
# Expected: 
# - 8x 10.9TB disks (sda-sdh) - ZFS pool disks
# - 1x boot disk (128GB)
# NOTE: NVMe is NOT passed through (remains on Proxmox host)
```

---

### Phase 4: Import ZFS Pool and Deploy Services

#### Step 4.1: Import ZFS Pool in VM
```bash
# Install ZFS utilities in VM
sudo apt update
sudo apt install -y zfsutils-linux

# Load ZFS kernel module
sudo modprobe zfs

# Verify ZFS module loaded
lsmod | grep zfs

# Check for importable pools
sudo zpool import
# Should show: pool: data01, state: ONLINE (or DEGRADED if resilvering)

# Import the data01 pool
sudo zpool import -f data01

# Verify pool is mounted
zpool status data01
zfs list
df -h | grep data01

# Check if resilver is still running
zpool status -v data01
# If resilver was in progress, it continues automatically

# Verify all datasets are accessible
ls -la /data01/
ls -la /data01/services/

# If resilver is running, monitor progress (optional)
watch -n 10 'zpool status data01 | grep -A 3 scan'
# Press Ctrl+C to exit watch
```

#### Step 4.2: Install Docker and System Dependencies

**Option A: Run Ansible Playbook (Recommended)**
```bash
# From your local machine with homelab repo
cd /path/to/homelab

# Run base system setup on ocean VM
ansible-playbook -i inventories/production/hosts.ini playbooks/01_base_system.yaml --limit ocean
```

**Option B: Manual Docker Installation**
```bash
# If Ansible not available, install Docker manually in VM
ssh terrac@192.168.1.143

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
 sh get-docker.sh
 usermod -aG docker terrac

# Install docker-compose
 apt install -y docker-compose-plugin

# Logout and back in for group changes
exit
ssh terrac@192.168.1.143
```

#### Step 4.3: Test GPU Access
```bash
# Install NVIDIA drivers in VM
 apt update
 apt install nvidia-driver nvidia-docker2

# Verify GPU
nvidia-smi

# Test Docker GPU access
docker run --rm --gpus all nvidia/cuda:11.8.0-base-ubuntu22.04 nvidia-smi
# Should show GPU information from within container
```

#### Step 4.4: Deploy All Services
```bash
# From your local machine with homelab repo
cd /path/to/homelab

# Deploy all ocean services via Ansible
ansible-playbook -i inventories/production/hosts.ini playbooks/00_site.yaml --limit ocean

# Or deploy specific services
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/*/ocean*.yaml

# Verify services are running
ssh terrac@192.168.1.143
docker ps -a
systemctl list-units --type=service | grep -E 'docker|service' | grep Active
```

---

### Phase 5: Validation and Cleanup

#### Step 5.1: Service Validation Checklist
```bash
# Test each service endpoint
services=(
  "plex:32400"
  "grafana:8910"
  "comfyui:8188"
  "nginx:80"
  "prometheus:9090"
  "llamacpp:8080"
  "open-webui:8085"
)

for svc in "${services[@]}"; do
  name="${svc%:*}"
  port="${svc#*:}"
  echo "Testing $name..."
  curl -f -s -o /dev/null "http://192.168.1.143:$port" && echo "✓ $name OK" || echo "✗ $name FAILED"
done

# Test GPU services
docker exec comfyui nvidia-smi

# Test media access
ls -la /data01/media/

# Test external access via cloudflare
curl https://grafana.terrac.com

# Verify ZFS pool health
zpool status data01

# Check resilver progress (if applicable)
zpool status -v data01 | grep scan
```

#### Step 5.2: Update Ansible Inventory
```bash
# Update inventories/production/hosts.ini
# Move ocean from [baremetal] to [vms] or create [proxmox_vms] section

# Before (old):
# [baremetal]
# ocean ansible_user=terrac nvidia_gpu=true ...

# After (new):
# [vms]
# ocean ansible_user=terrac nvidia_gpu=true ansible_ssh_host=192.168.1.143 bare_metal_host="node006"

# Commit changes
git add inventories/production/hosts.ini
git commit -m "ocean: migrated to Proxmox VM on node006"
```

#### Step 5.3: Rollback Procedure (If Needed)
```bash
# If critical issues found, old Ubuntu disk available via USB adapter

# Option A: Boot from old Ubuntu disk
1. Shutdown Proxmox node006
2. Change BIOS boot order to old disk (or remove new disk temporarily)
3. Boot into Ubuntu 20.04
4. Services resume automatically from /data01

# Option B: Fix issues in VM without rollback
1. SSH to Proxmox: ssh terrac@192.168.1.106
2. Access VM console: qm terminal 100
3. Debug and fix issues
4. Keep old Ubuntu disk available for 1-2 weeks
```

---

## ZFS Performance Architecture Summary

### ✅ Optimized Configuration

**Hardware Passthrough Design:**
- **GPU (PCI 42:00)** → ocean VM for AI/ML workloads
- **SAS Controller (PCI 02:00)** → ocean VM for native ZFS performance
- **All 8 disks (sda-sdh)** managed directly by VM

**Performance Benefits:**
- ⭐⭐⭐⭐⭐ **Native ZFS Performance**: Zero virtualization overhead
- ⭐⭐⭐⭐⭐ **Direct Hardware Access**: SAS controller managed by VM
- ⭐⭐⭐⭐⭐ **Full ZFS Features**: Snapshots, scrub, resilver all work natively
- ⭐⭐⭐⭐⭐ **Resilver Continuity**: Ongoing resilver continues seamlessly in VM

### Quick Reference Commands

**Create Cloud-Init Template:**
```bash
ssh terrac@192.168.1.106
cd /tmp
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2
curl https://github.com/bluefishforsale.keys > rsa.keys
qm create 9999 --name debian-12-generic-amd64 --net0 virtio,bridge=vmbr0,queues=128
qm importdisk 9999 debian-12-generic-amd64.qcow2 local-lvm
qm set 9999 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-9999-disk-0
qm set 9999 --bios ovmf --machine q35
qm set 9999 --efidisk0 local-lvm:0,format=raw,efitype=4m,pre-enrolled-keys=0,size=4M
qm set 9999 --boot order=scsi0 --ide2 local-lvm:cloudinit --serial0 socket --vga serial0
qm set 9999 --sshkeys rsa.keys
qm set 9999 --cores 2 --memory 4096 --agent enabled=1 --hotplug network,disk
qm template 9999
```

**Create VM with Hardware Passthrough:**
```bash
qm clone 9999 5000
qm set 5000 --name ocean --ipconfig0 ip=192.168.1.143/24,gw=192.168.1.1 --nameserver=192.168.1.2 --onboot 1
qm set 5000 --net0 virtio,bridge=vmbr0,queues=128  # Ensure multiqueue is configured
qm resize 5000 scsi0 +126G
qm set 5000 --cores 30 --memory 262144  # 30 cores, 256GB RAM
qm set 5000 --hostpci0=42:00,pcie=1,x-vga=1  # RTX 3090 GPU (includes audio)
qm set 5000 --hostpci1=02:00,pcie=1  # SAS Controller + all 8 ZFS disks
# NOTE: NVMe passthrough skipped due to MSI-X issues
qm start 5000
```

**Import ZFS Pool in VM:**
```bash
ssh terrac@192.168.1.143
sudo apt update && sudo apt install -y zfsutils-linux
sudo modprobe zfs
lsmod | grep zfs
sudo zpool import data01
zpool status data01
```

---

## Timeline Recommendation

**Total estimated time: 1.5-2 hours remaining**

- ✅ **Proxmox Installation**: COMPLETED
- ✅ **IOMMU/VFIO Configuration**: COMPLETED
- ✅ **Final Reboot**: IN PROGRESS
- **Phase 1 (Template creation)**: 15 min
- **Phase 2 (Verify VFIO)**: 5 min
- **Phase 3 (Create VM)**: 10 min
- **Phase 4 (Import ZFS & Deploy)**: 60-90 min
- **Phase 5 (Validation)**: 15 min

**Current Status:**
- ✅ Proxmox installed and accessible
- ✅ IOMMU enabled (intel_iommu=on iommu=pt)
- ✅ VFIO configured for GPU, SAS, NVMe
- ✅ Drivers blacklisted (nouveau, mpt3sas, nvme)
- 🔄 Final reboot in progress
- 📋 Next: Verify VFIO claimed all hardware
- Old Ubuntu disk available via USB for rollback

---

## Critical Safety Reminders

1. ✅ **DISK SWAP:** COMPLETED - New disk installed, Proxmox running
2. ✅ **ROLLBACK:** Old Ubuntu disk available via USB adapter
3. ✅ **VFIO CONFIG:** IOMMU and VFIO configured for all hardware
4. 🔄 **VFIO VERIFICATION:** After reboot, MUST verify devices using vfio-pci driver:
   - GPU (42:00.0, 42:00.1) - MUST use vfio-pci (NOT nouveau)
   - SAS Controller (02:00.0) - MUST use vfio-pci (NOT mpt3sas)
   - NVMe (05:00.0) - Should use nvme driver (NOT passed through due to MSI-X issues)
5. ⚠️ **DISK VISIBILITY:** After VFIO, sda-sdh will NOT be visible on Proxmox host (only in VM)
   - NVMe remains on Proxmox host (not passed through)
6. ⚠️ **ZFS IMPORT:** ZFS pool will be imported in VM after hardware passthrough
7. ⚠️ **RESILVER:** ZFS resilver continues automatically after import in VM
8. ⚠️ **DIRECT IP:** Using production IP 192.168.1.143 directly (no temporary IP)
9. ⚠️ **VALIDATION:** Test all services thoroughly before declaring success

---

## Alternative: Direct Debian Installation (No Proxmox)

If you decide to skip Proxmox and install Debian directly:

```bash
# Same process as Phase 2.3, but:
# 1. Install Debian 12 instead of Proxmox
# 2. Configure IP as 192.168.1.143 directly
# 3. No VM needed - all services run on host
# 4. Simpler, but less flexible
# 5. GPU directly available (easier setup)
```

**Pros:**
- Simpler setup
- Better performance (no VM overhead)
- Direct GPU access

**Cons:**
- No VM isolation
- Harder to test changes
- Less flexible for future use cases

---

## Post-Migration

Ocean is now managed via Ansible. Deploy services with:

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/03_ocean_services.yaml --ask-vault-pass
```

See [getting-started.md](/docs/setup/getting-started.md) for service deployment details.
