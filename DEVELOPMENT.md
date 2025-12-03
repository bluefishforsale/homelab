# Development Setup

Local development environment setup for the homelab Ansible repository.

---

## Quick Start

```bash
# One-time setup
make setup

# Add Python bin directory to PATH
echo 'export PATH="$HOME/Library/Python/3.13/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc

# Run all validation checks
make validate
```

---

## Prerequisites

- **macOS** with Homebrew
- **Python 3** (comes with macOS)
- **Homebrew**: <https://brew.sh>

---

## Repository Structure

```text
homelab/
├── playbooks/
│   ├── 00_site.yaml              # Complete infrastructure
│   ├── 01_base_system.yaml       # Base system config
│   ├── 02_core_infrastructure.yaml   # Core services
│   ├── 03_ocean_services.yaml    # Ocean server services
│   └── individual/               # Individual service playbooks
│       ├── base/                 # Base system playbooks
│       ├── infrastructure/       # Core infrastructure (docker, github runners)
│       └── ocean/                # Ocean server services
├── inventories/
│   ├── production/hosts.ini      # Production inventory
│   └── github_runners/hosts.ini  # GitHub runners inventory
├── roles/                        # Ansible roles
├── files/                        # Service configuration files
├── vault/                        # Encrypted secrets (ansible-vault)
└── docs/operations/              # Operations documentation
```

---

## Make Targets

### Setup

| Target | Description |
|--------|-------------|
| `make setup` | Install all dependencies (Homebrew + Python) |
| `make setup-brew` | Install Homebrew packages (ansible) |
| `make setup-python` | Install Python packages from requirements.txt |

### Validation

| Target | Description |
|--------|-------------|
| `make validate` | Run all validation checks |
| `make validate-yaml` | Validate YAML syntax |
| `make validate-ansible` | Validate Ansible playbook syntax |
| `make validate-templates` | Validate Jinja2 templates |
| `make security-scan` | Scan for hardcoded secrets |
| `make check-vault` | Verify vault files are encrypted |
| `make lint-ansible` | Lint Ansible playbooks (optional) |

### Utility

| Target | Description |
|--------|-------------|
| `make clean` | Clean temporary files |
| `make help` | Show all available targets |

---

## Dependencies

### Homebrew Packages

- `ansible` - Automation tool

### Python Packages (requirements.txt)

- `pyyaml>=6.0` - YAML parsing
- `jinja2>=3.1` - Template validation
- `ansible>=2.15` - Playbook execution

---

## PATH Configuration

```bash
# For zsh (default on macOS)
echo 'export PATH="$HOME/Library/Python/3.13/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc

# Check Python version if 3.13 doesn't match
python3 --version
```

---

## Ansible Vault

### Option 1: Interactive prompt (recommended)

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/00_site.yaml --ask-vault-pass
```

### Option 2: Environment variable

```bash
export ANSIBLE_VAULT_PASSWORD="your-vault-password"
ansible-playbook -i inventories/production/hosts.ini playbooks/00_site.yaml
```

### Vault file location

Secrets stored in `vault/secrets.yaml` (encrypted).

```bash
# Edit vault
ansible-vault edit vault/secrets.yaml

# Encrypt new file
ansible-vault encrypt vault/new-secrets.yaml

# View encrypted file
ansible-vault view vault/secrets.yaml
```

---

## Running Playbooks

### Master playbooks

```bash
# Full infrastructure
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/00_site.yaml --ask-vault-pass

# Base system only
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/01_base_system.yaml --ask-vault-pass

# Ocean services only
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/03_ocean_services.yaml --ask-vault-pass
```

### Individual service playbooks

```bash
# Deploy nginx
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/network/nginx_compose.yaml --ask-vault-pass

# Deploy Plex
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/plex.yaml --ask-vault-pass
```

### Dry-run mode

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/03_ocean_services.yaml --ask-vault-pass --check
```

---

## CI/CD Workflows

GitHub Actions workflows in `.github/workflows/`:

### Automatic (on push/PR)

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `ci-validate.yml` | Push/PR | YAML, Ansible, template validation |
| `pr-test.yml` | PR opened | Test playbooks on ephemeral VM |
| `pr-cleanup.yml` | PR closed | Destroy test VM |
| `main-apply.yml` | Merge to main | Auto-deploy changed playbooks |

### Manual (workflow_dispatch)

| Workflow | Description |
|----------|-------------|
| `deploy-services.yml` | Deploy master playbooks |
| `deploy-ocean-service.yml` | Deploy individual ocean services |
| `deploy-critical-service.yml` | Deploy DNS/DHCP/Plex (approval required) |
| `deploy-changed-services.yml` | Deploy all changed services |

### Required secrets

Configure in GitHub → Settings → Secrets:

- `ANSIBLE_VAULT_PASSWORD` - Vault decryption password

See `.github/workflows/README.md` for full documentation.

---

## Self-Hosted Runners

CI/CD uses self-hosted GitHub Actions runners:

```bash
# Deploy runners
ansible-playbook -i inventories/github_runners/hosts.ini \
  playbooks/individual/infrastructure/github_docker_runners.yaml --ask-vault-pass
```

Runner labels: `self-hosted`, `homelab`, `ansible`, `ephemeral`, `docker`

---

## Troubleshooting

### Permission Denied: ~/Library/Python

```bash
sudo chown -R $USER ~/Library/Python
```

### Command Not Found: ansible

```bash
# Check PATH includes Python bin
echo $PATH | grep "Library/Python"

# If not, add it
echo 'export PATH="$HOME/Library/Python/3.13/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

### PyYAML Not Installed

```bash
make setup-python
```

### Vault Decryption Fails

```bash
# Verify vault password
ansible-vault view vault/secrets.yaml --ask-vault-pass
```

---

## Validation Notes

- `make validate` runs syntax checks only (no vault access required)
- CI workflows use self-hosted runners with pre-mounted SSH keys
- All vault files must be encrypted before committing
