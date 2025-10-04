# 🚀 Homelab Setup Guide

## Prerequisites

Before starting your homelab deployment, ensure you have the following prerequisites in place.

### Hardware Requirements

#### Minimum Configuration
- **2x Physical Servers** - For Proxmox HA cluster
- **8GB+ RAM per host** - More RAM = more VMs
- **SSD Storage** - At least 100GB for host OS
- **Additional Storage** - HDDs/SSDs for Ceph cluster
- **Network Equipment** - Managed switch with VLAN support
- **10GbE NICs** - Recommended for bonding and performance

#### Recommended Configuration
- **2x Dell PowerEdge R720/R730** - Proven homelab platforms
- **64GB+ RAM per host** - Supports substantial VM workloads  
- **2x 10GbE NICs** - For bonded network connectivity
- **Multiple drives** - Mix of SSDs and HDDs for tiered storage
- **UPS** - Uninterruptible power for graceful shutdowns

### Software Prerequisites
- **Proxmox VE** - Latest stable version
- **Git** - For cloning this repository
- **Ansible** - Configuration management (installed locally)
- **SSH Keys** - For secure server access

### Network Prerequisites
- **Static IP Range** - 192.168.1.0/24 recommended
- **Internet Access** - For package downloads and external services
- **Domain Registration** - For Cloudflare integration (optional)

## 📋 Installation Roadmap

### Phase 1: Base Infrastructure 
1. [Proxmox Installation](#proxmox-installation)
2. [Network Configuration](#network-configuration)  
3. [Ceph Storage Setup](#ceph-storage-setup)
4. [Core VM Creation](#core-vm-creation)

### Phase 2: Core Services
1. [DNS & DHCP Services](#dns--dhcp-services)
2. [Pi-hole Ad Blocking](#pi-hole-setup)
3. [GitLab CI/CD Platform](#gitlab-setup)
4. [Monitoring Stack](#monitoring-setup)

### Phase 3: Container Platforms
1. [Docker Host Setup](#docker-host-setup)
2. [Media Services Stack](#media-services)
3. [AI/ML Services](#ai-ml-services)
4. [Kubernetes Cluster](#kubernetes-cluster)

### Phase 4: Advanced Services
1. [Cloudflare Tunnels](#cloudflare-tunnels)
2. [Secrets Management](#secrets-management)
3. [Backup & Monitoring](#backup--monitoring)
4. [Documentation & Operations](#documentation)

---

## 🏗️ Phase 1: Base Infrastructure

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

#### 3. Post-Installation Configuration
```bash
# SSH to first Proxmox host
ssh root@192.168.1.106

# Update system
apt update && apt upgrade -y

# Remove enterprise repo (unless you have subscription)
echo "#deb https://enterprise.proxmox.com/debian/pve bookworm pve-enterprise" > /etc/apt/sources.list.d/pve-enterprise.list

# Add community repo
echo "deb http://download.proxmox.com/debian/pve bookworm pve-no-subscription" >> /etc/apt/sources.list.d/pve-no-subscription.list
apt update
```

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

## 🔧 Phase 2: Core Services

### DNS & DHCP Services

#### 1. Clone Homelab Repository
```bash
# On your workstation
git clone https://github.com/bluefishforsale/homelab.git
cd homelab

# Install Ansible
pip install ansible

# Test connectivity to VMs
ansible -i inventory.ini dns -m ping
```

#### 2. Deploy DNS Services
```bash
# Deploy BIND DNS server
ansible-playbook -i inventory.ini playbook_core_svc_00_dns.yaml

# Deploy DHCP server  
ansible-playbook -i inventory.ini playbook_core_svc_00_dhcp_ddns.yaml

# Verify services
ansible dns -i inventory.ini -a "systemctl status bind9"
ansible dns -i inventory.ini -a "systemctl status isc-dhcp-server"
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

#### 1. Deploy Pi-hole
```bash
# Deploy Pi-hole ad-blocking DNS
ansible-playbook -i inventory.ini playbook_core_svc_00_pi-hole.yaml

# Access web interface: http://192.168.1.9/admin
# Default password will be shown in playbook output
```

#### 2. Configure DHCP to Use Pi-hole
```bash
# Update DHCP configuration to serve Pi-hole as primary DNS
ansible-vault edit vault_secrets.yaml
# Update DNS settings to point to Pi-hole

# Redeploy DHCP configuration
ansible-playbook -i inventory.ini playbook_core_svc_00_dhcp_ddns.yaml
```

### GitLab Setup

#### 1. Deploy GitLab
```bash
# Set up vault password
echo "your-vault-password" > ~/.ansible_vault_pass
chmod 600 ~/.ansible_vault_pass
export ANSIBLE_VAULT_PASSWORD_FILE=~/.ansible_vault_pass

# Deploy GitLab
ansible-playbook -i inventory.ini playbook_gitlab_packages.yaml

# Access GitLab: http://gitlab.home
# Login: root / admin (default from vault)
```

#### 2. Configure GitLab
1. Access GitLab web interface
2. Change admin password
3. Configure SSH keys
4. Create project for homelab repository
5. Set up CI/CD variables

### Monitoring Setup

#### 1. Deploy Monitoring Stack
```bash
# Deploy MySQL database
ansible-playbook -i inventory.ini playbook_ocean_mysql.yaml

# Deploy Grafana and Prometheus
ansible-playbook -i inventory.ini playbook_ocean_monitoring.yaml

# Access Grafana: http://192.168.1.143:8910
# Login: admin / grafana (from vault)
```

---

## 🐳 Phase 3: Container Platforms

### Docker Host Setup

#### 1. Prepare Ocean Host
```bash
# Deploy base Docker configuration
ansible-playbook -i inventory.ini playbook_ocean_base.yaml

# Verify Docker installation
ansible ocean -i inventory.ini -a "docker --version"
ansible ocean -i inventory.ini -a "docker-compose --version"
```

### Infrastructure Services

#### 1. Deploy Base Infrastructure
```bash

# Deploy MySQL database
ansible-playbook -i inventory.ini playbook_ocean_mysql.yaml

# Deploy Nginx reverse proxy
ansible-playbook -i inventory.ini playbook_ocean_nginx.yaml
```

#### 2. Deploy Network Services
```bash
# Deploy Cloudflare DDNS
ansible-playbook -i inventory.ini playbook_ocean_cloudflare_ddns.yaml

# Deploy Cloudflare tunnels and Access
ansible-playbook -i inventory.ini playbook_ocean_cloudflared.yaml
```

### Media Services

#### 1. Deploy Core Media Stack
```bash
# Deploy Plex media server
ansible-playbook -i inventory.ini playbook_ocean_plex.yaml

# Deploy download client
ansible-playbook -i inventory.ini playbook_ocean_nzbget.yaml

# Deploy media management suite (Arr stack)
ansible-playbook -i inventory.ini playbook_ocean_prowlarr.yaml  # Indexer management
ansible-playbook -i inventory.ini playbook_ocean_sonarr.yaml    # TV shows
ansible-playbook -i inventory.ini playbook_ocean_radarr.yaml    # Movies
ansible-playbook -i inventory.ini playbook_ocean_bazarr.yaml    # Subtitles

# Access services:
# Plex: http://192.168.1.143:32400
# NZBGet: http://192.168.1.143:6789
# Prowlarr: http://192.168.1.143:9696
# Sonarr: http://192.168.1.143:8902
# Radarr: http://192.168.1.143:7878
# Bazarr: http://192.168.1.143:6767
```

#### 2. Deploy Media Enhancement Services
```bash
# Deploy Plex monitoring
ansible-playbook -i inventory.ini playbook_ocean_tautulli.yaml

# Deploy request management
ansible-playbook -i inventory.ini playbook_ocean_overseerr.yaml

# Deploy transcoding optimization
ansible-playbook -i inventory.ini playbook_ocean_tdarr.yaml

# Deploy audiobook downloader
ansible-playbook -i inventory.ini playbook_ocean_audible-downloader.yaml

# Access services:
# Tautulli: http://192.168.1.143:8905  
# Overseerr: http://192.168.1.143:5055
# Tdarr: http://192.168.1.143:8265
# Audible Downloader: http://192.168.1.143:8080
```

### AI/ML Services

#### 1. Deploy AI Stack
```bash
# Deploy N8N workflow automation
ansible-playbook -i inventory.ini playbook_ocean_n8n.yaml

# Deploy llama.cpp API server (GPU-accelerated LLM)
ansible-playbook -i inventory.ini playbook_ocean_llamacpp.yaml

# Deploy Open WebUI for LLM interaction
ansible-playbook -i inventory.ini playbook_ocean_open_webui.yaml

# Deploy ComfyUI for AI image generation
ansible-playbook -i inventory.ini playbook_ocean_comfyui.yaml

# Access services:
# N8N: http://192.168.1.143:5678
# Llama.cpp API: http://192.168.1.143:8080
# Open WebUI: http://192.168.1.143:3000
# ComfyUI: http://192.168.1.143:8188
```

### Monitoring Services

#### 1. Deploy Monitoring Stack
```bash
# Deploy Prometheus metrics collection
ansible-playbook -i inventory.ini playbook_ocean_prometheus.yaml

# Deploy Grafana dashboards
ansible-playbook -i inventory.ini playbook_ocean_grafana.yaml

# Access services:
# Prometheus: http://192.168.1.143:9090
# Grafana: http://192.168.1.143:3001
```

### Service Deployment Order

For a complete deployment, run services in this recommended order:

#### Phase 1: Infrastructure Foundation
```bash
ansible-playbook -i inventory.ini playbook_ocean_base.yaml
ansible-playbook -i inventory.ini playbook_ocean_data01_virtio.yaml
ansible-playbook -i inventory.ini playbook_ocean_mysql.yaml
ansible-playbook -i inventory.ini playbook_ocean_nginx.yaml
```

#### Phase 2: Network Services
```bash
ansible-playbook -i inventory.ini playbook_ocean_cloudflare_ddns.yaml
ansible-playbook -i inventory.ini playbook_ocean_cloudflared.yaml
```

#### Phase 3: Media Stack
```bash
ansible-playbook -i inventory.ini playbook_ocean_plex.yaml
ansible-playbook -i inventory.ini playbook_ocean_nzbget.yaml
ansible-playbook -i inventory.ini playbook_ocean_prowlarr.yaml
ansible-playbook -i inventory.ini playbook_ocean_sonarr.yaml
ansible-playbook -i inventory.ini playbook_ocean_radarr.yaml
ansible-playbook -i inventory.ini playbook_ocean_bazarr.yaml
ansible-playbook -i inventory.ini playbook_ocean_tautulli.yaml
ansible-playbook -i inventory.ini playbook_ocean_overseerr.yaml
```

#### Phase 4: AI/ML Services
```bash
ansible-playbook -i inventory.ini playbook_ocean_llamacpp.yaml
ansible-playbook -i inventory.ini playbook_ocean_open_webui.yaml
ansible-playbook -i inventory.ini playbook_ocean_n8n.yaml
ansible-playbook -i inventory.ini playbook_ocean_comfyui.yaml
```

#### Phase 5: Optional Services
```bash
ansible-playbook -i inventory.ini playbook_ocean_tdarr.yaml              # Transcoding
ansible-playbook -i inventory.ini playbook_ocean_audible-downloader.yaml # Audiobooks
ansible-playbook -i inventory.ini playbook_ocean_prometheus.yaml         # Monitoring
ansible-playbook -i inventory.ini playbook_ocean_grafana.yaml           # Dashboards
```

### Kubernetes Cluster

#### 1. Create Kubernetes VMs
```bash
# Create 6 VMs for Kubernetes cluster
for i in {101..103}; do
  qm clone 9000 $i --name k8s-master0$(($i-100)) --storage local-lvm
  qm set $i --ipconfig0 ip=192.168.1.$i/24,gw=192.168.1.1
  qm set $i --memory 4096 --cores 2
done

for i in {111..113}; do  
  qm clone 9000 $i --name k8s-worker0$(($i-110)) --storage local-lvm
  qm set $i --ipconfig0 ip=192.168.1.$i/24,gw=192.168.1.1
  qm set $i --memory 8192 --cores 4
done

# Start all Kubernetes VMs
for i in {101..103} {111..113}; do qm start $i; done
```

#### 2. Deploy Kubernetes
```bash
# Deploy base packages and configuration
ansible-playbook -i inventory.ini -l k8s playbook_base_packages.yaml
ansible-playbook -i inventory.ini -l k8s playbook_base_host_settings.yaml
ansible-playbook -i inventory.ini -l k8s playbook_base_users.yaml

# Deploy Kubernetes step by step
ansible-playbook -i inventory.ini playbook_kube_00.1_gen_pki.yaml
ansible-playbook -i inventory.ini playbook_kube_00.2_gen_kubeconfigs.yaml
# ... continue with remaining kube playbooks in sequence

# Or deploy all at once (not recommended for first deployment)
ls -1 playbook_kube_* | xargs -n1 -I% ansible-playbook -i inventory.ini %
```

---

## 🌐 Phase 4: Advanced Services  

### Cloudflare Tunnels

#### 1. Configure Cloudflare
```bash
# Deploy cloudflared tunnels
ansible-playbook -i inventory.ini playbook_ocean_cloudflared.yaml

# Services will be accessible via:
# https://service-name.your-domain.com
```

### Secrets Management

#### 1. Encrypt Vault
```bash
# Your secrets are already populated in vault_secrets.yaml
# Encrypt the vault file
ansible-vault encrypt vault_secrets.yaml

# Test vault access  
ansible-vault view vault_secrets.yaml
```

### Backup & Monitoring

#### 1. Configure Backups
```bash
# Set up automated backup scripts
ansible-playbook -i inventory.ini playbook_backup_configuration.yaml

# Configure Prometheus monitoring
ansible-playbook -i inventory.ini playbook_monitoring_complete.yaml
```

---

## ✅ Verification & Testing

### Service Health Checks
```bash
# Test all major services
curl -f http://192.168.1.143:32400/web/index.html    # Plex
curl -f http://192.168.1.143:8905                    # Tautulli  
curl -f http://192.168.1.143:8910                    # Grafana
curl -f http://192.168.1.143:3000                    # Open WebUI

# Test DNS resolution
nslookup gitlab.home 192.168.1.2
nslookup ocean.home 192.168.1.9

# Test Kubernetes cluster
kubectl get nodes
kubectl get pods --all-namespaces
```

### Performance Validation
```bash
# Network performance
iperf3 -c 192.168.1.143 -t 30

# Storage performance  
dd if=/dev/zero of=/tmp/testfile bs=1G count=1 oflag=direct
```

## 🎉 Completion

Congratulations! Your homelab should now be fully operational with:

- ✅ **High-availability Proxmox cluster** with Ceph storage
- ✅ **Core network services** (DNS, DHCP, Pi-hole)  
- ✅ **CI/CD platform** (GitLab)
- ✅ **Container platforms** (Docker + Kubernetes)
- ✅ **Media services** (Plex ecosystem)
- ✅ **AI/ML services** (N8N, Open WebUI, llama.cpp)
- ✅ **Monitoring & observability** (Grafana, Prometheus)
- ✅ **Secure remote access** (Cloudflare tunnels)
- ✅ **Automated secrets management** (Ansible Vault)

## 📚 Next Steps

1. Review the [Operations Guide](../operations/README.md) for day-to-day management
2. Explore service-specific documentation in the `docs/operations/` directory  
3. Set up additional services based on your needs
4. Configure automated backups and disaster recovery procedures
5. Implement advanced monitoring and alerting

Your homelab is now ready for production use with enterprise-grade automation and monitoring capabilities!
