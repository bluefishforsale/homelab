# GitHub Actions Ephemeral Docker Runners Role

Production-grade Ansible role for deploying ephemeral, Docker-based GitHub Actions self-hosted runners in a homelab environment.

## Overview

This role configures a dedicated runner host VM with:

- **Ephemeral runners**: Fresh, clean runner instance for each job
- **Docker support**: Run Docker commands in job steps via mounted socket
- **Ansible capabilities**: SSH access to homelab inventory for automation
- **Automatic restart**: Runners re-register after each job completes
- **Resource isolation**: Configurable CPU and memory limits per runner
- **Scalable**: Run N concurrent runners on a single host

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Runner Host VM (16 vCPUs, 64 GB RAM)                        │
│                                                               │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐ │
│  │ Runner 1       │  │ Runner 2       │  │ Runner N       │ │
│  │                │  │                │  │                │ │
│  │ • Registers    │  │ • Registers    │  │ • Registers    │ │
│  │ • Runs 1 job   │  │ • Runs 1 job   │  │ • Runs 1 job   │ │
│  │ • Exits        │  │ • Exits        │  │ • Exits        │ │
│  │ • Restarts     │  │ • Restarts     │  │ • Restarts     │ │
│  │ • Re-registers │  │ • Re-registers │  │ • Re-registers │ │
│  └────────────────┘  └────────────────┘  └────────────────┘ │
│         ↓                    ↓                    ↓          │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Host Docker Daemon                                    │   │
│  │ /var/run/docker.sock                                  │   │
│  └──────────────────────────────────────────────────────┘   │
│         ↓                    ↓                    ↓          │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Homelab Network (SSH to other hosts)                  │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
         ↓                    ↓                    ↓
┌──────────────────────────────────────────────────────────────┐
│ GitHub Actions (jobs queued, dispatched to runners)          │
└──────────────────────────────────────────────────────────────┘
```

## Deployment Workflow

### 1. Create Runner Host VM on Proxmox

```bash
cd homelab_playbooks/

# Edit variables in playbook or create vars file
ansible-playbook 00_create_runner_vm.yaml

# VM will be created on node005:
#   - Name: gh-runner-01
#   - VMID: 250
#   - vCPUs: 16
#   - RAM: 64 GB
#   - IP: 192.168.1.250 (or via DNS)
```

### 2. Add Runner Host to Inventory

```ini
# inventory.ini
[github_runners]
gh-runner-01 ansible_host=192.168.1.250 ansible_user=root
```

### 3. Configure Runner Settings

```bash
# Copy example configuration
cp group_vars/github_runners.yml group_vars/github_runners.yml

# Edit configuration
vim group_vars/github_runners.yml
```

Key settings to configure:

```yaml
github_scope: "org"  # or "repo"
github_org: "your-org-name"
github_runner_count: 4
github_runner_labels:
  - self-hosted
  - homelab
  - ansible
  - ephemeral
```

### 4. Obtain GitHub Registration Token

**For organization-level runners:**
```
https://github.com/organizations/YOUR-ORG/settings/actions/runners/new
```

**For repository-level runners:**
```
https://github.com/YOUR-OWNER/YOUR-REPO/settings/actions/runners/new
```

**Important:** Tokens expire after 1 hour!

### 5. Store Token Securely

**Option A: Ansible Vault (recommended)**

```bash
# Edit vault file
ansible-vault edit vault_secrets.yaml

# Add token
vault_github_registration_token: "YOUR_TOKEN_HERE"

# Reference in group_vars/github_runners.yml
github_registration_token: "{{ development.github.vault_github_registration_token }}"
```

**Option B: Command-line override**

```bash
ansible-playbook github-docker-runners.yml \
  --extra-vars "github_registration_token=YOUR_TOKEN_HERE"
```

### 6. Deploy Runners

```bash
# Deploy with vault
ansible-playbook github-docker-runners.yml --ask-vault-pass

# Or with command-line token
ansible-playbook github-docker-runners.yml \
  --extra-vars "github_registration_token=YOUR_TOKEN_HERE"
```

## Verification

### Check Service Status

```bash
# On the runner host
systemctl status github-docker-runners

# Should show: Active: active (exited)
```

### Check Runner Containers

```bash
# List running containers
docker ps

# Should show N containers: github-runner-1, github-runner-2, ...
```

### Check Runner Registration

```bash
# View logs from all runners
cd /opt/github-runners
docker compose logs -f

# View logs from specific runner
docker logs github-runner-1 -f
```

### Verify on GitHub

**Organization runners:**
```
https://github.com/organizations/YOUR-ORG/settings/actions/runners
```

**Repository runners:**
```
https://github.com/YOUR-OWNER/YOUR-REPO/settings/actions/runners
```

You should see N runners listed with your configured labels.

## Using Runners in Workflows

### Basic Example

```yaml
name: Deploy with Ansible

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: [self-hosted, homelab, ansible]
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Run Ansible playbook
        run: ansible-playbook deploy.yml -i inventory.ini
```

### Docker Build Example

```yaml
name: Build Docker Image

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: [self-hosted, homelab, docker]
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Build image
        run: docker build -t myapp:latest .
      
      - name: Push image
        run: docker push myapp:latest
```

### Matrix Build Example

```yaml
name: Test Matrix

on:
  pull_request:

jobs:
  test:
    runs-on: [self-hosted, homelab, ephemeral]
    
    strategy:
      matrix:
        python: ['3.9', '3.10', '3.11']
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Run tests
        run: python${{ matrix.python }} -m pytest
```

## Configuration Options

### Runner Count

Adjust based on concurrent job requirements:

```yaml
# Low usage: 2-4 runners
github_runner_count: 2

# Medium usage: 4-8 runners
github_runner_count: 4

# High usage: 8-16 runners
github_runner_count: 8
```

### Resource Limits

Set CPU and memory limits per container:

```yaml
# Conservative (4 cores, 8GB per runner)
github_runner_cpus: "4.0"
github_runner_memory: "8g"

# Moderate (3 cores, 12GB per runner)
github_runner_cpus: "3.0"
github_runner_memory: "12g"

# Generous (no limits, runners share host resources)
# github_runner_cpus: <not set>
# github_runner_memory: <not set>
```

### Docker Configuration

**Docker-on-host (recommended):**
```yaml
github_runner_mount_docker_socket: true
```

**Docker-in-Docker:**
```yaml
github_runner_mount_docker_socket: false
# Then modify docker-compose.yml.j2 to enable privileged mode
```

### Runner Labels

Customize labels to target specific runners:

```yaml
github_runner_labels:
  - self-hosted       # Required
  - homelab           # Environment
  - ansible           # Capability
  - ephemeral         # Behavior
  - docker            # Capability
  - gpu               # Hardware (if applicable)
  - linux             # OS
  - x64               # Architecture
```

## Management

### Restart Runners

```bash
# Restart all runners
systemctl restart github-docker-runners

# Or using docker compose
cd /opt/github-runners
docker compose restart
```

### Update Configuration

```bash
# 1. Edit group_vars/github_runners.yml
vim group_vars/github_runners.yml

# 2. Re-run playbook
ansible-playbook github-docker-runners.yml --ask-vault-pass

# Ansible will:
#   - Update docker-compose.yml
#   - Restart containers via handler
#   - Runners will re-register with new configuration
```

### Scale Runner Count

```bash
# 1. Change github_runner_count in group_vars
github_runner_count: 8  # Was 4, now 8

# 2. Re-run playbook
ansible-playbook github-docker-runners.yml --ask-vault-pass

# New containers will be created automatically
```

### View Logs

```bash
# All runners
docker compose -f /opt/github-runners/docker-compose.yml logs -f

# Specific runner
docker logs github-runner-1 -f

# With timestamps
docker logs github-runner-1 -f --timestamps

# Last 100 lines
docker logs github-runner-1 --tail 100
```

### Stop Runners

```bash
# Stop service (stops all runners)
systemctl stop github-docker-runners

# Or using docker compose
cd /opt/github-runners
docker compose down
```

## Troubleshooting

### Runners Not Registering

**Check token validity:**
```bash
# Tokens expire after 1 hour
# Generate a new token and re-run playbook
```

**Check container logs:**
```bash
docker logs github-runner-1
```

**Common issues:**
- Expired registration token
- Incorrect GitHub URL (org vs repo)
- Network connectivity to GitHub
- Invalid labels format

### Runners Not Picking Up Jobs

**Verify labels match:**
```yaml
# Workflow must target runner labels
runs-on: [self-hosted, homelab]  # Must match github_runner_labels
```

**Check runner status on GitHub:**
- Runners should show as "Idle" (ready) or "Active" (running job)
- If "Offline", check container status

**Check Docker socket:**
```bash
# If jobs fail with Docker errors
docker ps  # Should work from inside container
docker exec github-runner-1 docker ps
```

### Container Keeps Restarting

**Check logs for errors:**
```bash
docker logs github-runner-1 --tail 100
```

**Common causes:**
- Invalid registration token
- Network issues reaching GitHub
- Docker socket permissions (if mounted)
- Resource exhaustion (CPU/memory limits too low)

**Temporarily stop restart:**
```bash
# For debugging
docker update --restart=no github-runner-1
docker stop github-runner-1
```

### Jobs Fail with Docker Errors

**Verify Docker socket mount:**
```bash
# Check docker-compose.yml
grep docker.sock /opt/github-runners/docker-compose.yml

# Should show:
# - /var/run/docker.sock:/var/run/docker.sock
```

**Check Docker group permissions:**
```bash
# Runner user should be in docker group
id github-runner
# Should show: groups=...,docker,...
```

**Test Docker from container:**
```bash
docker exec github-runner-1 docker ps
# Should list containers
```

### Performance Issues

**Check resource usage:**
```bash
# CPU and memory usage
docker stats

# System resources
htop
```

**Adjust limits:**
```yaml
# Reduce limits if overcommitted
github_runner_cpus: "2.0"
github_runner_memory: "6g"
```

**Reduce runner count:**
```yaml
# If host is overloaded
github_runner_count: 2  # Was 4
```

## Security Considerations

### Docker Socket Access

Mounting `/var/run/docker.sock` gives containers access to the Docker daemon:

- **Risk**: Containers can start/stop other containers, build images, etc.
- **Mitigation**: Only run trusted workflows on these runners
- **Alternative**: Use Docker-in-Docker (more isolated, heavier)

### SSH Key Access

Runners have SSH access to your homelab:

- **Risk**: Compromised runner could SSH to other hosts
- **Mitigation**:
  - Use GitHub environment protection rules
  - Require approval for sensitive deployments
  - Use read-only SSH keys where possible
  - Monitor SSH access logs

### GitHub Token Expiration

Registration tokens expire after 1 hour:

- **Issue**: Cannot register new runners after token expires
- **Solution**: Runners stay registered until they exit
- **Best practice**: Re-run playbook periodically with fresh token

### Ephemeral Benefits

Ephemeral runners improve security:

- Fresh environment per job (no state carryover)
- No cache poisoning between jobs
- Easier to audit (isolated executions)
- Automatic cleanup after each job

## Maintenance

### Regular Updates

```bash
# Update runner images
ansible-playbook github-docker-runners.yml --tags update

# Or manually
cd /opt/github-runners
docker compose pull
docker compose up -d
```

### Monitoring

**Set up monitoring for:**
- Runner availability (uptime)
- Job success/failure rates
- Resource usage (CPU, memory, disk)
- Queue times (jobs waiting for runners)

**Useful metrics:**
```bash
# Container uptime
docker ps --format "table {{.Names}}\t{{.Status}}"

# Resource usage
docker stats --no-stream

# Disk usage
df -h /opt/github-runners
```

### Backup

Important files to backup:

```bash
/opt/github-runners/docker-compose.yml      # Runner configuration
/home/github-runner/.ssh/                   # SSH keys
group_vars/github_runners.yml               # Ansible configuration
```

## Advanced Configuration

### Custom Runner Image

Build a custom image with additional tools:

```dockerfile
FROM myoung34/github-runner:latest

# Install additional tools
RUN apt-get update && apt-get install -y \
    terraform \
    kubectl \
    helm \
    && rm -rf /var/lib/apt/lists/*
```

```yaml
# Use custom image
github_runner_image: "your-registry/custom-runner"
github_runner_version: "latest"
```

### Per-Runner Configuration

Different configurations per runner:

```yaml
# In docker-compose.yml.j2, customize per runner
{% if runner_index == 1 %}
  # Runner 1: GPU access
  - /dev/nvidia0:/dev/nvidia0
{% elif runner_index == 2 %}
  # Runner 2: Extra memory
  mem_limit: "24g"
{% endif %}
```

### Multiple Runner Hosts

Deploy to multiple VMs for higher capacity:

```ini
# inventory.ini
[github_runners]
gh-runner-01 ansible_host=192.168.1.250
gh-runner-02 ansible_host=192.168.1.251
gh-runner-03 ansible_host=192.168.1.252
```

Each host runs N runners independently.

## References

- [GitHub Actions Self-hosted Runners](https://docs.github.com/en/actions/hosting-your-own-runners)
- [GitHub Actions Runner Documentation](https://github.com/actions/runner)
- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [Ansible Documentation](https://docs.ansible.com/)

## Support

For issues or questions:
1. Check container logs: `docker logs github-runner-1`
2. Check systemd status: `systemctl status github-docker-runners`
3. Verify GitHub runner status on GitHub UI
4. Review this documentation's Troubleshooting section
