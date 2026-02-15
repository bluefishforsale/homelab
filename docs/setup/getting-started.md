# Homelab Setup Guide

Complete setup guide for the homelab infrastructure.

---

## Quick Reference

| Host | IP | Purpose |
|------|----|---------|
| node005 | 192.168.1.105 | Proxmox (56 cores, 128GB RAM) |
| node006 | 192.168.1.106 | Proxmox (40 cores, 680GB RAM, RTX 3090) |
| ocean | 192.168.1.143 | Main services VM (media, AI, monitoring) |
| dns01 | 192.168.1.2 | BIND9 DNS |
| pihole | 192.168.1.9 | DNS filtering |
| gitlab | 192.168.1.5 | GitLab CI/CD |

---

## Prerequisites

### Hardware (Current Setup)

- **node006** (Dell R720): 40 cores, 680GB RAM, 64TB ZFS (8x12TB), RTX 3090, bonded 10G
- **node005** (Dell R620): 56 cores, 128GB RAM, 1TB SSD

### Software Prerequisites

- **Proxmox VE 8.x** on hypervisors
- **Ansible** on local workstation
- **SSH Keys** configured for all hosts

### Local Development Setup

```bash
# Clone repo and setup
git clone https://github.com/bluefishforsale/homelab.git
cd homelab
make setup
echo 'export PATH="$HOME/Library/Python/3.13/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

---

## Phase 1: Base Infrastructure

### Proxmox Installation

#### 1. Prepare Installation Media
```bash
# Download Proxmox VE ISO
wget https://www.proxmox.com/en/downloads

# Create bootable USB (Linux/macOS)
sudo dd if=proxmox-ve_8.1-2.iso of=/dev/sdX bs=1M status=progress
```

#### 2. Install Proxmox on Both Hosts
1. Boot from USB and follow installation wizard
2. Configure basic network settings:
   - **Host 1**: 192.168.1.106/24
   - **Host 2**: 192.168.1.107/24  
   - **Gateway**: 192.168.1.1
   - **DNS**: 192.168.1.1 (temporarily)

#### 3. Post-Installation Configuration (Ansible)

```bash
# Configure repos and remove subscription warning
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/proxmox_repos.yaml
```

See `docs/operations/proxmox.md` for manual setup.

### Network Configuration

#### 1. Configure Network Bonding
```bash
# Edit network configuration
nano /etc/network/interfaces

# Add bonding configuration
auto lo
iface lo inet loopback

iface eno1 inet manual
iface eno2 inet manual

auto bond0  
iface bond0 inet manual
    bond-slaves eno1 eno2
    bond-miimon 100
    bond-mode 802.3ad
    bond-xmit-hash-policy layer3+4

auto vmbr0
iface vmbr0 inet static
    address 192.168.1.106/24
    gateway 192.168.1.1
    bridge-ports bond0
    bridge-stp off
    bridge-fd 0

# Apply network changes
ifreload -a
```

#### 2. Test Connectivity
```bash
# Verify network connectivity  
ping 192.168.1.1
ping 8.8.8.8
ping google.com

# Check bond status
cat /proc/net/bonding/bond0
```

### Ceph Storage Setup

#### 1. Prepare Storage Devices
```bash
# List available disks
lsblk
fdisk -l

# Identify disks for Ceph (e.g., /dev/sdb, /dev/sdc, etc.)
# DO NOT use the boot disk!
```

#### 2. Initialize Ceph Cluster
```bash
# From Proxmox web interface:
# Datacenter > Ceph > Install Ceph
# Follow the wizard to install Ceph packages

# Or via command line:
apt install ceph ceph-mgr-modules-core -y
```

#### 3. Create Ceph Monitor
```bash
# Initialize first monitor (replace with your network)
ceph-deploy mon create-initial --cluster ceph --mon-hosts proxmox01:192.168.1.106
```

#### 4. Add OSDs (Object Storage Daemons)
```bash
# Add disks as OSDs (repeat for each disk)
ceph-deploy osd create proxmox01 --data /dev/sdb
ceph-deploy osd create proxmox01 --data /dev/sdc
# ... continue for all available disks

# Verify cluster health
ceph -s
ceph osd tree
```

### Core VM Creation

#### 1. Download VM Templates
```bash
# Download Ubuntu cloud image
wget https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img

# Create VM template
qm create 9000 --name ubuntu-template --memory 2048 --cores 2 --net0 virtio,bridge=vmbr0
qm importdisk 9000 jammy-server-cloudimg-amd64.img local-lvm
qm set 9000 --scsihw virtio-scsi-pci --scsi0 local-lvm:vm-9000-disk-0
qm set 9000 --boot c --bootdisk scsi0
qm set 9000 --serial0 socket --vga serial0
qm template 9000
```

#### 2. Create Core Service VMs
```bash
# DNS/DHCP VM
qm clone 9000 102 --name dns01 --storage local-lvm
qm set 102 --ipconfig0 ip=192.168.1.2/24,gw=192.168.1.1
qm set 102 --memory 2048 --cores 2

# Pi-hole VM  
qm clone 9000 109 --name pihole01 --storage local-lvm
qm set 109 --ipconfig0 ip=192.168.1.9/24,gw=192.168.1.1
qm set 109 --memory 2048 --cores 2

# Ocean (Docker host) VM
qm clone 9000 143 --name ocean --storage local-lvm  
qm set 143 --ipconfig0 ip=192.168.1.143/24,gw=192.168.1.1
qm set 143 --memory 16384 --cores 8
qm set 143 --scsi1 local-lvm:200  # Additional storage for Docker

# GitLab VM
qm clone 9000 150 --name gitlab --storage local-lvm
qm set 150 --ipconfig0 ip=192.168.1.150/24,gw=192.168.1.1  
qm set 150 --memory 8192 --cores 4
```

#### 3. Start Core VMs
```bash
# Start VMs in order
qm start 102  # DNS first
sleep 30
qm start 109  # Pi-hole  
qm start 143  # Ocean
qm start 150  # GitLab
```

---

### DNS & DHCP Services

```bash
# Deploy BIND DNS server
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/core/services/dns.yaml --ask-vault-pass

# Deploy DHCP with dynamic DNS
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/core/services/dhcp_ddns.yaml --ask-vault-pass
```

#### 3. Update Network to Use New DNS
```bash
# Update Proxmox hosts to use new DNS
echo "nameserver 192.168.1.2" > /etc/resolv.conf

# Test DNS resolution
nslookup google.com 192.168.1.2
dig @192.168.1.2 google.com
```

### Pi-hole Setup

```bash
# Deploy Pi-hole
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/core/services/pi_hole.yaml --ask-vault-pass

# Access: http://192.168.1.9/admin
```

### GitLab Setup

```bash
# Deploy GitLab
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/core/services/gitlab.yaml --ask-vault-pass

# Access: http://192.168.1.5 or http://gitlab.home
```

### Monitoring Setup

```bash
# Deploy Prometheus
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/monitoring/prometheus.yaml --ask-vault-pass

# Deploy Grafana with MySQL
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/monitoring/grafana_compose.yaml --ask-vault-pass

# Access Grafana: http://192.168.1.143:8910
# Login: admin / grafana (from vault)
```

---

## Phase 3: Ocean Services

### Docker & Infrastructure

```bash
# Deploy Docker
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/docker_ce.yaml

# Deploy nginx reverse proxy
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/network/nginx_compose.yaml --ask-vault-pass

# Deploy Cloudflare tunnel
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/network/cloudflared.yaml --ask-vault-pass
```

### Media Services

```bash
# Deploy Plex
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/plex.yaml --ask-vault-pass

# Deploy Tautulli (Plex monitoring)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/tautulli.yaml --ask-vault-pass

# Deploy Arr stack
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/prowlarr.yaml --ask-vault-pass
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/sonarr.yaml --ask-vault-pass
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/radarr.yaml --ask-vault-pass
```

**Access:**

- Plex: `http://192.168.1.143:32400`
- Tautulli: `http://192.168.1.143:8905`

### AI/ML Services

```bash
# Deploy llama.cpp (GPU LLM server)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/llamacpp.yaml --ask-vault-pass

# Deploy Open WebUI
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/open_webui.yaml --ask-vault-pass
```

**Access:**

- llama.cpp: `http://192.168.1.143:8080`
- Open WebUI: `http://192.168.1.143:3000`

### Other Services

```bash
# Frigate NVR
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/services/frigate.yaml --ask-vault-pass
```

### Full Ocean Deployment

Use the master playbook for complete ocean deployment:

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/03_ocean_services.yaml --ask-vault-pass
```

---

## Phase 4: Advanced Configuration

### Secrets Management

```bash
# Edit vault secrets
ansible-vault edit vault/secrets.yaml --ask-vault-pass

# View vault contents
ansible-vault view vault/secrets.yaml --ask-vault-pass
```

### GitHub Runners

```bash
# Deploy self-hosted GitHub Actions runners
ansible-playbook -i inventories/github_runners/hosts.ini \
  playbooks/individual/infrastructure/github_docker_runners.yaml --ask-vault-pass
```

---

## Verification

### Service Health Checks

```bash
# Plex
curl -sf http://192.168.1.143:32400/web/index.html > /dev/null && echo "Plex OK"

# Grafana
curl -sf http://192.168.1.143:8910 > /dev/null && echo "Grafana OK"

# Prometheus
curl -sf http://192.168.1.143:9090/-/healthy && echo "Prometheus OK"

# DNS
nslookup ocean.home 192.168.1.2
```

---

## Resources

- **Development Setup**: [DEVELOPMENT.md](/DEVELOPMENT.md)
- **Operations Guide**: [docs/operations/proxmox.md](/docs/operations/proxmox.md)
- **Troubleshooting**: [docs/troubleshooting/common-issues.md](/docs/troubleshooting/common-issues.md)
- **Playbooks README**: [playbooks/README.md](/playbooks/README.md)
