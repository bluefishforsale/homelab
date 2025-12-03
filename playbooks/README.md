# Ansible Playbooks

Master orchestration playbooks and individual service playbooks.

---

## Structure

```text
playbooks/
├── 00_site.yaml                    # Complete infrastructure
├── 01_base_system.yaml             # Base system config
├── 02_core_infrastructure.yaml     # Core services (DNS, DHCP, Docker)
├── 03_ocean_services.yaml          # Ocean server services
├── tasks/                          # Reusable task files
└── individual/
    ├── base/                       # Base system (packages, users, fail2ban)
    ├── core/                       # Core services (DNS, DHCP, Pi-hole)
    ├── infrastructure/             # Docker, Proxmox, exporters, runners
    └── ocean/                      # Ocean services (ai, media, monitoring)
```

---

## Usage

### Master Playbooks

```bash
# Complete infrastructure
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/00_site.yaml --ask-vault-pass

# Base system only
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/01_base_system.yaml --ask-vault-pass

# Core infrastructure
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/02_core_infrastructure.yaml --ask-vault-pass

# Ocean services
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/03_ocean_services.yaml --ask-vault-pass
```

### Individual Playbooks

```bash
# Deploy specific service
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/network/nginx_compose.yaml --ask-vault-pass

# Dry run
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/base/packages.yaml --check

# With tags
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/comfyui.yaml --tags models --ask-vault-pass

# Skip tags
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/comfyui.yaml --skip-tags models --ask-vault-pass
```

### GitHub Runners

```bash
ansible-playbook -i inventories/github_runners/hosts.ini \
  playbooks/individual/infrastructure/github_docker_runners.yaml --ask-vault-pass
```

---

## Organization

### Base System (`individual/base/`)

| Playbook | Description |
|----------|-------------|
| `packages.yaml` | Base package installation |
| `users.yaml` | User and group management |
| `fail2ban.yaml` | SSH brute force protection |
| `io_cpu_ups.yaml` | I/O, CPU, and UPS config |
| `tz_sysctl_udev.yaml` | Timezone, sysctl, udev |
| `logging.yaml` | Logging configuration |
| `unattended_upgrade.yaml` | Auto security updates |

### Core (`individual/core/`)

- **network/** - Bonding, ethtool, QoS
- **storage/** - ZFS configuration
- **services/** - DNS, DHCP, Pi-hole, GitLab

### Infrastructure (`individual/infrastructure/`)

- Docker CE installation
- NVIDIA GPU support
- Proxmox repository config
- Node exporters (Prometheus)
- GitHub Actions runners
- fail2ban exporter

### Ocean (`individual/ocean/`)

- **ai/** - llama.cpp, ComfyUI, Open WebUI
- **media/** - Plex, Tautulli, Sonarr, Radarr, Prowlarr
- **monitoring/** - Prometheus, Grafana
- **network/** - nginx, Cloudflare tunnel
- **services/** - Frigate, NextCloud

---

## Best Practices

1. **Always include inventory**: `-i inventories/production/hosts.ini`
2. **Always include vault**: `--ask-vault-pass` (most playbooks need it)
3. **Test first**: Use `--check` for dry runs
4. **Use tags**: `--tags` and `--skip-tags` for partial runs
5. **Limit hosts**: `-l ocean` to target specific hosts

---

## CI/CD Integration

```yaml
jobs:
  deploy:
    runs-on: [self-hosted, homelab, ansible]
    environment: Github Actions CI
    steps:
      - uses: actions/checkout@v4
      - name: Deploy service
        run: |
          ansible-playbook -i inventories/production/hosts.ini \
            playbooks/individual/ocean/network/nginx_compose.yaml
        env:
          ANSIBLE_VAULT_PASSWORD: ${{ secrets.ANSIBLE_VAULT_PASSWORD }}
```

---

## Related Documentation

- [Getting Started](../docs/setup/getting-started.md)
- [Troubleshooting](../docs/troubleshooting/common-issues.md)
- [DEVELOPMENT.md](../DEVELOPMENT.md)
