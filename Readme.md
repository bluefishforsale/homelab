# 🏠 Homelab Infrastructure Automation

A comprehensive, production-ready homelab built on enterprise automation principles with fully automated deployment, secrets management, and monitoring.

## 🎯 Vision & Goals

- **🤖 Fully Automated** - Infrastructure as Code with minimal manual intervention
- **🔐 SSH-Free Operations** - 99% of tasks handled through automation and web interfaces
- **🛡️ Security First** - Publicly signed certificates, encrypted secrets management
- **📝 Git-Driven** - All infrastructure and services managed through version control
- **📊 Observable** - Comprehensive logging and monitoring across all components
- **⚡ High Availability** - Resilient architecture with redundancy for critical services

## 🏗️ Architecture Overview

### Infrastructure Stack
- **Proxmox VE Cluster** - High-availability hypervisor with Ceph storage
- **Docker Services** - Media stack, AI/ML services, monitoring (Ocean host)
- **Kubernetes Cluster** - Scalable container orchestration (6-node cluster)
- **Core Network Services** - DNS (BIND), DHCP, Pi-hole ad-blocking
- **Automation Platform** - GitLab CI/CD with Ansible automation
- **Secure Access** - Cloudflare tunnels with zero-trust policies

### Key Services
| Service | Purpose | Access |
|---------|---------|---------|
| **Plex** | Media streaming server | https://plex.terrac.com |
| **Grafana** | Monitoring dashboards | https://grafana.terrac.com |
| **N8N** | Workflow automation | https://n8n.terrac.com |
| **Open WebUI** | AI chat interface | https://open-webui.terrac.com |
| **GitLab** | CI/CD and source control | http://gitlab.home |
| **Pi-hole** | Network ad-blocking | http://192.168.1.9/admin |

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

## 🚀 Quick Deployment

### Prerequisites
- 2x Dell PowerEdge servers (or similar)
- 64GB+ RAM per host recommended
- Multiple drives for Ceph storage
- 10GbE networking equipment

### Deployment Steps
```bash
# 1. Clone repository
git clone https://github.com/bluefishforsale/homelab.git
cd homelab

# 2. Set up vault password
echo "your-vault-password" > ~/.ansible_vault_pass
chmod 600 ~/.ansible_vault_pass
export ANSIBLE_VAULT_PASSWORD_FILE=~/.ansible_vault_pass

# 3. Deploy core infrastructure
ansible-playbook -i inventory.ini playbook_core_svc_00_dns.yaml
ansible-playbook -i inventory.ini playbook_core_svc_00_dhcp_ddns.yaml
ansible-playbook -i inventory.ini playbook_core_svc_00_pi-hole.yaml

# 4. Deploy Docker services
ansible-playbook -i inventory.ini playbook_ocean_plex.yaml
ansible-playbook -i inventory.ini playbook_ocean_n8n.yaml
ansible-playbook -i inventory.ini playbook_ocean_open_webui.yaml

# 5. Set up monitoring
ansible-playbook -i inventory.ini playbook_ocean_monitoring.yaml
```

## 🏗️ Infrastructure Components

### Physical Layer
- **Proxmox Host 1** (192.168.1.106) - Primary hypervisor
- **Proxmox Host 2** (192.168.1.107) - Secondary hypervisor  
- **Network Equipment** - 10GbE managed switches with LACP bonding

### Virtual Machines
- **DNS Server** (192.168.1.2) - BIND9 with dynamic DNS
- **Pi-hole** (192.168.1.9) - Network-wide ad blocking
- **Ocean** (192.168.1.143) - Docker services host with GPU
- **GitLab** (192.168.1.150) - CI/CD and source control
- **K8s Cluster** (192.168.1.101-103, 111-113) - 6-node Kubernetes

### Container Services (Ocean Host)
- **Media Stack**: Plex, Sonarr, Radarr, Tautulli, NZBGet, Overseerr
- **AI/ML Services**: N8N, Open WebUI, ComfyUI, llama.cpp
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
- **GitLab Integration** - Automated testing and deployment
- **Cloudflare Tunnels** - Secure access without port forwarding
- **Monitoring Integration** - Grafana dashboards for all components
- **Alerting** - Proactive notification of issues

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

## 🤝 Contributing

This homelab follows enterprise automation best practices:
- **All changes via Git** - No direct production modifications
- **Idempotent Playbooks** - Safe to run multiple times
- **Documentation First** - All procedures documented
- **Testing Required** - Validate changes in development

## 📖 Additional Resources

- **[Roadmap 2025](roadmap-2025.md)** - Future enhancement plans
- **[Proxmox Setup](readme_proxmox.md)** - Hardware-specific setup guide
- **[macOS Development](readme_OSX.md)** - Local development environment

---

**Built with ❤️ for learning, automation, and enterprise-grade homelab practices**