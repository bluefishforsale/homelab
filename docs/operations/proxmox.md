# 🏗️ Proxmox Operations Guide

## Overview

This guide covers day-to-day operations, maintenance, and advanced configuration for your Proxmox VE cluster.

## 📋 Daily Operations

### Cluster Health Monitoring
```bash
# Check cluster status
pvecm status
pvecm nodes

# Check node health
pveversion
uptime
df -h

# Monitor resource usage
pvesh get /cluster/resources
pvesh get /nodes/{node}/status
```

### VM Management
```bash
# List all VMs
qm list

# Start/stop VMs
qm start {vmid}
qm stop {vmid}
qm shutdown {vmid}  # Graceful shutdown

# VM status and configuration
qm status {vmid}
qm config {vmid}

# Live migration
qm migrate {vmid} {target-node}
qm migrate {vmid} {target-node} --online  # Live migration
```

### Container Management
```bash
# List containers
pct list

# Container operations
pct start {ctid}
pct stop {ctid}
pct enter {ctid}  # Enter container console

# Container configuration
pct config {ctid}
pct set {ctid} --memory 2048  # Modify container
```

## 🔧 System Maintenance

### Package Management
```bash
# Update package lists
apt update

# Upgrade Proxmox (be careful with major versions)
apt list --upgradable
apt upgrade

# Check for Proxmox-specific updates
pve-efiboot-tool refresh  # Update EFI boot entries
update-grub              # Update GRUB configuration
```

### Storage Management
```bash
# List storage configurations
pvesm status

# Storage usage
pvesm list {storage}
df -h

# Clean up storage
vzdump --clean 1  # Clean old backups (keep 1)
qm disk-cleanup {vmid}  # Clean unused VM disks
```

### Backup Operations
```bash
# Manual backup
vzdump {vmid} --storage {backup-storage} --mode snapshot

# Backup all VMs
vzdump --all --storage {backup-storage} --mode snapshot

# Restore from backup
qmrestore {backup-file} {vmid} --storage {storage}

# List backups
pvesm list {backup-storage}
```

## 🌐 Network Operations

### Bridge Management
```bash
# List network interfaces
ip link show
brctl show

# Network configuration
cat /etc/network/interfaces

# Apply network changes (be careful!)
ifreload -a

# Test connectivity
ping -c 4 192.168.1.1
```

### VLAN Configuration
```bash
# Add VLAN to bridge
echo "auto vmbr0.100
iface vmbr0.100 inet manual" >> /etc/network/interfaces

# Configure VLAN bridge
echo "auto vmbr100
iface vmbr100 inet static
    address 192.168.100.1/24
    bridge-ports vmbr0.100
    bridge-stp off
    bridge-fd 0" >> /etc/network/interfaces
```

### Firewall Management
```bash
# Cluster firewall status
pvesh get /cluster/firewall/options

# Node firewall status
pvesh get /nodes/{node}/firewall/options

# List firewall rules
pvesh get /cluster/firewall/rules
pvesh get /nodes/{node}/firewall/rules

# VM-specific firewall
pvesh get /nodes/{node}/qemu/{vmid}/firewall/rules
```

## 💾 Ceph Storage Operations

### Cluster Health
```bash
# Overall cluster status
ceph -s
ceph health detail

# Monitor cluster in real-time
ceph -w

# Check cluster usage
ceph df
rados df
```

### OSD Management
```bash
# List OSDs
ceph osd tree
ceph osd ls

# OSD status
ceph osd stat
ceph osd dump

# Manage OSDs
ceph osd out {osd-id}    # Mark OSD out
ceph osd in {osd-id}     # Mark OSD in
ceph osd down {osd-id}   # Mark OSD down

# Remove OSD (careful!)
ceph osd out {osd-id}
systemctl stop ceph-osd@{osd-id}
ceph osd crush remove osd.{osd-id}
ceph osd rm {osd-id}
```

### Pool Management
```bash
# List pools
ceph osd lspools
rados lspools

# Pool information
ceph osd pool get {pool-name} all
ceph osd pool stats

# Create pool
ceph osd pool create {pool-name} {pg-num}

# Set pool quotas
ceph osd pool set-quota {pool-name} max_objects 1000
ceph osd pool set-quota {pool-name} max_bytes 1TB
```

### RBD Operations
```bash
# List RBD images
rbd ls {pool-name}
rbd ls -l {pool-name}  # Detailed list

# RBD image information
rbd info {pool-name}/{image-name}

# Create RBD snapshot
rbd snap create {pool-name}/{image-name}@{snap-name}

# List snapshots
rbd snap ls {pool-name}/{image-name}

# Clone from snapshot
rbd clone {pool-name}/{image-name}@{snap-name} {pool-name}/{new-image}
```

## 🔧 Advanced Configuration

### GPU Passthrough Setup
```bash
# Enable IOMMU in GRUB
echo 'GRUB_CMDLINE_LINUX_DEFAULT="quiet intel_iommu=on iommu=pt"' >> /etc/default/grub
update-grub

# Load VFIO modules
echo 'vfio
vfio_iommu_type1
vfio_pci
vfio_virqfd' >> /etc/modules

# Blacklist GPU driver on host
echo 'blacklist nouveau
blacklist nvidia' >> /etc/modprobe.d/blacklist.conf

# Bind GPU to VFIO
lspci | grep NVIDIA  # Note the PCIe ID
echo 'options vfio-pci ids=10de:1b06' >> /etc/modprobe.d/vfio.conf

# Update initramfs
update-initramfs -u
reboot
```

### PCI Passthrough Validation
```bash
# Check IOMMU groups
find /sys/kernel/iommu_groups/ -type l | sort -V

# Verify VFIO binding
lspci -nnk | grep -A 3 NVIDIA

# Check available devices for passthrough
pvesh get /nodes/{node}/hardware/pci
```

### CPU Configuration
```bash
# Check CPU features
cat /proc/cpuinfo | grep flags

# Enable CPU features for VMs
qm set {vmid} --cpu host  # Pass through all CPU features
qm set {vmid} --cpu kvm64,+aes,+avx,+avx2  # Specific features

# NUMA configuration
qm set {vmid} --numa 1  # Enable NUMA
qm set {vmid} --sockets 2 --cores 4  # Multi-socket config
```

## 🚨 Troubleshooting

### Common Issues

#### VM Won't Start
```bash
# Check VM configuration
qm config {vmid}

# Check for locks
qm unlock {vmid}

# Check storage availability
pvesm status

# VM logs
tail -f /var/log/syslog | grep {vmid}
```

#### Network Issues
```bash
# Check bridge status
brctl show
ip link show vmbr0

# Reset network stack (last resort)
systemctl restart networking

# Check for dropped packets
cat /proc/net/dev | grep vmbr0
```

#### Storage Issues
```bash
# Check Ceph health
ceph health detail

# Check OSD status
ceph osd tree

# Fix common issues
ceph osd scrub {osd-id}     # Manual scrub
ceph pg deep-scrub {pg-id}  # Deep scrub specific PG
```

### Performance Optimization

#### VM Performance Tuning
```bash
# Enable VirtIO drivers
qm set {vmid} --scsi0 local-lvm:vm-{vmid}-disk-0,cache=writeback,iothread=1
qm set {vmid} --net0 virtio,bridge=vmbr0

# CPU optimization
qm set {vmid} --cpu host,flags=+aes,+avx,+avx2
qm set {vmid} --vcpus 4  # Enable vCPU hotplug

# Memory optimization
qm set {vmid} --balloon 0  # Disable memory ballooning
qm set {vmid} --shares 2000  # CPU shares for priority
```

#### Storage Performance
```bash
# Enable SSD optimizations for Ceph
ceph tell osd.* injectargs '--osd_op_queue wpq'
ceph tell osd.* injectargs '--osd_op_queue_cut_off high'

# Tune VM disk settings
qm set {vmid} --scsi0 local-lvm:vm-{vmid}-disk-0,discard=on,ssd=1
```

## 📊 Monitoring Integration

### Prometheus Metrics
```bash
# Install PVE exporter
wget https://github.com/prometheus-pve/prometheus-pve-exporter/releases/download/v3.0.0/pve_exporter-3.0.0.pve
dpkg -i pve_exporter-3.0.0.pve

# Configure exporter
cat > /etc/pve-exporter/pve.yml << EOF
default:
  user: monitoring@pve
  password: your-password
  verify_ssl: false
EOF

# Start exporter
systemctl enable --now pve-exporter
```

### Log Analysis
```bash
# Important log files
tail -f /var/log/syslog          # System logs
tail -f /var/log/pveproxy.log    # Web interface logs
tail -f /var/log/ceph/ceph.log   # Ceph cluster logs

# Journal logs
journalctl -u pvedaemon -f       # PVE daemon
journalctl -u ceph-mon@$(hostname) -f  # Ceph monitor
```

## 🔐 Security Operations

### User Management
```bash
# List users
pvesh get /access/users

# Add user
pvesh create /access/users --userid newuser@pve --password secretpass

# User permissions
pvesh set /access/acl --path /vms/{vmid} --users user@pve --roles PVEVMUser

# API tokens
pvesh create /access/users/{userid}/token/{tokenid} --expire 1577836800
```

### SSL Certificate Management
```bash
# Upload custom certificate
pvesm upload {storage} {cert-file} --content vztmpl

# Let's Encrypt (if using ACME)
pvenode acme account register default mail@example.com
pvenode acme cert order --domains pve.example.com
```

## 📅 Maintenance Schedule

### Daily Tasks
- [ ] Check cluster health (`pvecm status`)
- [ ] Monitor resource usage
- [ ] Review backup status
- [ ] Check for critical alerts

### Weekly Tasks
- [ ] Update package lists (`apt update`)
- [ ] Review storage usage
- [ ] Clean old backups
- [ ] Check Ceph health
- [ ] Review security logs

### Monthly Tasks
- [ ] Apply security updates
- [ ] Review and rotate certificates
- [ ] Performance analysis
- [ ] Capacity planning review
- [ ] Backup/restore testing

### Quarterly Tasks
- [ ] Major version updates (plan carefully)
- [ ] Hardware health check
- [ ] Disaster recovery testing
- [ ] Security audit
- [ ] Documentation updates

## 🆘 Emergency Procedures

### Node Failure Recovery
```bash
# Single node failure
pvecm expected 1  # If losing quorum

# Fence failed node
pvecm add_node {new-node} # Add replacement
pvecm delnode {failed-node} # Remove failed node
```

### Split-Brain Recovery
```bash
# Determine which node to keep
pvecm expected 1
pvecm node --force # Force node to be master

# Rejoin other node
systemctl stop pve-cluster
rm -rf /var/lib/pve-cluster/cfs_lock
systemctl start pve-cluster
```

### Ceph Recovery
```bash
# Emergency Ceph restart
systemctl restart ceph.target

# Force cluster recovery (dangerous!)
ceph osd set noout
ceph osd set norebalance
# Fix issues
ceph osd unset noout
ceph osd unset norebalance
```

This operations guide provides comprehensive coverage of daily Proxmox management tasks while following the idempotency principles from your homelab memories - all operations are designed to be repeatable and safe.
