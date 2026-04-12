# Homelab Claude Code Skills

**Date:** 2026-04-12
**Status:** Approved

## Overview

Five Claude Code skills for operating the homelab Ansible infrastructure, layered from base utilities to composite diagnostics.

## Skill Inventory

| Skill | Layer | Purpose |
|-------|-------|---------|
| `ansible-playbook` | Base | Run any playbook from homelab root |
| `ansible-shell` | Base | Ad-hoc shell command on a specified host/group |
| `docker-logs` | Base | SSH to host (from inventory) and run `docker logs` |
| `plex-logs` | Shortcut | SSH to ocean, tail 1000 lines of PMS log |
| `debug-service` | Composite | Auto-discover service, gather diagnostics, brainstorm analysis |

All skills live in `~/.claude/skills/<name>/SKILL.md`.

## Environment

- **Repo root:** `/home/terrac/projects/bluefishforsale/homelab`
- **Config:** `.envrc` provides `ANSIBLE_VAULT_PASSWORD_FILE`, `ANSIBLE_CONFIG`, `ANSIBLE_INVENTORY`, `HOMELAB_VAULT_FILE`
- **Inventory:** `inventories/production/hosts.ini`
- **Playbooks:** `playbooks/individual/` organized by host/category

## Skill Designs

### 1. ansible-playbook

Runs an Ansible playbook from the homelab repo root.

- **Requires:** playbook path
- **Optional:** `--tags`, `--skip-tags`, `--limit`, any extra ansible-playbook flags
- **Working directory:** always `/home/terrac/projects/bluefishforsale/homelab`
- **Environment:** relies on `.envrc` for vault password, config, inventory
- **Command:** `ansible-playbook <path> [flags]`

### 2. ansible-shell

Runs an ad-hoc shell command on a host or group via Ansible.

- **Requires:** host or group name, command string
- **Working directory:** always `/home/terrac/projects/bluefishforsale/homelab`
- **Environment:** relies on `.envrc`
- **Command:** `ansible <host> -m shell -a "<command>"`
- **Host/group is always required** — no default target

### 3. docker-logs

Retrieves docker logs from a container on a remote host via SSH.

- **Requires:** container name, host name (as it appears in inventory)
- **Optional:** `--tail N`, `--since`, `--follow`
- **Host resolution:** parses `inventories/production/hosts.ini` to resolve `ansible_user` and `ansible_ssh_host` for the given host name
- **Command:** `ssh <user>@<ip> docker logs [flags] <container>`

### 4. plex-logs

Zero-argument shortcut to tail the Plex Media Server application log.

- **Host:** ocean (`terrac@192.168.1.143` from inventory)
- **Log path:** `/data01/services/plex/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log`
- **Command:** `ssh terrac@192.168.1.143 tail -n1000 '/data01/services/plex/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log'`

### 5. debug-service

Composite diagnostic skill. Auto-discovers service details from playbooks, collects diagnostics from multiple sources, then invokes brainstorming to analyze the output.

#### Dynamic Service Discovery

At invocation, parses YAML playbook files under `playbooks/individual/` to build a service map. From each playbook it extracts:

- **`hosts`** — which inventory host runs the service
- **`service`** — container name
- **`data`** — base data path (e.g. `/data01`)
- **Derived service home:** `<data>/services/<service>`

The user's request (e.g. "debug plex", "debug sonarr") is matched against discovered service names. If ambiguous or not found, the skill lists all available services.

#### Data Collection

Four diagnostic sources, collected via SSH to the resolved host:

1. **`docker logs --tail 200 <container>`** — recent container stdout/stderr
2. **`docker inspect <container>`** — health check status, container state, restart count, exit code
3. **`systemctl status <service>.service`** — systemd unit state and recent journal
4. **Filesystem log (plex only):** `tail -n1000 /data01/services/plex/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log`

SSH commands use `<ansible_user>@<ansible_ssh_host>` resolved from inventory.

#### Analysis

After collection, all diagnostic output is fed into brainstorming with a prompt: analyze for errors, warnings, patterns, and root causes; propose next steps.

#### Error Handling

If a container doesn't exist, a service unit isn't found, or SSH fails for a specific command, the skill reports what it couldn't reach and analyzes what it did collect. Partial data is better than total failure.

## Inventory Reference

Hosts and their SSH details are in `inventories/production/hosts.ini`. Key entries:

- `ocean` — `terrac@192.168.1.143` (GPU VM, runs media/AI services)
- `dns01` — `debian@192.168.1.2`
- `dns02` — `debian@192.168.1.3`
- `node005` — `root@192.168.1.105` (Proxmox bare metal)
- `node006` — `root@192.168.1.106` (Proxmox bare metal)

## Playbook Convention

All service playbooks follow a consistent pattern with these vars:

```yaml
service: <container_name>
hosts: <inventory_host>
data: /data01  # or similar
```

Service home is derived as `<data>/services/<service>`.
