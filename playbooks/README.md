# Ansible Playbooks

This directory contains all Ansible playbooks organized into master orchestration playbooks and individual service playbooks.

## Structure

```
playbooks/
├── 00_site.yaml                    # Complete infrastructure deployment
├── 01_base_system.yaml             # Base system configuration
├── 02_core_infrastructure.yaml     # Core services (DNS, DHCP, Docker)
├── 03_ocean_services.yaml          # Ocean server application services
└── individual/                     # Granular playbooks
    ├── base/                       # Base system playbooks
    ├── core/                       # Core infrastructure playbooks
    ├── ocean/                      # Ocean server service playbooks
    └── infrastructure/             # General infrastructure playbooks
```

## Usage

### Master Playbooks (Recommended)

Run complete deployments using master playbooks:

```bash
# Deploy complete infrastructure
ansible-playbook playbooks/00_site.yaml

# Deploy only base system configuration
ansible-playbook playbooks/01_base_system.yaml

# Deploy only core infrastructure services
ansible-playbook playbooks/02_core_infrastructure.yaml

# Deploy only ocean server services
ansible-playbook playbooks/03_ocean_services.yaml
```

### Individual Playbooks

Run specific services when needed:

```bash
# Deploy specific service
ansible-playbook playbooks/individual/ocean/network/nginx_compose.yaml

# Deploy with specific inventory
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/comfyui.yaml

# Check mode (dry run)
ansible-playbook playbooks/individual/base/packages.yaml --check

# With tags
ansible-playbook playbooks/individual/ocean/ai/comfyui.yaml --tags models
```

### Multi-Environment

Target specific environments using inventory flags:

```bash
# Production (default)
ansible-playbook playbooks/00_site.yaml

# GitHub Runners
ansible-playbook -i inventories/github_runners/hosts.ini \
  playbooks/infrastructure/github_runners.yaml
```

## Organization

### Base System (`individual/base/`)
Foundation system configuration applied to all hosts:
- `io_cpu_ups.yaml` - I/O, CPU, and UPS configuration
- `packages.yaml` - Base package installation
- `users.yaml` - User and group management
- `tz_sysctl_udev_logging.yaml` - System settings
- `unattended_upgrade.yaml` - Automatic security updates

### Core Infrastructure (`individual/core/`)
Essential infrastructure services:
- **network/** - Network configuration (bonding, ethtool, QoS)
- **storage/** - Storage configuration (RAID, ZFS)
- **services/** - Core services (DNS, DHCP, Docker)

### Ocean Services (`individual/ocean/`)
Application services running on ocean server:
- **ai/** - AI/ML services (llama.cpp, ComfyUI, Open WebUI)
- **media/** - Media stack (Plex, Sonarr, Radarr, etc.)
- **monitoring/** - Monitoring (Prometheus, Grafana)
- **network/** - Network services (nginx, Cloudflare)
- **services/** - Other services (NextCloud, TinaCMS, etc.)

### Infrastructure (`individual/infrastructure/`)
General infrastructure playbooks:
- Docker installation
- NVIDIA GPU support
- Proxmox integration
- Node exporters
- NFS servers

## Best Practices

1. **Use Master Playbooks** for complete deployments
2. **Use Individual Playbooks** for specific service updates
3. **Always test with `--check`** before applying changes
4. **Use tags** to run specific tasks within playbooks
5. **Specify inventory** with `-i` when targeting non-default environments
6. **Run with vault** when deploying services with secrets: `--ask-vault-pass`

## Examples

### Complete Infrastructure Deployment
```bash
# Deploy everything
ansible-playbook playbooks/00_site.yaml

# With vault password prompt
ansible-playbook playbooks/00_site.yaml --ask-vault-pass

# Check mode to see what would change
ansible-playbook playbooks/00_site.yaml --check
```

### Individual Service Deployment
```bash
# Deploy nginx
ansible-playbook playbooks/individual/ocean/network/nginx_compose.yaml

# Deploy ComfyUI with models
ansible-playbook playbooks/individual/ocean/ai/comfyui.yaml

# Skip model downloads
ansible-playbook playbooks/individual/ocean/ai/comfyui.yaml --skip-tags models
```

### GitHub Actions Integration
```yaml
jobs:
  deploy:
    runs-on: [self-hosted, homelab, ansible]
    steps:
      - uses: actions/checkout@v4
      - name: Deploy service
        run: ansible-playbook playbooks/individual/ocean/network/nginx_compose.yaml
```

## See Also

- [Getting Started Guide](../docs/setup/getting-started.md)
- [Architecture Documentation](../docs/architecture/)
- [Troubleshooting Guide](../docs/troubleshooting/common-issues.md)
- [Vault Management](../vault/secrets.yaml.template)
