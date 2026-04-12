# Homelab Claude Code Skills Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create five Claude Code skills for operating the homelab: ansible-playbook, ansible-shell, docker-logs, plex-logs, and debug-service.

**Architecture:** Each skill is a single `SKILL.md` file in `~/.claude/skills/<name>/`. The four base skills are standalone instructions. The `debug-service` skill composes the others by dynamically parsing playbook YAML files to discover services, then collecting diagnostics via SSH and analyzing the output.

**Tech Stack:** Claude Code skills (Markdown with YAML frontmatter), Bash (ansible, ssh, docker), YAML parsing via grep/awk in Bash.

---

### Task 1: Create ansible-playbook skill

**Files:**
- Create: `~/.claude/skills/ansible-playbook/SKILL.md`

- [ ] **Step 1: Create the skill directory**

```bash
mkdir -p ~/.claude/skills/ansible-playbook
```

- [ ] **Step 2: Write the SKILL.md**

Create `~/.claude/skills/ansible-playbook/SKILL.md` with this content:

```markdown
---
name: ansible-playbook
description: Run Ansible playbooks from the homelab repo. Use when the user asks to deploy, configure, or run a playbook against homelab infrastructure.
---

# Ansible Playbook Runner

Run Ansible playbooks from the homelab repository root.

## Environment

- **Repo root:** `/home/terrac/projects/bluefishforsale/homelab`
- **Config:** `.envrc` provides `ANSIBLE_VAULT_PASSWORD_FILE`, `ANSIBLE_CONFIG`, `ANSIBLE_INVENTORY`
- **Inventory:** `inventories/production/hosts.ini`

## How to Use

1. Identify which playbook the user wants to run. Playbooks are under `playbooks/individual/` organized by host and category.
2. Run the playbook from the repo root using Bash. The `.envrc` is loaded by direnv automatically.

**Command pattern:**

` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible-playbook <playbook-path> [flags]
` ` `

**Common flags:**
- `--tags <tag1,tag2>` — run only tagged tasks
- `--skip-tags <tag>` — skip tagged tasks (e.g. `--skip-tags restart`)
- `--limit <host>` — restrict to specific host
- `--check` — dry run
- `--diff` — show file diffs
- `-v` / `-vv` / `-vvv` — verbosity

## Playbook Discovery

List available playbooks:

` ` `bash
find /home/terrac/projects/bluefishforsale/homelab/playbooks/individual -name '*.yaml' | sort
` ` `

Each playbook has header comments documenting available tags and usage examples. Read the playbook file before running to check for tag options.

## Examples

Deploy plex with all defaults:
` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible-playbook playbooks/individual/ocean/media/plex.yaml
` ` `

Deploy plex config only, no restart:
` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible-playbook playbooks/individual/ocean/media/plex.yaml --tags config --skip-tags restart
` ` `

Dry run of base system playbook:
` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible-playbook playbooks/01_base_system.yaml --check --diff
` ` `
```

- [ ] **Step 3: Verify the skill appears**

```bash
ls ~/.claude/skills/ansible-playbook/SKILL.md
```

Expected: file exists.

Note: Skills live in `~/.claude/skills/` which is outside this git repo. No commit needed for individual skill files. The plan and spec docs in this repo will be committed at the end.

---

### Task 2: Create ansible-shell skill

**Files:**
- Create: `~/.claude/skills/ansible-shell/SKILL.md`

- [ ] **Step 1: Create the skill directory**

```bash
mkdir -p ~/.claude/skills/ansible-shell
```

- [ ] **Step 2: Write the SKILL.md**

Create `~/.claude/skills/ansible-shell/SKILL.md` with this content:

```markdown
---
name: ansible-shell
description: Run ad-hoc shell commands on homelab hosts via Ansible. Use when the user asks to run a command on a remote host or group of hosts.
---

# Ansible Ad-Hoc Shell Commands

Run shell commands on remote homelab hosts via Ansible's shell module.

## Environment

- **Repo root:** `/home/terrac/projects/bluefishforsale/homelab`
- **Config:** `.envrc` provides `ANSIBLE_VAULT_PASSWORD_FILE`, `ANSIBLE_CONFIG`, `ANSIBLE_INVENTORY`
- **Inventory:** `inventories/production/hosts.ini`

## How to Use

The user MUST specify a target host or group. There is no default target.

**Command pattern:**

` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible <host-or-group> -m shell -a "<command>"
` ` `

## Available Hosts and Groups

From `inventories/production/hosts.ini`:

| Host/Group | User | IP | Description |
|------------|------|----|-------------|
| `ocean` | terrac | 192.168.1.143 | GPU VM, media/AI services |
| `dns01` | debian | 192.168.1.2 | DNS server |
| `dns02` | debian | 192.168.1.3 | DNS server |
| `node005` | root | 192.168.1.105 | Proxmox bare metal |
| `node006` | root | 192.168.1.106 | Proxmox bare metal |
| `gitlab` | debian | 192.168.1.5 | GitLab server |
| `pihole` | debian | 192.168.1.9 | Pi-hole |
| `gh-runner-01` | debian | 192.168.1.20 | GitHub runner |
| `openclaw` | debian | 192.168.1.31 | OpenClaw |

**Groups:** `proxmox`, `baremetal`, `vms`, `github_runners`, `dns`, `dns_servers`, `k8s`

## Examples

Check disk usage on ocean:
` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible ocean -m shell -a "df -h"
` ` `

Check docker containers on ocean:
` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible ocean -m shell -a "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'"
` ` `

Check uptime on all VMs:
` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible vms -m shell -a "uptime"
` ` `

Restart a service on ocean:
` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && ansible ocean -m shell -a "systemctl restart plex.service"
` ` `
```

- [ ] **Step 3: Verify the skill appears**

```bash
ls ~/.claude/skills/ansible-shell/SKILL.md
```

Expected: file exists.

---

### Task 3: Create docker-logs skill

**Files:**
- Create: `~/.claude/skills/docker-logs/SKILL.md`

- [ ] **Step 1: Create the skill directory**

```bash
mkdir -p ~/.claude/skills/docker-logs
```

- [ ] **Step 2: Write the SKILL.md**

Create `~/.claude/skills/docker-logs/SKILL.md` with this content:

```markdown
---
name: docker-logs
description: Retrieve Docker container logs from remote homelab hosts via SSH. Use when the user asks to check container logs, debug a container, or see what a container is outputting.
---

# Docker Logs via SSH

Retrieve Docker container logs from remote homelab hosts by SSH-ing directly to the host.

## Host Resolution

Resolve the target host from the Ansible inventory at `inventories/production/hosts.ini` in the homelab repo (`/home/terrac/projects/bluefishforsale/homelab`).

Parse the inventory to extract `ansible_user` and `ansible_ssh_host` for the given host name.

**Quick reference:**

| Host | SSH Target |
|------|-----------|
| `ocean` | `terrac@192.168.1.143` |
| `dns01` | `debian@192.168.1.2` |
| `dns02` | `debian@192.168.1.3` |
| `node005` | `root@192.168.1.105` |
| `node006` | `root@192.168.1.106` |
| `gitlab` | `debian@192.168.1.5` |
| `pihole` | `debian@192.168.1.9` |
| `gh-runner-01` | `debian@192.168.1.20` |
| `openclaw` | `debian@192.168.1.31` |

## How to Use

The user MUST specify a container name and a host.

**Command pattern:**

` ` `bash
ssh <user>@<ip> docker logs [flags] <container>
` ` `

**Common flags:**
- `--tail N` — last N lines (default: 100)
- `--since <duration>` — logs since duration (e.g. `1h`, `30m`, `2024-01-01`)
- `--follow` or `-f` — stream logs (use with caution, may need Ctrl-C)
- `--timestamps` or `-t` — show timestamps

## Examples

Last 100 lines of plex on ocean:
` ` `bash
ssh terrac@192.168.1.143 docker logs --tail 100 plex
` ` `

Sonarr logs from the last hour:
` ` `bash
ssh terrac@192.168.1.143 docker logs --since 1h sonarr
` ` `

Last 50 lines of frigate with timestamps:
` ` `bash
ssh terrac@192.168.1.143 docker logs --tail 50 --timestamps frigate
` ` `

## Listing Containers

If the user doesn't know the container name, list running containers first:
` ` `bash
ssh <user>@<ip> docker ps --format 'table {{.Names}}\t{{.Status}}'
` ` `
```

- [ ] **Step 3: Verify the skill appears**

```bash
ls ~/.claude/skills/docker-logs/SKILL.md
```

Expected: file exists.

---

### Task 4: Create plex-logs skill

**Files:**
- Create: `~/.claude/skills/plex-logs/SKILL.md`

- [ ] **Step 1: Create the skill directory**

```bash
mkdir -p ~/.claude/skills/plex-logs
```

- [ ] **Step 2: Write the SKILL.md**

Create `~/.claude/skills/plex-logs/SKILL.md` with this content:

```markdown
---
name: plex-logs
description: Tail the last 1000 lines of the Plex Media Server application log on ocean. Use when the user asks about plex logs, plex errors, or wants to see what plex is doing.
---

# Plex Media Server Log Viewer

Tail the last 1000 lines of the Plex Media Server application log on ocean.

## How to Use

This skill takes no arguments. Just run:

` ` `bash
ssh terrac@192.168.1.143 tail -n1000 '/data01/services/plex/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log'
` ` `

## Details

- **Host:** ocean (`terrac@192.168.1.143`)
- **Log path:** `/data01/services/plex/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log`
- **Lines:** 1000

## After Reading Logs

After retrieving the logs, analyze the output for:
- Errors and warnings
- Transcoding issues (look for "EAE" or "transcode" entries)
- Library scan activity
- Network/connectivity issues
- Database errors
- Authentication or token problems

Summarize findings for the user with specific log lines as evidence.
```

- [ ] **Step 3: Verify the skill appears**

```bash
ls ~/.claude/skills/plex-logs/SKILL.md
```

Expected: file exists.

---

### Task 5: Create debug-service skill

**Files:**
- Create: `~/.claude/skills/debug-service/SKILL.md`

- [ ] **Step 1: Create the skill directory**

```bash
mkdir -p ~/.claude/skills/debug-service
```

- [ ] **Step 2: Write the SKILL.md**

Create `~/.claude/skills/debug-service/SKILL.md` with this content:

```markdown
---
name: debug-service
description: Debug any homelab Docker service by auto-discovering its host and container from playbooks, collecting diagnostics (docker logs, docker inspect, systemd status, app logs), and analyzing the output. Use when the user says "debug <service>", reports a service is down, or asks to investigate a service issue.
---

# Homelab Service Debugger

Composite diagnostic skill. Auto-discovers service details from Ansible playbooks, collects diagnostics from multiple sources, then analyzes the output to identify issues and propose fixes.

## Step 1: Identify the Service

Match the user's request against a service name. If ambiguous, discover all available services and let the user pick.

### Dynamic Service Discovery

Parse playbook YAML files under `playbooks/individual/` in the homelab repo to build a service map.

` ` `bash
cd /home/terrac/projects/bluefishforsale/homelab && grep -rl "^    service:" playbooks/individual/ | while read f; do
  svc=$(grep "^    service:" "$f" | head -1 | awk '{print $2}')
  host=$(grep "^  hosts:" "$f" | head -1 | awk '{print $2}')
  data=$(grep "^    data:" "$f" | head -1 | awk '{print $2}')
  echo "$svc|$host|$data|$f"
done | sort -u
` ` `

This produces lines like:
` ` `
plex|ocean|/data01|playbooks/individual/ocean/media/plex.yaml
sonarr|ocean|/data01|playbooks/individual/ocean/media/sonarr.yaml
frigate|ocean|/data01|playbooks/individual/ocean/services/frigate.yaml
` ` `

### Host Resolution

Once you have the host name (e.g. `ocean`), resolve SSH connection details from `inventories/production/hosts.ini`:

` ` `bash
grep "^<hostname> " /home/terrac/projects/bluefishforsale/homelab/inventories/production/hosts.ini
` ` `

Extract `ansible_user` and `ansible_ssh_host` from the line. Quick reference:

| Host | SSH Target |
|------|-----------|
| `ocean` | `terrac@192.168.1.143` |
| `dns01` | `debian@192.168.1.2` |
| `dns02` | `debian@192.168.1.3` |
| `node005` | `root@192.168.1.105` |
| `node006` | `root@192.168.1.106` |

## Step 2: Collect Diagnostics

Run these four commands via SSH to the resolved host. Run them in parallel where possible.

### 2a. Docker Logs

` ` `bash
ssh <user>@<ip> docker logs --tail 200 <container>
` ` `

### 2b. Docker Inspect

` ` `bash
ssh <user>@<ip> docker inspect <container> --format '{{json .State}}'
` ` `

Key fields to examine:
- `.State.Status` — running, exited, restarting
- `.State.ExitCode` — non-zero means crash
- `.State.RestartCount` — high count means crash loop
- `.State.Health.Status` — healthy, unhealthy, starting
- `.State.Health.Log` — recent health check results

### 2c. Systemd Status

` ` `bash
ssh <user>@<ip> systemctl status <service>.service
` ` `

### 2d. Filesystem Log (Plex Only)

Only for the `plex` service:

` ` `bash
ssh <user>@<ip> tail -n1000 '/data01/services/plex/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log'
` ` `

For all other services, skip this step.

## Step 3: Analyze

After collecting all diagnostic output, analyze for:

1. **Container state:** Is it running? Restarting? Exited?
2. **Health checks:** Passing or failing? What do recent health check logs show?
3. **Error patterns:** Scan docker logs and app logs for ERROR, WARN, Exception, panic, OOM, killed
4. **Restart loops:** High restart count + recent exit = crash loop
5. **Systemd issues:** Is the unit active? Did it fail to start? Any compose errors?
6. **Resource issues:** OOM kills, disk full, GPU errors
7. **Network issues:** Connection refused, timeouts, DNS failures
8. **Configuration issues:** Missing files, permission denied, bad config values

Present findings organized by severity:
- **CRITICAL:** Service is down or crash-looping
- **WARNING:** Health checks failing, errors in logs, approaching limits
- **INFO:** Normal operation, minor warnings

Propose concrete next steps for each finding.

## Error Handling

If any diagnostic command fails (container not found, SSH timeout, unit not found):
- Report what failed and why
- Analyze whatever data was successfully collected
- Do not fail entirely — partial diagnostics are still useful

## Examples

User says "debug plex":
1. Discovery finds: `plex|ocean|/data01`
2. Resolve ocean: `terrac@192.168.1.143`
3. Run all four diagnostic commands (including plex filesystem log)
4. Analyze combined output

User says "debug sonarr":
1. Discovery finds: `sonarr|ocean|/data01`
2. Resolve ocean: `terrac@192.168.1.143`
3. Run three diagnostic commands (no filesystem log for sonarr)
4. Analyze combined output

User says "debug something-unknown":
1. Discovery doesn't match — list all discovered services
2. Ask user to pick one
```

- [ ] **Step 3: Verify the skill appears**

```bash
ls ~/.claude/skills/debug-service/SKILL.md
```

Expected: file exists.

---

### Task 6: Test all skills end-to-end

- [ ] **Step 1: Verify all skill directories exist**

```bash
ls -la ~/.claude/skills/ansible-playbook/SKILL.md ~/.claude/skills/ansible-shell/SKILL.md ~/.claude/skills/docker-logs/SKILL.md ~/.claude/skills/plex-logs/SKILL.md ~/.claude/skills/debug-service/SKILL.md
```

Expected: all five files exist.

- [ ] **Step 2: Verify skill frontmatter is valid**

For each skill, confirm the YAML frontmatter has `name` and `description` fields:

```bash
for s in ansible-playbook ansible-shell docker-logs plex-logs debug-service; do
  echo "=== $s ===" && head -4 ~/.claude/skills/$s/SKILL.md
done
```

Expected: each shows `---`, `name:`, `description:`, `---`.

- [ ] **Step 3: Test service discovery command**

```bash
cd /home/terrac/projects/bluefishforsale/homelab && grep -rl "^    service:" playbooks/individual/ | while read f; do
  svc=$(grep "^    service:" "$f" | head -1 | awk '{print $2}')
  host=$(grep "^  hosts:" "$f" | head -1 | awk '{print $2}')
  data=$(grep "^    data:" "$f" | head -1 | awk '{print $2}')
  echo "$svc|$host|$data|$f"
done | sort -u
```

Expected: list of services including plex, sonarr, radarr, frigate, etc. with their host and data path.

- [ ] **Step 4: Test SSH connectivity to ocean**

```bash
ssh -o ConnectTimeout=5 terrac@192.168.1.143 echo "connected"
```

Expected: `connected`

- [ ] **Step 5: Test plex-logs command**

```bash
ssh terrac@192.168.1.143 tail -n5 '/data01/services/plex/config/Library/Application Support/Plex Media Server/Logs/Plex Media Server.log'
```

Expected: 5 lines of Plex log output (just verifying the path is valid).

- [ ] **Step 6: Test docker inspect on plex**

```bash
ssh terrac@192.168.1.143 docker inspect plex --format '{{json .State}}' | head -1
```

Expected: JSON with Status, ExitCode, Health fields.
