# 📚 Homelab Documentation

Welcome to the comprehensive documentation for this enterprise-grade homelab infrastructure.

## 🚀 Getting Started

New to this homelab? Start here:

- **[🚀 Getting Started Guide](setup/getting-started.md)** - Complete deployment from bare metal to production
- **[🏗️ Architecture Overview](architecture/overview.md)** - Understand the system design and components
- **[🌐 Network Architecture](architecture/networking.md)** - Network topology, VLANs, and connectivity

## 📖 Documentation Structure

### Architecture & Design
- **[System Overview](architecture/overview.md)** - High-level architecture and component relationships
- **[Network Design](architecture/networking.md)** - Network topology, security, and performance

### Setup & Installation
- **[Getting Started](setup/getting-started.md)** - Step-by-step deployment guide
- **[Prerequisites](setup/getting-started.md#prerequisites)** - Hardware and software requirements

### Operations Guides
- **[Proxmox Operations](operations/proxmox.md)** - VM management, clustering, storage
- **[ZFS Storage Management](operations/zfs.md)** - Storage operations, snapshots, performance
- **[GPU Management](operations/gpu-management.md)** - NVIDIA GPU configuration and optimization
- **[Dell Hardware](operations/dell-hardware.md)** - Hardware monitoring, RAID, firmware
- **[Networking Operations](../homelab_playbooks/NETWORKING.md)** - Network troubleshooting and optimization
- **[Secrets Management](../homelab_playbooks/SECRETS_MANAGEMENT.md)** - Vault operations and credential management

### Troubleshooting
- **[Common Issues](troubleshooting/common-issues.md)** - Frequently encountered problems and solutions
- **[Emergency Procedures](troubleshooting/common-issues.md#emergency-recovery-procedures)** - Disaster recovery and emergency response

## 🎯 Quick Navigation

### By Use Case

#### 🔧 System Administration
- [Proxmox cluster management](operations/proxmox.md)
- [Storage administration](operations/zfs.md)
- [Hardware monitoring](operations/dell-hardware.md)
- [Network configuration](architecture/networking.md)

#### 🐳 Container Operations
- [Docker service management](operations/proxmox.md#container-management)
- [GPU acceleration](operations/gpu-management.md)
- [Service deployment](../homelab_playbooks/SECRETS_MANAGEMENT.md)

#### 🚨 Emergency Response
- [System recovery procedures](troubleshooting/common-issues.md#emergency-recovery-procedures)
- [Disaster recovery](troubleshooting/common-issues.md#data-recovery)
- [Performance troubleshooting](troubleshooting/common-issues.md#performance-issues)

#### 🔐 Security Operations
- [Secrets management](../homelab_playbooks/SECRETS_MANAGEMENT.md)
- [Access control](architecture/networking.md#security-architecture)
- [Certificate management](troubleshooting/common-issues.md#ssltls-certificate-issues)

### By Component

#### Infrastructure
- **Proxmox**: [Architecture](architecture/overview.md#infrastructure-layers) | [Operations](operations/proxmox.md) | [Troubleshooting](troubleshooting/common-issues.md#virtualization-issues)
- **Ceph Storage**: [Architecture](architecture/overview.md#storage-architecture) | [Operations](operations/proxmox.md#ceph-storage-operations) | [Troubleshooting](troubleshooting/common-issues.md#ceph-storage-issues)
- **ZFS**: [Operations](operations/zfs.md) | [Troubleshooting](troubleshooting/common-issues.md#zfs-problems)

#### Networking  
- **Core Network**: [Architecture](architecture/networking.md) | [Operations](operations/proxmox.md#network-operations) | [Troubleshooting](troubleshooting/common-issues.md#network-issues)
- **Cloudflare**: [Architecture](architecture/overview.md#secure-access) | [Troubleshooting](troubleshooting/common-issues.md#cloudflare-tunnel-issues)

#### Hardware
- **Dell Servers**: [Operations](operations/dell-hardware.md) | [Troubleshooting](troubleshooting/common-issues.md#hardware-diagnostics)
- **NVIDIA GPU**: [Operations](operations/gpu-management.md) | [Troubleshooting](troubleshooting/common-issues.md#gpu-issues)

#### Services
- **GitLab**: [Setup](setup/getting-started.md#gitlab-setup) | [Troubleshooting](troubleshooting/common-issues.md#gitlab-issues)
- **Media Stack**: [Setup](setup/getting-started.md#media-services) | [Troubleshooting](troubleshooting/common-issues.md#plex-issues)
- **AI/ML Services**: [Setup](setup/getting-started.md#ai-ml-services) | [GPU Config](operations/gpu-management.md#service-specific-gpu-configuration)

## 📊 Service Access

### Web Interfaces
- **Grafana**: https://grafana.terrac.com - Monitoring dashboards
- **Plex**: https://plex.terrac.com - Media streaming
- **N8N**: https://n8n.terrac.com - Workflow automation  
- **Open WebUI**: https://open-webui.terrac.com - AI chat interface
- **Pi-hole**: http://192.168.1.9/admin - DNS ad-blocking
- **Proxmox**: https://192.168.1.106:8006 - Virtualization management

### SSH Access
- **Ocean**: `ssh terrac@192.168.1.143` - Docker services host
- **GitLab**: `ssh root@192.168.1.150` - CI/CD server
- **DNS**: `ssh root@192.168.1.2` - DNS/DHCP server

## 🔧 Common Tasks

### Daily Operations
```bash
# Check system health
ansible-playbook -i inventory.ini playbook_health_check.yaml

# View service status
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# Check storage usage
zfs list -o space
ceph df
```

### Deployment Operations
```bash
# Deploy specific service
ansible-playbook -i inventory.ini playbook_ocean_service.yaml --ask-vault-pass

# Update all services
ansible-playbook -i inventory.ini playbook_update_all.yaml --ask-vault-pass

# Check deployment status
ansible-playbook -i inventory.ini playbook_health_check.yaml --check
```

### Troubleshooting Commands
```bash
# Check logs
journalctl -f
docker logs container-name

# Network diagnostics
ping -c 4 google.com
nslookup service.home 192.168.1.2

# Storage diagnostics
zpool status
ceph health detail
```

## 🆘 Emergency Contacts

### Quick Reference
- **Vault Password**: Stored in password manager
- **Admin Credentials**: See encrypted vault_secrets.yaml
- **Service URLs**: Listed in [Service Access](#service-access) section

### Emergency Procedures
1. **System Down**: [Emergency Recovery](troubleshooting/common-issues.md#emergency-recovery-procedures)
2. **Network Issues**: [Network Troubleshooting](troubleshooting/common-issues.md#network-issues)
3. **Storage Failure**: [Storage Recovery](troubleshooting/common-issues.md#storage-issues)
4. **Service Issues**: [Service-Specific Troubleshooting](troubleshooting/common-issues.md#service-specific-issues)

## 📅 Maintenance Schedule

### Daily
- [ ] Check system alerts in Grafana
- [ ] Verify backup completion
- [ ] Review service logs for errors

### Weekly  
- [ ] Update operating systems
- [ ] Check storage capacity
- [ ] Review performance metrics

### Monthly
- [ ] Update firmware and BIOS
- [ ] Rotate credentials
- [ ] Test disaster recovery procedures

## 🤝 Contributing to Documentation

### Documentation Standards
- **Markdown Format**: All documentation in Markdown
- **Consistent Structure**: Follow existing templates
- **Code Examples**: Include working command examples
- **Screenshots**: When helpful for UI-based tasks

### Update Procedures
1. Edit documentation files
2. Test all command examples
3. Update relevant cross-references
4. Commit changes with descriptive messages

---

**This documentation follows enterprise standards for maintainability, searchability, and operational excellence.**
