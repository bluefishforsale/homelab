---
description: Run ad-hoc ansible commands against homelab hosts
---

# Ad-Hoc Ansible Commands

Run ad-hoc ansible commands against homelab infrastructure.

## Prerequisites

- Vault password available via `ANSIBLE_VAULT_PASSWORD_FILE` or `ANSIBLE_VAULT_PASSWORD` environment variable
- SSH access configured for target hosts
- Working directory: repository root

## Inventory

- **Path**: `inventories/production/hosts.ini` (configured in `ansible.cfg`, no `-i` flag needed)
- **Groups**: `proxmox`, `baremetal`, `vms`, `github_runners`, `dns`, `k8s`
- **Key hosts**: `ocean` (192.168.1.143), `node005` (192.168.1.105), `node006` (192.168.1.106), `dns01` (192.168.1.2), `pihole` (192.168.1.9), `gitlab` (192.168.1.5), `gh-runner-01` (192.168.1.20)

## Steps

1. Ask the user what they want to run. Gather:
   - **Target**: host name or group (default: `ocean`)
   - **Module**: ansible module to use (e.g. `ping`, `shell`, `command`, `setup`, `service`, `apt`, `copy`, `file`, `docker_container`, `systemd`)
   - **Args**: module arguments (required for most modules except `ping`)
   - **Become**: whether to use sudo (default: yes for most operations)

2. Build and run the command using this pattern:

```bash
ansible <target> -m <module> -a "<args>" --become
```

### Common Examples

**Connectivity check**

```bash
// turbo
ansible all -m ping
```

**Run a shell command**

```bash
ansible ocean -m shell -a "docker ps --format 'table {{.Names}}\t{{.Status}}'" --become
```

**Check disk usage**

```bash
ansible ocean -m shell -a "df -h /data01" --become
```

**Check service status**

```bash
ansible ocean -m systemd -a "name=plex state=status" --become
```

**Restart a service**

```bash
ansible ocean -m systemd -a "name=plex state=restarted" --become
```

**Get system facts**

```bash
ansible ocean -m setup -a "filter=ansible_memtotal_mb"
```

**Check GPU status**

```bash
ansible ocean -m shell -a "nvidia-smi" --become
```

**Tail container logs**

```bash
ansible ocean -m shell -a "docker logs --tail 50 <container>" --become
```

**Check ZFS pool status**

```bash
ansible ocean -m shell -a "zpool status data01" --become
```

**Run against multiple hosts**

```bash
ansible vms -m shell -a "uptime" --become
```

**Run a playbook (single service)**

```bash
ansible-playbook playbooks/individual/ocean/<category>/<service>.yaml
```

**Dry-run a playbook**

```bash
ansible-playbook playbooks/individual/ocean/<category>/<service>.yaml --check
```

1. Present the constructed command to the user for approval before running. Never auto-run destructive commands (restart, stop, apt remove, file deletion, etc.).

1. After execution, summarize the output concisely — highlight failures, changed state, or key data points.
