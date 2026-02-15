# Homelab Infrastructure Automation

Ansible-driven homelab with Docker services, GPU passthrough, and CI/CD automation.

---

## Quick Reference

| Host | IP | Purpose |
|------|----|---------|  
| node005 | 192.168.1.105 | Proxmox - Control VMs |
| node006 | 192.168.1.106 | Proxmox - Ocean VM |
| ocean | 192.168.1.143 | Docker services (30 cores, 256GB, RTX 3090) |
| dns01 | 192.168.1.2 | BIND DNS |
| pihole | 192.168.1.9 | DNS filtering |
| gitlab | 192.168.1.5 | CI/CD |
| gh-runner-01 | 192.168.1.250 | GitHub Actions runners |

---

## Documentation

| Topic | Link |
|-------|------|
| Getting Started | [docs/setup/getting-started.md](docs/setup/getting-started.md) |
| Architecture | [docs/architecture/overview.md](docs/architecture/overview.md) |
| Network | [docs/architecture/networking.md](docs/architecture/networking.md) |
| Proxmox | [docs/operations/proxmox.md](docs/operations/proxmox.md) |
| ZFS Storage | [docs/operations/zfs.md](docs/operations/zfs.md) |
| GPU Management | [docs/operations/gpu-management.md](docs/operations/gpu-management.md) |
| Dell Hardware | [docs/operations/dell-hardware.md](docs/operations/dell-hardware.md) |
| Troubleshooting | [docs/troubleshooting/common-issues.md](docs/troubleshooting/common-issues.md) |

---

## Deployment

### Full Infrastructure

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/00_site.yaml --ask-vault-pass
```

### By Layer

```bash
# Base system
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/01_base_system.yaml --ask-vault-pass

# Core infrastructure
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/02_core_infrastructure.yaml --ask-vault-pass

# Ocean services
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/03_ocean_services.yaml --ask-vault-pass
```

### Individual Service

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/plex.yaml --ask-vault-pass
```

### GitHub Actions

See [.github/SETUP.md](.github/SETUP.md) for CI/CD deployment.

---

## Project Structure

```text
homelab/
├── playbooks/
│   ├── 00_site.yaml              # Full deployment
│   ├── 01_base_system.yaml       # Base config
│   ├── 02_core_infrastructure.yaml
│   ├── 03_ocean_services.yaml    # All ocean services
│   └── individual/               # Per-service playbooks
├── inventories/
│   └── production/hosts.ini      # Host inventory
├── roles/                        # Ansible roles
├── vault/secrets.yaml            # Encrypted secrets
├── docs/                         # Documentation
└── .github/workflows/            # CI/CD
```

---

## Key Features

### GPU Acceleration

- RTX 3090 (24GB VRAM) passed through to ocean VM
- llama.cpp, ComfyUI, Plex hardware transcoding

### Storage

- ZFS raidz2 pool (data01) with 8x 12TB disks
- Services mount `/data01/services/`

### Monitoring

- Prometheus + Grafana
- NVIDIA DCGM exporter
- UnPoller for UniFi metrics

### External Access

- Cloudflare tunnels (no port forwarding)
- nginx reverse proxy

---

## Development

### Test Changes

```bash
# Syntax check
ansible-playbook --syntax-check \
  playbooks/individual/ocean/media/plex.yaml

# Dry run
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/plex.yaml --check
```

### View Logs

```bash
# Service logs
ssh terrac@192.168.1.143 "docker logs plex --tail 50"

# GPU status
ssh terrac@192.168.1.143 "nvidia-smi"
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for full guide.

---

## Related Documentation

- [playbooks/README.md](playbooks/README.md) - Playbook reference
- [docs/README.md](docs/README.md) - Full documentation index
- [.github/SETUP.md](.github/SETUP.md) - GitHub Actions setup

---

<!-- GitOps CI/CD Test - 2026-02-15 -->
_This change tests the end-to-end GitOps deployment cycle with health checks._