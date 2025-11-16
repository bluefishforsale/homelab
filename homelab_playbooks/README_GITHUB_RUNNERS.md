# Ephemeral GitHub Actions Runners for Homelab

Complete production-grade implementation of ephemeral, Docker-based GitHub Actions self-hosted runners for Proxmox-based homelab environments.

## What This Provides

✅ **Ephemeral runners**: Fresh, clean runner instance for every job  
✅ **Docker support**: Run Docker commands in jobs via mounted socket  
✅ **Ansible ready**: SSH access to homelab for automation workflows  
✅ **Auto-scaling**: N concurrent runners on dedicated VM  
✅ **Production-grade**: Systemd integration, health checks, logging  
✅ **Idempotent**: Safe to re-run, configuration in version control  
✅ **Secure**: Vault integration, no hardcoded tokens

## Architecture Overview

```
Proxmox (node005)
└─ Runner VM (250)
   ├─ 16 vCPUs, 64 GB RAM
   ├─ Docker daemon
   └─ N ephemeral runner containers
      ├─ Register with GitHub
      ├─ Run ONE job
      ├─ Exit and restart
      └─ Re-register as fresh runner
```

## Key Feature: Ephemeral Runners

Each runner container:
1. **Starts** → Registers as new ephemeral runner with GitHub
2. **Runs** → Executes ONE job from GitHub Actions queue
3. **Exits** → De-registers and terminates after job completes
4. **Restarts** → Docker restarts container (restart: always)
5. **Repeats** → Back to step 1 with fresh state

**Benefits:**
- No state pollution between jobs
- No cache/artifact carryover
- Better security (isolated environments)
- Easier debugging (reproducible runs)
- Automatic cleanup

## Files Structure

```
homelab_playbooks/
├── 00_create_runner_vm.yaml              # Proxmox VM creation
├── github-docker-runners.yml              # Main runner config playbook
├── inventory_github_runners.ini.example   # Example inventory
├── group_vars/
│   └── github_runners.yml                 # Runner configuration
├── roles/
│   └── github_docker_runners/
│       ├── README.md                      # Full documentation
│       ├── defaults/main.yml              # Default variables
│       ├── tasks/main.yml                 # Ansible tasks
│       ├── handlers/main.yml              # Service handlers
│       └── templates/
│           ├── docker-compose.yml.j2      # Runner containers
│           └── github-docker-runners.service.j2  # Systemd unit
├── GITHUB_RUNNERS_QUICKSTART.md          # Quick start guide
└── README_GITHUB_RUNNERS.md              # This file
```

## Quick Start (5 Steps)

### 1. Create VM on Proxmox

```bash
cd homelab_playbooks/
ansible-playbook 00_create_runner_vm.yaml
```

Creates `gh-runner-01` VM (VMID 250) with 16 vCPUs and 64 GB RAM.

### 2. Add to Inventory

```ini
# inventory.ini
[github_runners]
gh-runner-01 ansible_host=192.168.1.250 ansible_user=root
```

### 3. Configure Settings

Edit `group_vars/github_runners.yml`:

```yaml
github_scope: "org"
github_org: "your-org-name"
github_runner_count: 4
github_registration_token: "{{ vault_github_registration_token }}"
```

### 4. Get GitHub Token

- Org: `https://github.com/organizations/YOUR-ORG/settings/actions/runners/new`
- Repo: `https://github.com/YOUR-OWNER/YOUR-REPO/settings/actions/runners/new`

Store in Ansible Vault or pass via `--extra-vars`.

### 5. Deploy

```bash
ansible-playbook github-docker-runners.yml --ask-vault-pass
```

## Verification

```bash
# SSH to runner host
ssh root@192.168.1.250

# Check service
systemctl status github-docker-runners

# Check containers
docker ps

# View logs
docker compose -f /opt/github-runners/docker-compose.yml logs -f
```

Check GitHub UI:
- `https://github.com/organizations/YOUR-ORG/settings/actions/runners`

Should show N runners with status "Idle" (green).

## Using Runners in Workflows

```yaml
name: Deploy to Homelab

on:
  push:
    branches: [main]

jobs:
  deploy:
    # Target your self-hosted runners
    runs-on: [self-hosted, homelab, ansible]
    
    steps:
      - uses: actions/checkout@v4
      
      - name: Run Ansible
        run: ansible-playbook deploy.yml -i inventory.ini
      
      - name: Build Docker image
        run: docker build -t myapp:latest .
```

## Configuration Options

### Runner Count

```yaml
# Light: 2-4 runners
github_runner_count: 2

# Medium: 4-8 runners
github_runner_count: 4

# Heavy: 8-16 runners
github_runner_count: 8
```

### Resource Limits

```yaml
# Set per-container limits
github_runner_cpus: "3.0"
github_runner_memory: "12g"
```

### Runner Labels

```yaml
github_runner_labels:
  - self-hosted      # Required
  - homelab          # Environment
  - ansible          # Has Ansible
  - ephemeral        # Fresh per job
  - docker           # Has Docker
  - linux            # OS
  - x64              # Architecture
```

### Docker Configuration

```yaml
# Docker-on-host (recommended, efficient)
github_runner_mount_docker_socket: true

# Docker-in-Docker (isolated, heavier)
github_runner_mount_docker_socket: false
```

## Management Commands

```bash
# Restart runners
systemctl restart github-docker-runners

# View logs
docker compose -f /opt/github-runners/docker-compose.yml logs -f

# Stop runners
systemctl stop github-docker-runners

# Update configuration
vim group_vars/github_runners.yml
ansible-playbook github-docker-runners.yml --ask-vault-pass

# Scale runner count
# 1. Edit github_runner_count
# 2. Re-run playbook
```

## Customization

### VM Specifications

Edit `00_create_runner_vm.yaml`:

```yaml
runner_vm_cores: 16        # CPU count
runner_vm_memory_mb: 65536 # RAM in MB
runner_vm_id: 250          # Proxmox VMID
runner_vm_ip: "192.168.1.250"  # IP address
```

### Multiple Runner Hosts

```ini
[github_runners]
gh-runner-01 ansible_host=192.168.1.250
gh-runner-02 ansible_host=192.168.1.251
gh-runner-03 ansible_host=192.168.1.252
```

Each host runs N independent runners.

### Organization vs Repository

```yaml
# Organization-level (all repos)
github_scope: "org"
github_org: "bluefishforsale"

# Repository-level (one repo)
github_scope: "repo"
github_repo: "bluefishforsale/homelab"
```

## Security

### Token Management

**Never commit tokens to git:**

```yaml
# Use Ansible Vault
github_registration_token: "{{ vault_github_registration_token }}"

# Or pass at runtime
ansible-playbook github-docker-runners.yml \
  --extra-vars "github_registration_token=TOKEN"
```

### Docker Socket Access

Mounting `/var/run/docker.sock` gives containers Docker access:

- **Mitigation**: Only run trusted workflows
- **Best practice**: Use GitHub environment protection
- **Alternative**: Docker-in-Docker (more isolated)

### SSH Access

Runners can SSH to homelab hosts:

- **Mitigation**: Use GitHub environment protection for sensitive deployments
- **Best practice**: Require approval for production deployments
- **Alternative**: Use read-only SSH keys where possible

## Troubleshooting

### Runners Not Registering

```bash
# Check logs
docker logs github-runner-1

# Common causes:
# - Expired token (tokens expire after 1 hour)
# - Wrong GitHub URL (check org/repo name)
# - Network issues (test: curl https://github.com)
```

### Runners Not Picking Up Jobs

```bash
# Verify labels match
# Workflow: runs-on: [self-hosted, homelab]
# Config: github_runner_labels must include these

# Check runner status on GitHub
# Should be "Idle" not "Offline"
```

### Docker Commands Failing

```bash
# Verify socket mount
docker exec github-runner-1 docker ps

# Check configuration
grep mount_docker_socket group_vars/github_runners.yml
# Should be: true
```

## Documentation

- **Quick Start**: `GITHUB_RUNNERS_QUICKSTART.md` - Step-by-step guide
- **Full Docs**: `roles/github_docker_runners/README.md` - Complete reference
- **Defaults**: `roles/github_docker_runners/defaults/main.yml` - All variables
- **Examples**: `group_vars/github_runners.yml` - Configuration examples

## Implementation Details

### Proxmox VM Creation

- Playbook: `00_create_runner_vm.yaml`
- Uses same patterns as existing `kubeadm_ansible/00_create_vms.yaml`
- Clones from template 9999
- Configurable CPU, RAM, disk, network
- Idempotent: safe to re-run

### Runner Configuration

- Role: `roles/github_docker_runners`
- Installs Docker and docker-compose plugin
- Creates `github-runner` user (uid/gid 1100)
- Deploys `docker-compose.yml` with N containers
- Systemd service: `github-docker-runners`
- Idempotent: safe to re-run

### Ephemeral Behavior

Implemented via:
- Environment variable: `RUNNER_EPHEMERAL=true`
- Docker restart policy: `restart: always`
- Runner exits after one job
- Docker restarts container
- Container re-registers as new runner

### Docker Support

Two options:

1. **Docker-on-host** (default):
   - Mounts `/var/run/docker.sock`
   - Uses host's Docker daemon
   - More efficient

2. **Docker-in-Docker**:
   - Runs dockerd inside container
   - More isolated
   - More resource intensive

## Requirements

- Proxmox node with template 9999 configured
- Network access to GitHub
- SSH key authentication configured
- Ansible 2.9+ with `community.general` collection
- Python 3.6+ on target hosts

## Tested Environments

- **Proxmox**: 7.x, 8.x
- **VM OS**: Debian 11, Debian 12, Ubuntu 22.04
- **Runner Image**: `myoung34/github-runner:latest`
- **Docker**: 24.x, 25.x

## GitHub Actions Compatibility

- **Supported**: Organization and repository-level runners
- **Labels**: Custom labels for workflow targeting
- **Features**: Ephemeral runners, runner groups, labels
- **Plans**: Works with all GitHub plans (Free, Team, Enterprise)

## Performance Characteristics

### Resource Usage per Runner

- **Idle**: ~100 MB RAM, minimal CPU
- **Active**: Depends on job (1-4 GB RAM typical)
- **Docker builds**: Can use significant resources

### Capacity Planning

```
16 vCPUs / 4 runners = 4 cores per runner (with overhead)
64 GB RAM / 4 runners = 16 GB per runner (with overhead)
```

Adjust based on typical job requirements.

### Scaling Guidelines

- **Light usage** (few jobs/day): 2-4 runners, 8 vCPUs, 32 GB RAM
- **Medium usage** (many jobs/day): 4-8 runners, 16 vCPUs, 64 GB RAM
- **Heavy usage** (continuous jobs): 8-16 runners, 32 vCPUs, 128 GB RAM

## Maintenance

### Regular Tasks

```bash
# Update runner images (monthly)
cd /opt/github-runners
docker compose pull
docker compose up -d

# Check disk usage
df -h /opt/github-runners

# Review logs for errors
journalctl -u github-docker-runners --since "1 week ago"
```

### Token Rotation

```bash
# Tokens expire after 1 hour
# Generate new token before deployment
# Update vault or pass via --extra-vars
ansible-playbook github-docker-runners.yml --ask-vault-pass
```

### Configuration Updates

```bash
# Edit configuration
vim group_vars/github_runners.yml

# Apply changes
ansible-playbook github-docker-runners.yml --ask-vault-pass

# Runners will restart with new configuration
```

## Advanced Usage

### Build a custom image with additional tools:

```dockerfile
FROM myoung34/github-runner:latest
RUN apt-get update && apt-get install -y \
    terraform kubectl helm
```

```yaml
github_runner_image: "your-registry/custom-runner"
```

### GPU Support

```yaml
# In docker-compose.yml.j2, add to specific runner:
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          count: 1
          capabilities: [gpu]
```

### Multiple Organizations

Deploy separate runner hosts per org:

```yaml
# host_vars/gh-runner-01.yml
github_org: "org-one"

# host_vars/gh-runner-02.yml
github_org: "org-two"
```

## Support and Contributions

For issues, questions, or improvements:

1. Check `roles/github_docker_runners/README.md` for detailed troubleshooting
2. Review container logs: `docker logs github-runner-1 -f`
3. Verify GitHub runner status on GitHub UI
4. Test with minimal workflow first

## License

This implementation follows the same license as your homelab repository.

## References

- [GitHub Actions Self-hosted Runners](https://docs.github.com/en/actions/hosting-your-own-runners)
- [GitHub Actions Runner Images](https://github.com/actions/runner)
- [Docker Documentation](https://docs.docker.com/)
- [Ansible Documentation](https://docs.ansible.com/)
