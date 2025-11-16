# GitHub Actions Ephemeral Runners - Quick Start Guide

Complete guide to deploying ephemeral Docker-based GitHub Actions runners in your homelab.

## Prerequisites

- Proxmox node `node005` accessible via Ansible
- VM template 9999 configured with cloud-init
- GitHub organization or repository admin access
- Network: 192.168.1.0/24 (adjust if different)

## Step 1: Create Runner Host VM

```bash
cd homelab_playbooks/

# Create VM on Proxmox
ansible-playbook 00_create_runner_vm.yaml

# Output will show:
#   VM ID: 250
#   VM Name: gh-runner-01
#   IP Address: 192.168.1.250
#   vCPUs: 16
#   Memory: 64 GB
```

**What this does:**
- Clones template 9999 to create VMID 250
- Configures 16 vCPUs and 64 GB RAM
- Sets hostname to `gh-runner-01`
- Assigns IP 192.168.1.250 (or from DNS)
- Installs SSH keys from GitHub
- Starts the VM

**Customize if needed:**

```yaml
# In 00_create_runner_vm.yaml, adjust variables:
runner_vm_id: 250                    # Change VMID
runner_vm_name: gh-runner-01         # Change name
runner_vm_cores: 16                  # Change CPU count
runner_vm_memory_mb: 65536           # Change RAM (in MB)
runner_vm_ip: "192.168.1.250"        # Change IP
```

## Step 2: Add VM to Inventory

```bash
# Option A: Add to existing inventory.ini
cat >> inventory.ini << 'EOF'

[github_runners]
gh-runner-01 ansible_host=192.168.1.250 ansible_user=root
EOF

# Option B: Create dedicated inventory
cp inventory_github_runners.ini.example inventory_github_runners.ini
# Edit as needed
```

## Step 3: Obtain GitHub Registration Token

**For organization-level runners:**

1. Go to: `https://github.com/organizations/YOUR-ORG/settings/actions/runners/new`
2. Click "New self-hosted runner"
3. Copy the registration token (starts with `AAAA...`)

**For repository-level runners:**

1. Go to: `https://github.com/YOUR-OWNER/YOUR-REPO/settings/actions/runners/new`
2. Click "New self-hosted runner"
3. Copy the registration token (starts with `AAAA...`)

**Important:** Tokens expire after 1 hour!

## Step 4: Configure Runner Settings

```bash
# Copy example configuration
cp group_vars/github_runners.yml group_vars/github_runners.yml.backup
vim group_vars/github_runners.yml
```

**Minimum required configuration:**

```yaml
# Scope: "org" or "repo"
github_scope: "org"

# Organization name (if scope is "org")
github_org: "bluefishforsale"

# Repository name (if scope is "repo")
# github_repo: "bluefishforsale/homelab"

# Registration token - DO NOT COMMIT
# Use one of the methods below
github_registration_token: ""

# Number of runners (adjust based on needs)
github_runner_count: 4

# Labels (customize as needed)
github_runner_labels:
  - self-hosted
  - homelab
  - ansible
  - ephemeral
  - docker
```

## Step 5: Store Registration Token Securely

**Option A: Ansible Vault (recommended for production)**

```bash
# Create/edit vault file
ansible-vault create vault_secrets.yaml
# Or edit existing:
# ansible-vault edit vault_secrets.yaml

# Add this content:
---
vault_github_registration_token: "AAAA...YOUR_TOKEN_HERE"

# In group_vars/github_runners.yml, reference it:
github_registration_token: "{{ vault_github_registration_token }}"
```

**Option B: Command-line (quick testing)**

```bash
# Pass token directly (not recommended for production)
ansible-playbook github-docker-runners.yml \
  --extra-vars "github_registration_token=AAAA...YOUR_TOKEN_HERE"
```

## Step 6: Deploy Runners

**With Vault:**

```bash
ansible-playbook github-docker-runners.yml --ask-vault-pass
```

**With command-line token:**

```bash
ansible-playbook github-docker-runners.yml \
  --extra-vars "github_registration_token=AAAA...YOUR_TOKEN_HERE"
```

**What this does:**
1. Installs Docker and dependencies
2. Creates `github-runner` user (uid/gid 1100)
3. Creates `/opt/github-runners` directory structure
4. Deploys `docker-compose.yml` with N runner containers
5. Creates systemd service `github-docker-runners`
6. Starts all runner containers
7. Runners register with GitHub as ephemeral runners

## Step 7: Verify Deployment

**Check service status:**

```bash
ssh root@192.168.1.250

# Check systemd service
systemctl status github-docker-runners

# Should show: Active: active (exited)
```

**Check runner containers:**

```bash
# List running containers
docker ps

# Should show:
#   github-runner-1
#   github-runner-2
#   github-runner-3
#   github-runner-4
```

**View runner logs:**

```bash
# All runners
cd /opt/github-runners
docker compose logs -f

# Specific runner
docker logs github-runner-1 -f

# Look for:
#   "Listening for Jobs"
#   "Runner successfully added"
```

**Verify on GitHub:**

- Organization: `https://github.com/organizations/YOUR-ORG/settings/actions/runners`
- Repository: `https://github.com/YOUR-OWNER/YOUR-REPO/settings/actions/runners`

You should see 4 runners (or your configured count):
- Names: `gh-runner-01-runner-1`, `gh-runner-01-runner-2`, etc.
- Status: Idle (green dot)
- Labels: self-hosted, homelab, ansible, ephemeral, docker

## Step 8: Test with a Workflow

Create a simple test workflow in your repo:

```bash
# .github/workflows/test-runner.yml
name: Test Self-Hosted Runner

on:
  workflow_dispatch:

jobs:
  test:
    runs-on: [self-hosted, homelab]
    
    steps:
      - name: Print runner info
        run: |
          echo "Runner: $(hostname)"
          echo "User: $(whoami)"
          echo "Docker: $(docker --version)"
          echo "Ansible: $(ansible --version | head -1)"
          echo "Python: $(python3 --version)"
      
      - name: Test Docker
        run: docker ps
      
      - name: Test Ansible
        run: ansible --version
```

**Run the workflow:**

1. Go to Actions tab in your repo
2. Select "Test Self-Hosted Runner"
3. Click "Run workflow"
4. Watch it run on one of your runners

**Expected output:**
- Workflow runs on your self-hosted runner
- All commands succeed
- Runner shows Docker and Ansible versions
- After job completes, runner exits and restarts (ephemeral)

## Common Configuration Scenarios

### Scenario 1: Organization with 4 runners

```yaml
github_scope: "org"
github_org: "bluefishforsale"
github_runner_count: 4
github_runner_labels:
  - self-hosted
  - homelab
  - ansible
  - ephemeral
github_runner_ephemeral: true
github_runner_mount_docker_socket: true
```

### Scenario 2: Single repository with 2 runners

```yaml
github_scope: "repo"
github_repo: "bluefishforsale/homelab"
github_runner_count: 2
github_runner_labels:
  - self-hosted
  - homelab
  - ansible
github_runner_ephemeral: true
github_runner_mount_docker_socket: true
```

### Scenario 3: High-capacity setup (8 runners)

```yaml
github_scope: "org"
github_org: "bluefishforsale"
github_runner_count: 8
github_runner_labels:
  - self-hosted
  - homelab
  - ansible
  - ephemeral
  - docker
github_runner_ephemeral: true
github_runner_mount_docker_socket: true

# Resource limits per runner
github_runner_cpus: "2.0"
github_runner_memory: "6g"
```

## Quick Reference Commands

```bash
# Restart all runners
systemctl restart github-docker-runners

# View logs
docker compose -f /opt/github-runners/docker-compose.yml logs -f

# Stop runners
systemctl stop github-docker-runners

# Update configuration
vim group_vars/github_runners.yml
ansible-playbook github-docker-runners.yml --ask-vault-pass

# Scale runner count
# 1. Edit github_runner_count in group_vars/github_runners.yml
# 2. Re-run playbook
ansible-playbook github-docker-runners.yml --ask-vault-pass

# Check Docker status
docker ps
docker stats

# View systemd logs
journalctl -u github-docker-runners -f
```

## Troubleshooting

### Runners not registering

```bash
# Check logs
docker logs github-runner-1

# Common issues:
# - Expired token (re-run with fresh token)
# - Wrong GitHub URL (check github_scope and github_org/github_repo)
# - Network connectivity (test: curl https://github.com)
```

### Runners not picking up jobs

```bash
# Verify labels match workflow
# Workflow: runs-on: [self-hosted, homelab]
# Runners must have these labels in github_runner_labels

# Check runner status on GitHub
# Should show "Idle" not "Offline"
```

### Docker commands failing in jobs

```bash
# Verify Docker socket is mounted
docker exec github-runner-1 docker ps

# Check github_runner_mount_docker_socket is true
grep mount_docker_socket group_vars/github_runners.yml
```

### Token expired during deployment

```bash
# Tokens expire after 1 hour
# Generate new token and re-run
ansible-playbook github-docker-runners.yml \
  --extra-vars "github_registration_token=NEW_TOKEN"
```

## Next Steps

1. **Add more runners**: Increase `github_runner_count` and re-run playbook
2. **Scale horizontally**: Create additional runner VMs on other Proxmox nodes
3. **Customize labels**: Add labels for specific capabilities (gpu, terraform, etc.)
4. **Set resource limits**: Add `github_runner_cpus` and `github_runner_memory`
5. **Monitor performance**: Track job queue times and runner utilization
6. **Update regularly**: Pull new runner images and redeploy

## Security Reminders

- **Never commit GitHub tokens** to version control
- **Use Ansible Vault** for production deployments
- **Rotate tokens regularly** (they expire anyway)
- **Monitor runner logs** for suspicious activity
- **Use GitHub environment protection** for sensitive deployments
- **Review workflow permissions** (GITHUB_TOKEN scope)

## Documentation

- Full documentation: `roles/github_docker_runners/README.md`
- Role defaults: `roles/github_docker_runners/defaults/main.yml`
- Example configuration: `group_vars/github_runners.yml`
- Docker Compose template: `roles/github_docker_runners/templates/docker-compose.yml.j2`

## Support

For detailed troubleshooting, configuration options, and advanced usage, see:
- `roles/github_docker_runners/README.md`
- [GitHub Actions Documentation](https://docs.github.com/en/actions/hosting-your-own-runners)
- Your container logs: `docker logs github-runner-1 -f`
