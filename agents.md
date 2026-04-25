# Homelab Infrastructure — Agent Reference

Ansible-driven homelab managing Docker services, GPU passthrough, and CI/CD across a multi-host Proxmox cluster.

---

## Documentation Index

### Getting Started

- **Quick Start** → [`docs/setup/getting-started.md`](docs/setup/getting-started.md)
- **macOS Development Setup** → [`docs/setup/macos-setup.md`](docs/setup/macos-setup.md)
- **Development Guide** → [`DEVELOPMENT.md`](DEVELOPMENT.md)

### Architecture

- **System Overview** → [`docs/architecture/overview.md`](docs/architecture/overview.md)
- **Network Design** → [`docs/architecture/networking.md`](docs/architecture/networking.md)
- **Ocean Services** → [`docs/architecture/ocean-services.md`](docs/architecture/ocean-services.md)
- **Deployment Flow** → [`docs/architecture/deployment-flow.md`](docs/architecture/deployment-flow.md)

### Operations

- **Proxmox Management** → [`docs/operations/proxmox.md`](docs/operations/proxmox.md)
- **ZFS Storage** → [`docs/operations/zfs.md`](docs/operations/zfs.md)
- **GPU Management** → [`docs/operations/gpu-management.md`](docs/operations/gpu-management.md)
- **Dell Hardware** → [`docs/operations/dell-hardware.md`](docs/operations/dell-hardware.md)

### Troubleshooting

- **Common Issues** → [`docs/troubleshooting/common-issues.md`](docs/troubleshooting/common-issues.md)

### CI/CD & Automation

- **GitHub Actions Workflows** → [`.github/workflows/`](.github/workflows/)
- **Playbook Documentation** → [`playbooks/README.md`](playbooks/README.md)

---

## Quick Reference

**Primary Host:** ocean (192.168.1.143)  
**Environment Setup:** `source .envrc`  
**Deploy All:** `ansible-playbook -i inventories/production/hosts.ini playbooks/00_site.yaml`  
**Validate:** `make validate`

---

## Hardware

- **node006** (Dell R720): 40 cores, 680GB RAM, 64TB ZFS, RTX 3090 → ocean VM
- **node005** (Dell R620): 56 cores, 128GB RAM → dns01, pihole, k8s, runners

## Grafana + MySQL Consolidated Stack

MySQL consolidated into Grafana docker-compose (MySQL only serves Grafana):

- **grafana_internal** network: Grafana ↔ MySQL (private, no host exposure)
- **web_proxy** network: nginx ↔ Grafana
- MySQL: percona/percona-server:5.7, 1 CPU, 1GB, buffer_pool=512M
- Storage: `/data01/services/grafana/{mysql-data,mysql-logs,mysql-conf,data,logs}/`
- Deploy: single playbook manages both containers
