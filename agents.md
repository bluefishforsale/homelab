# Homelab Infrastructure — Agent Reference

Ansible-driven homelab managing Docker services, GPU passthrough, and CI/CD across a multi-host Proxmox cluster.

---

## Quick Commands

**Prerequisite:** Source the environment before running any commands:
```bash
source .envrc
```
This sets `ANSIBLE_VAULT_PASSWORD_FILE`, `ANSIBLE_CONFIG`, and `ANSIBLE_INVENTORY`.

### Validation & Testing
```bash
# Run all validation checks
make validate

# Individual checks
make validate-yaml          # YAML syntax validation
make validate-ansible       # Ansible syntax check
make validate-templates     # Jinja2 template validation
make security-scan          # Check for hardcoded secrets
make check-vault            # Verify vault encryption
make lint-ansible           # Ansible linting (optional)

# Test a single playbook
ansible-playbook --syntax-check playbooks/individual/ocean/media/plex.yaml
ansible-playbook --check playbooks/individual/ocean/media/plex.yaml  # Dry run
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/media/plex.yaml
```

### Deployment Commands
```bash
# Deploy full infrastructure
ansible-playbook -i inventories/production/hosts.ini playbooks/00_site.yaml

# Deploy ocean services only
ansible-playbook -i inventories/production/hosts.ini playbooks/03_ocean_services.yaml

# Deploy single service
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/media/plex.yaml

# Deploy with specific tags
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/ai/comfyui.yaml --tags models

# Deploy from specific branch (for git-based services)
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/services/terrac_com_static.yaml -e "git_branch=develop"
```

---

## Code Style Guidelines

### Ansible Playbooks

**1. File Structure**
- Use YAML format with `.yaml` extension (not `.yml`)
- Start every playbook with `---`
- Include descriptive header comments explaining purpose, tags, and usage examples
- Use 2-space indentation consistently

**2. Naming Conventions**
```yaml
# Playbook names: Use descriptive, action-oriented names
- name: Deploy Plex Media Server
- name: Configure nginx reverse proxy
- name: Sync built files to web root

# Variables: Use snake_case
service: plex
home_directory: /data01/services/plex
docker_image_version: latest

# Hosts: Use inventory names exactly
hosts: ocean
hosts: dns01
```

**3. Module Usage**
```yaml
# REQUIRED: Use fully qualified collection names (FQCN)
ansible.builtin.file:
ansible.builtin.template:
ansible.builtin.systemd:
community.docker.docker_compose:

# BAD - Don't use short names
file:
template:
```

**4. Variable References**
```yaml
# Use Jinja2 templating for all variables
path: "{{ home }}/config"
port: "{{ service_ports.plex.port }}"

# Access vault secrets
api_key: "{{ cloudflare.api_token }}"
password: "{{ media_services.plex.api_key }}"

# Use lookup for environment variables with defaults
git_branch: "{{ lookup('env', 'GIT_BRANCH') | default('main', true) }}"
```

**5. Task Organization**
```yaml
tasks:
  # 1. Display information
  - name: Display deployment information
    ansible.builtin.debug:
      msg: "Deploying {{ service }}"
    tags: [always]

  # 2. Create directories
  - name: Ensure base directory exists
    ansible.builtin.file:
      path: "{{ home }}"
      state: directory
      mode: '0755'
    tags: [setup]

  # 3. Deploy templates
  - name: Deploy docker-compose template
    ansible.builtin.template:
      src: "{{ files }}/docker-compose.yml.j2"
      dest: "{{ home }}/docker-compose.yml"
      mode: '0644'
    notify: Restart service

  # 4. Enable services
  - name: Enable systemd service
    ansible.builtin.systemd:
      name: "{{ service }}.service"
      enabled: yes
      state: started

handlers:
  - name: Reload systemd daemon
    ansible.builtin.systemd:
      daemon_reload: yes

  - name: Restart service
    ansible.builtin.systemd:
      name: "{{ service }}.service"
      state: restarted
```

**6. Error Handling**
```yaml
# AVOID: Don't suppress errors without justification
- name: Do something
  shell: command
  ignore_errors: true  # ❌ BAD

# GOOD: Check state explicitly
- name: Check if service exists
  ansible.builtin.shell: systemctl status servicename
  register: service_check
  failed_when: false      # Expected to fail if not exists
  changed_when: false     # Never reports as changed
  tags: [check]

- name: Start service only if it exists
  ansible.builtin.systemd:
    name: servicename
    state: started
  when: service_check.rc == 0

# ACCEPTABLE: Ignore errors for build steps with pre-built fallback
- name: Build the application
  ansible.builtin.command:
    cmd: npm run build
  register: build_result
  ignore_errors: true  # OK - Uses pre-built dist/ if build fails
  tags: [build]
```

**7. Idempotency**
- Every playbook must be safe to run multiple times
- Use `state:` parameters correctly (present/absent/started/stopped)
- Use `changed_when:` to prevent false positive changes
- Check state before making destructive changes

---

## File Organization

### Standard Service Structure
```
playbooks/individual/ocean/<category>/<service>.yaml
files/<service>/
  ├── docker-compose.yml.j2
  ├── <service>.service.j2
  └── <service>.env.j2
```

### Standard Playbook Template
```yaml
---
# <Service Name> - Brief description
#
# Available tags:
#   --tags setup    Description
#   --tags deploy   Description
#
# Usage:
#   ansible-playbook -i inventories/production/hosts.ini playbooks/path/to/playbook.yaml
#
- name: Configure <Service>
  hosts: ocean
  become: true
  gather_facts: true

  vars_files:
    - ../../../../vault/secrets.yaml
    - ../../../../vars/vars_service_ports.yaml

  vars:
    service: <name>
    port: "{{ service_ports.<service>.port }}"
    home: "/data01/services/{{ service }}"
    user: media
    uid: 1001
    gid: 1001

  tasks:
    # Tasks here

  handlers:
    # Handlers here
```

---

## Important Conventions

**Paths & Storage**
- All services: `/data01/services/<service>/`
- Systemd units: `/etc/systemd/system/<service>.service`
- Templates: `files/<service>/<template>.j2`

**Users & Permissions**
- Media services: `media:1001`
- System user: `terrac:1002`
- Directory mode: `0755`
- File mode: `0644`
- Executable mode: `0755`

**Ports**
- Define in `vars/vars_service_ports.yaml`
- Access via `{{ service_ports.<service>.port }}`
- Never hardcode ports in playbooks

**Secrets**
- Store in `vault/secrets.yaml` (encrypted)
- Never commit unencrypted secrets
- Access via `{{ vault_var.sub_var }}`

**Docker**
- Use Docker Compose for all services
- Manage via systemd services (Type=oneshot)
- Use journald logging driver
- Always set resource limits
- Always include health checks
- Pin image versions (no `latest` tags in production)

---

## Testing Workflow

1. **Before committing:**
   ```bash
   make validate-yaml
   ansible-playbook --syntax-check playbooks/path/to/playbook.yaml
   ```

2. **After changes:**
   ```bash
   ansible-playbook --check playbooks/path/to/playbook.yaml  # Dry run
   ```

3. **Deploy:**
   ```bash
   ansible-playbook -i inventories/production/hosts.ini playbooks/path/to/playbook.yaml
   ```

4. **Verify idempotency:**
   ```bash
   # Run twice - second run should show 0 changed
   ansible-playbook -i inventories/production/hosts.ini playbooks/path/to/playbook.yaml
   ansible-playbook -i inventories/production/hosts.ini playbooks/path/to/playbook.yaml
   ```

---

## Common Patterns

### Clean Git Deployment Pattern
```yaml
- name: Always remove existing git repository for clean deployment
  ansible.builtin.file:
    path: "{{ git_clone_path }}"
    state: absent

- name: Recreate repo directory with correct ownership
  ansible.builtin.file:
    path: "{{ git_clone_path }}"
    state: directory
    owner: terrac
    group: terrac
    mode: '0755'

- name: Clone git repository
  ansible.builtin.git:
    repo: "{{ git_repo }}"
    dest: "{{ git_clone_path }}"
    version: "{{ git_branch }}"
    force: yes
  become_user: terrac
```

### Docker Compose with Systemd Pattern
```yaml
- name: Deploy docker-compose template
  ansible.builtin.template:
    src: "{{ files }}/docker-compose.yml.j2"
    dest: "{{ home }}/docker-compose.yml"
  notify: Restart service

- name: Deploy systemd service
  ansible.builtin.template:
    src: "{{ files }}/{{ service }}.service.j2"
    dest: "/etc/systemd/system/{{ service }}.service"
  notify:
    - Reload systemd daemon
    - Restart service

- name: Enable and start service
  ansible.builtin.systemd:
    name: "{{ service }}.service"
    enabled: yes
    state: started
```

---

## Troubleshooting

**Syntax errors:** Check YAML indentation (2 spaces, no tabs)  
**Undefined variables:** Verify vars_files are loaded and vault is decrypted  
**Permission denied:** Check user/group ownership and become_user  
**Service won't start:** Check systemd logs: `journalctl -u <service>.service`  
**Git ownership issues:** Use clean deployment pattern (remove + recreate)

---

## CI/CD Integration

All changes are validated via GitHub Actions:
- YAML syntax validation
- Ansible syntax check
- Template validation
- Security scanning
- Automated deployment on merge to main

Critical services (DNS, DHCP, Plex) require manual approval before deployment.

---

## Hardware & Infrastructure

**Node006 (Dell R720):** 40 cores, 680GB RAM, 64TB ZFS RAID2, RTX 3090 24GB, Proxmox → ocean VM
**Node005 (Dell R620):** 56 cores, 128GB RAM, 1TB SSD, Proxmox → dns01, pihole, k8s, GitHub runners

**Ocean IP:** 192.168.1.143 (primary service host)

---

## Development Preferences

- **Spec first**: Write detailed specs before coding, review for resilience/performance/recovery
- **Go over Python**: Better performance, lower memory for services
- **PostgreSQL over SQLite**: Dedicated DB containers, not embedded
- **IP addresses over hostnames**: Use 192.168.1.143:PORT, not container names (DNS fragility)
- **Isolation**: Separate containers and playbooks per service
- **Graceful degradation**: Services must work when dependencies fail
- **Dead letter queues**: Never lose data, buffer in Redis
- **Circuit breakers**: Backoff to prevent cascade failures
- **Health checks**: Return healthy/degraded/unhealthy with component status
- **Auto-recovery**: Replay buffered data on startup
- **Retry with backoff**: 3 attempts, exponential, then dead letter

---

## Docker Network Architecture

**web_proxy network** (172.20.0.0/16): Created by nginx for reverse proxy container communication
- Containerized services use container names (grafana:3000, comfyui:8188)
- Host services use IP addresses (192.168.1.143:PORT)

**Logging:** Docker → journald → Promtail → Loki (daemon.json: `"log-driver": "journald"`, `"tag": "{{.Name}}"`)

---

## Systemd Service Template (Critical Fixes)

```ini
[Unit]
Description={{ service }}
After=network-online.target docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
Environment=PATH=/usr/local/bin:/usr/bin:/bin
Environment=COMPOSE_PROJECT_NAME={{ service }}
Environment=COMPOSE_HTTP_TIMEOUT=60
ExecStartPre=/bin/bash -c 'cd {{ home }} && /usr/bin/docker compose down'
ExecStartPre=-/bin/bash -c 'cd {{ home }} && /usr/bin/docker compose pull --quiet'
ExecStart=/bin/bash -c 'cd {{ home }} && /usr/bin/docker compose up -d'
ExecStop=/bin/bash -c 'cd {{ home }} && /usr/bin/docker compose down'
ExecReload=/bin/bash -c 'cd {{ home }} && /usr/bin/docker compose restart'
NoNewPrivileges=true
ReadWritePaths={{ home }}
ReadOnlyPaths=/var/run/docker.sock

[Install]
WantedBy=multi-user.target
```

**Key fixes:** Use `bash -c` with `cd` (not `-f` flag), full paths to `/usr/bin/docker compose`, remove `PrivateTmp`/`ProtectSystem`/`ProtectHome` (causes namespace mount failures).

---

## Go Service Structure

```
files/{service-name}/
├── docker-compose.yml.j2
├── {service-name}.service.j2
└── go/
    ├── Dockerfile
    ├── go.mod
    ├── main.go        # Entry point, HTTP handlers, config
    ├── types.go       # Shared types/structs
    ├── metrics.go     # Prometheus metrics (promauto)
    └── {component}.go # Component logic
```

Required: health check (healthy/degraded/unhealthy), Prometheus promauto metrics, graceful shutdown with context cancellation, config via `getEnv()`/`getEnvInt()`/`getEnvFloat()` helpers.

---

## Container Data Directory Permissions

| Container | UID:GID | Mode |
|-----------|---------|------|
| Redis | 999:999 | 0755 |
| PostgreSQL | 999:999 | 0700 |
| Media services | 1001:1001 | 0755 |
| System (terrac) | 1002:1002 | 0755 |

Centralized uid/gid in vault (`vault/secrets.yaml`):
```yaml
system_users:
  media: { uid: 1001, gid: 1001 }
  terrac: { uid: 1002, gid: 1002 }
  debian: { uid: 1003, gid: 1003 }
```

Vault variable pattern: `{{ log_anomaly_ml.postgres_password }}` (nested, not flat `vault_` prefix). Use `| default('value')` for optional secrets.

---

## DNS Stack Architecture

```
Client → dnsdist (:53) → pdns-recursor (:5353) → upstream (8.8.8.8, 8.8.4.4)
                       → powerdns-auth (:5300)  → .home, cluster.local, reverse zones
```

- dns01=192.168.1.2, dns02=192.168.1.3, all PowerDNS containers use network_mode: host
- dnssec=off in recursor (internal zones unsigned)
- DHCP HA: dns02=primary, dns01=standby, TSIG zone replication
- kea-ctrl-agent on port 8001 (not 8000, conflicts with HA hook)
- Deploy: `ansible-playbook -i inventories/production/hosts.ini playbooks/individual/core/services/dns_ha_stack.yaml`

---

## GPU Troubleshooting

**Plex GPU Transcoding (LD_LIBRARY_PATH)**
NVIDIA CUDA libs at `/usr/lib/x86_64-linux-gnu/nvidia/current/` not in default search path. Fix:
```yaml
environment:
  LD_LIBRARY_PATH: /usr/lib/x86_64-linux-gnu/nvidia/current:/usr/lib/plexmediaserver/lib
```

**llamacpp GPU Layer Offloading**
`runtime: nvidia` alone doesn't guarantee offloading. Use deploy.resources.reservations:
```yaml
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          count: all
          capabilities: [gpu, compute, utility]
command: ["--n-gpu-layers", "41"]  # Explicit count, not "-1"
environment:
  - NVIDIA_VISIBLE_DEVICES=all
  - NVIDIA_DRIVER_CAPABILITIES=all
  - CUDA_VISIBLE_DEVICES=0
```

---

## Cloudflare Access Policies

- Use **PUT** (not PATCH) to replace all policies — prevents mixed legacy + reusable policies
- Vault email groups:
```yaml
cloudflare:
  access_allow_emails:
    admin: ["terracnosaur@gmail.com"]
    plex-users: ["terracnosaur@gmail.com", "family@gmail.com"]
```

---

## Service Catalog

**Infrastructure:** mysql, nginx, cloudflare_ddns, cloudflared
**Media:** plex, sonarr, radarr, nzbget, prowlarr, bazarr, tautulli, overseerr, tdarr, audible-downloader
**AI/ML:** llamacpp, open_webui, comfyui, n8n
**Cloud:** nextcloud
**Monitoring:** prometheus, grafana, loki, log-anomaly-detector

**Deployment Phases:**
1. Infrastructure Foundation (base, storage, database, web server)
2. Network Services (DNS, tunnels)
3. Media Stack (Arr suite)
4. AI/ML & Cloud (GPU pipeline, NextCloud)
5. Optional (transcoding, audiobooks, monitoring)

**Access:** `.home` internal, `.terrac.com` external via Cloudflare Zero Trust

---

## CI/CD Automation Details

**PR Flow:** PR Opened → Provision Test VM → Test Playbooks → Post Results → Destroy VM
**Merge Flow:** Merge to Main → Detect Changes → Apply to Production → Generate Summary

**GitHub Secrets** (must be **Repository** secrets, not Environment):
- `ANSIBLE_VAULT_PASSWORD`: Plain text
- `PROXMOX_SSH_KEY`: Base64-encoded SSH key for root@node005.home

**Test VM:** Proxmox node005, template VMID 9999, VMID = 8000 + (PR_NUMBER % 1000), 4 cores, 2GB RAM

---

## Log Anomaly Detector

- Alert dedup: 1-hour cooldown per unique key (host:service:type:description)
- Severity filtering: MIN_SEVERITY=high (only high/critical sent)
- Thresholds: frequency_sigma=4.0, entropy=6.5, levenshtein=0.90, min_repetition=20
- Suppressed services: blackbox-exporter, promtail, node-exporter
- Structured alerts: host, service, severity, impact, actions[], log_sample
- Deploy: `ansible-playbook playbooks/individual/ocean/log_anomaly_detector_go.yaml`

---

## Grafana + MySQL Consolidated Stack

MySQL consolidated into Grafana docker-compose (MySQL only serves Grafana):
- **grafana_internal** network: Grafana ↔ MySQL (private, no host exposure)
- **web_proxy** network: nginx ↔ Grafana
- MySQL: percona/percona-server:5.7, 1 CPU, 1GB, buffer_pool=512M
- Storage: `/data01/services/grafana/{mysql-data,mysql-logs,mysql-conf,data,logs}/`
- Deploy: single playbook manages both containers
