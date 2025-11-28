# 🏠 Homelab Infrastructure Automation

A bare-bones, semi-production like homelab built on FOSS principles with automated deployment, secrets management, and monitoring.

## 🎯 Vision & Goals

- **🤖 Fully Automated** - Infrastructure as Code with minimal manual intervention
- **🔐 SSH-Free Operations** - 99% of tasks handled through automation and web interfaces
- **🛡️ Security First** - Publicly signed certificates, encrypted secrets management
- **📝 Git-Driven** - All infrastructure and services managed through version control
- **📊 Observable** - Comprehensive logging and monitoring across all components
- **⚡ High Availability** - Resilient architecture with redundancy for critical services

## 🏗️ Architecture Overview

📋 **[View Visual Architecture Diagrams](docs/architecture/README.md)** - Complete infrastructure diagrams including physical layout, services, network topology, and deployment flow.

### Infrastructure Stack  
- **Ocean Server** - Ubuntu Docker host with ZFS storage and NVIDIA P2000 GPU
- **Proxmox Host** - Hypervisor running VMs (DNS, GitLab, Pi-hole)
- **Docker Services** - 20+ containerized services, primary workload platform
- **Core Network Services** - DNS (BIND), DHCP, Pi-hole ad-blocking
- **CI/CD Platform** - GitHub Actions with 4 self-hosted ephemeral runners
- **Secure Access** - Cloudflare tunnels with zero-trust policies
- **Automation** - 65+ idempotent Ansible playbooks

### Key Services
| Service | Purpose | Access |
|---------|---------|---------|
| **Plex** | Media streaming server | https://plex.home |
| **Grafana** | Monitoring dashboards | https://grafana.home |
| **Open WebUI** | AI chat interface | https://open-webui.home|
| **GitLab** | CI/CD and source control | http://gitlab.home |
| **Pi-hole** | Network ad-blocking | http://pihole.home/admin |

### Administration Endpoints
| Service | Purpose | Access |
|---------|---------|---------|

## 📚 Documentation

### Quick Start
- **[🚀 Getting Started](docs/setup/getting-started.md)** - Complete setup guide from bare metal to production
- **[🏗️ Architecture Overview](docs/architecture/overview.md)** - System design and component relationships
- **[🌐 Network Architecture](docs/architecture/networking.md)** - Network design, VLANs, and connectivity

### Operations Guides
- **[🖥️ Proxmox Operations](docs/operations/proxmox.md)** - VM management, storage, clustering
- **[💾 ZFS Storage](docs/operations/zfs.md)** - Storage management, snapshots, performance
- **[🎮 GPU Management](docs/operations/gpu-management.md)** - NVIDIA P2000 configuration and optimization
- **[🖥️ Dell Hardware](docs/operations/dell-hardware.md)** - Hardware monitoring, RAID, firmware updates

### Troubleshooting
- **[🔧 Common Issues](docs/troubleshooting/common-issues.md)** - Frequently encountered problems and solutions

### Secrets Management
- **[🔐 Secrets Operations](homelab_playbooks/SECRETS_MANAGEMENT.md)** - Vault operations, credential rotation, DR procedures

## 🚀 Deployment

### Prerequisites
- Dell PowerEdge R720 or similar hardware
- 64GB+ RAM recommended
- NVIDIA GPU for AI workloads (optional)
- 10GbE networking for storage

### Deployment Methods

#### Option 1: GitHub Actions (Recommended)

**For standard services:**
```
1. Go to Actions tab → "Deploy Ocean Service"
2. Select service from dropdown
3. Click "Run workflow"
4. Automated: Validation → Lint → Dry-run → Deploy
```

**For critical services (DNS/DHCP/Plex):**
```
1. Go to Actions tab → "Deploy Critical Service (Protected)"
2. Select critical service
3. Click "Run workflow"
4. Review validation and dry-run logs
5. Approve deployment manually
6. Automated: Backup → Deploy → Health check
```

See [GitHub Actions Setup Guide](.github/SETUP.md) for details.

#### Option 2: Local Ansible

```bash
# 1. Clone repository
git clone https://github.com/bluefishforsale/homelab.git
cd homelab

# 2. Deploy complete infrastructure
ansible-playbook playbooks/00_site.yaml --ask-vault-pass

# 3. Or deploy specific layers
ansible-playbook playbooks/01_base_system.yaml --ask-vault-pass
ansible-playbook playbooks/02_core_infrastructure.yaml --ask-vault-pass
ansible-playbook playbooks/03_ocean_services.yaml --ask-vault-pass

# 4. Or deploy individual services
ansible-playbook playbooks/individual/ocean/network/nginx_compose.yaml --ask-vault-pass
```

See [Playbooks README](playbooks/README.md) for complete usage guide.

## 🏗️ Infrastructure Components

### Physical Layer
- **Proxmox Host 1** (192.168.1.106) - Primary hypervisor
- **Proxmox Host 2** (192.168.1.107) - Secondary hypervisor  
- **Network Equipment** - 10GbE managed switches with LACP bonding

### Virtual Machines
- **DNS Server** (192.168.1.2) - BIND9 with dynamic DNS
- **Pi-hole** (192.168.1.9) - Network-wide ad blocking
- **Ocean** (192.168.1.143) - Docker services host with GPU
- **GitLab** (192.168.1.150) - Source control (optional)

### Container Services (Ocean Host)
- **Media Stack**: Plex, Sonarr, Radarr, Tautulli, NZBGet, Overseerr
- **AI/ML Services**: Open WebUI, ComfyUI, llama.cpp
- **Monitoring**: Grafana, Prometheus, cAdvisor
- **Development**: Portainer, MySQL

### Storage Architecture
- **Ceph Cluster** - Distributed storage with replication
- **ZFS** - High-performance local storage with snapshots
- **Backup Strategy** - Automated snapshots and off-site replication

## 🔧 Automation Features

### Infrastructure as Code
- **Ansible Playbooks** - 50+ idempotent playbooks for all services
- **Vault Secrets** - Encrypted credential management with rotation
- **Configuration Management** - Consistent, repeatable deployments
- **Disaster Recovery** - Automated backup and restore procedures

### CI/CD Pipeline
- **GitHub Actions** - Automated validation and deployment
- **Self-Hosted Runners** - 4 ephemeral Docker runners with Ansible
- **Multi-Stage Gates** - Validation → Lint → Dry-run → Deploy
- **Critical Service Protection** - Mandatory approval for DNS/DHCP/Plex
- **Cloudflare Tunnels** - Secure access without port forwarding
- **Health Checks** - Pre/post deployment verification

### GPU Acceleration
- **NVIDIA P2000** - CUDA acceleration for AI workloads
- **Docker GPU Runtime** - Container GPU access
- **Model Management** - Automated AI model downloading and management
- **Hardware Transcoding** - GPU-accelerated media processing

## 📊 Monitoring & Observability

### Metrics Collection
- **Prometheus** - Metrics aggregation and alerting
- **Grafana** - Visualization and dashboards
- **Node Exporters** - System-level metrics
- **GPU Monitoring** - NVIDIA metrics and performance

### Key Dashboards
- **Infrastructure Overview** - System health and resource usage
- **Service Health** - Application-specific monitoring
- **Network Performance** - Bandwidth and connectivity metrics
- **Storage Analytics** - Capacity planning and performance

## 🔐 Security & Access

### Zero-Trust Architecture
- **Cloudflare Access** - Identity-based application protection
- **SSH Key Management** - GitHub-based key distribution
- **Network Segmentation** - VLAN isolation and firewall rules
- **Encrypted Storage** - All secrets and sensitive data encrypted

### Certificate Management
- **Public Certificates** - Let's Encrypt integration
- **Automated Renewal** - No manual certificate management
- **TLS Everywhere** - End-to-end encryption for all services

## 🚨 Disaster Recovery

### Backup Strategy
- **Automated Snapshots** - Hourly ZFS and Ceph snapshots
- **Off-site Replication** - Encrypted backups to external storage
- **Configuration Backup** - All automation code in Git
- **Database Backups** - Regular dumps with retention policies

### Recovery Procedures
- **Infrastructure Recovery** - Rebuild from automation
- **Data Recovery** - Point-in-time restore from snapshots
- **Service Recovery** - Container and VM restoration
- **Network Recovery** - Automated DNS and routing restoration

## 📅 Maintenance

### Automated Tasks
- **Daily**: Health checks, backup verification, security updates
- **Weekly**: Performance analysis, capacity planning
- **Monthly**: Firmware updates, security audit
- **Quarterly**: Disaster recovery testing, documentation updates

## 👩‍💻 Developer Guide

### For Contributors & Developers

#### Making Changes

1. **Always test locally first**
   ```bash
   # Test syntax
   ansible-playbook --syntax-check playbooks/individual/ocean/media/sonarr.yaml
   
   # Test in dry-run mode
   ansible-playbook --check playbooks/individual/ocean/media/sonarr.yaml
   ```

2. **Commit and push changes**
   ```bash
   git add playbooks/
   git commit -m "Update sonarr configuration"
   git push origin main
   ```

3. **Automatic validation**
   - CI workflow automatically validates YAML, Ansible syntax, and lints
   - Check Actions tab for results
   - Pull requests must pass validation before merge

4. **Deploy via GitHub Actions**
   - Use workflows for deployment (not direct ansible commands)
   - Standard services: Deploy Ocean Service workflow
   - Critical services: Deploy Critical Service workflow

#### Safety Rules

⚠️ **CRITICAL INFRASTRUCTURE** (requires approval):
- DNS (192.168.1.2) - Network foundation
- DHCP (192.168.1.2) - Address management
- Plex (192.168.1.143:32400) - Primary media service

✅ **Standard services** (automated deployment):
- All other ocean services (nginx, sonarr, AI services, etc.)

#### Best Practices

1. **Idempotency** - All playbooks must be safe to run multiple times
2. **No `ignore_errors`** - Let failures fail (see [Error Handling Audit](.github/PLAYBOOK_ERROR_HANDLING_AUDIT.md))
3. **Dry-run testing** - Always test with `--check` first
4. **Documentation** - Update docs when changing functionality
5. **Fail-fast** - Exit immediately on errors (`set -e` in scripts)
6. **Health checks** - Add verification for critical services

#### Workflow Structure

```
Every deployment:
  ├─ Validate YAML syntax
  ├─ Validate Ansible syntax  
  ├─ Lint with ansible-lint
  ├─ Dry-run in check mode (if not already check mode)
  └─ Deploy (only if all gates pass)

Critical services add:
  ├─ Pre-deployment health check
  ├─ Manual approval gate
  ├─ Configuration backup
  └─ Post-deployment health verification
```

#### Key Documentation

- **[GitHub Actions Setup](.github/SETUP.md)** - CI/CD configuration
- **[Safety Guide](.github/SAFETY.md)** - Critical infrastructure protection
- **[Error Handling Audit](.github/PLAYBOOK_ERROR_HANDLING_AUDIT.md)** - Error suppression review
- **[Workflow README](.github/workflows/README.md)** - Workflow usage guide
- **[Playbooks README](playbooks/README.md)** - Ansible playbook reference

#### Common Tasks

**Add a new service:**
```bash
# 1. Create playbook
cp playbooks/individual/ocean/media/sonarr.yaml playbooks/individual/ocean/media/newservice.yaml

# 2. Update service configuration
# Edit docker-compose template, systemd service, etc.

# 3. Add to deploy-ocean-service.yml workflow
# Edit .github/workflows/deploy-ocean-service.yml
# Add service to dropdown and case statement

# 4. Test locally
ansible-playbook --check playbooks/individual/ocean/media/newservice.yaml

# 5. Commit and deploy via workflow
```

**Update existing service:**
```bash
# 1. Make changes to playbook/templates
# 2. Test locally with --check
# 3. Push changes
# 4. CI validates automatically
# 5. Deploy via GitHub Actions workflow
```

**Troubleshooting:**
```bash
# Check workflow logs in GitHub Actions tab
# View service logs on target host
ssh ocean "docker logs servicename"

# Test playbook locally with verbose output
ansible-playbook -vvv playbooks/individual/ocean/media/sonarr.yaml
```

### Code Quality

- ✅ All playbooks pass `ansible-lint`
- ✅ All YAML passes syntax validation
- ✅ No hardcoded secrets (use vault)
- ✅ Idempotent operations only
- ✅ Proper error handling (no silent failures)

## 📖 Additional Resources

- **[Roadmap 2025](roadmap-2025.md)** - Future enhancement plans
- **[Proxmox Setup](readme_proxmox.md)** - Hardware-specific setup guide
- **[macOS Development](readme_OSX.md)** - Local development environment

---

**Built with ❤️ for learning, automation, and enterprise-grade homelab practices**