# Homelab Infrastructure — Agent Reference

Ansible-driven homelab managing Docker services, GPU passthrough, and CI/CD across a multi-host Proxmox cluster.

---

## Hardware

| Host | IP | Hardware | Purpose |
|------|----|----------|---------|
| node005 | 192.168.1.105 | Dell R620 — 56 cores, 128GB RAM | Proxmox — control VMs |
| node006 | 192.168.1.106 | Dell R720 — 40 cores, 680GB RAM, RTX 3090 24GB | Proxmox — ocean VM |
| ocean | 192.168.1.143 | VM on node006 — 30 cores, 256GB RAM, GPU passthrough | Primary Docker services host |
| dns01 | 192.168.1.2 | VM on node005 | BIND DNS |
| pihole | 192.168.1.9 | VM on node005 | DNS filtering |
| gitlab | 192.168.1.5 | VM on node005 | CI/CD |
| gh-runner-01 | 192.168.1.20 | VM on node005 | GitHub Actions runners |

**Storage**: 64TB ZFS raidz2 pool (`data01`) on node006, passed through to ocean. All services mount `/data01/services/<name>/`.

---

## Repository Structure

```text
homelab/
├── playbooks/
│   ├── 00_site.yaml                 # Full infrastructure deployment
│   ├── 01_base_system.yaml          # Base: packages, users, sysctl, logging, fail2ban
│   ├── 02_core_infrastructure.yaml  # Core: ethtool, Docker, DNS, DHCP, Pi-hole
│   ├── 03_ocean_services.yaml       # Ocean: all ~30 application services
│   ├── tasks/                       # Reusable task includes
│   └── individual/                  # Per-service playbooks
│       ├── base/                    #   System-level (packages, users, logging)
│       ├── core/                    #   Network, storage, DNS/DHCP
│       ├── infrastructure/          #   Docker, GPU, exporters, runners
│       └── ocean/                   #   ai/, media/, monitoring/, network/, services/
├── files/                           # Jinja2 templates per service (docker-compose, systemd, configs)
├── vars/                            # Shared variables
│   ├── vars_service_ports.yaml      #   Centralized port assignments
│   ├── vars_cloudflared.yaml        #   Tunnel ingress rules
│   ├── vars_llamacpp_models.yaml    #   LLM model definitions
│   ├── vars_comfyui_models.yaml     #   Image model definitions
│   ├── vars_log_anomaly.yaml        #   Log anomaly detector config
│   └── vars_users.yaml              #   User/group definitions
├── vault/
│   ├── secrets.yaml                 # Encrypted secrets (ansible-vault)
│   └── secrets.yaml.template        # Secret structure reference
├── inventories/production/hosts.ini # Host inventory with per-host vars
├── roles/                           # Ansible roles (github_docker_runners)
├── docs/                            # Architecture, operations, setup, troubleshooting
├── .github/workflows/               # CI/CD pipelines
├── Makefile                         # Validation targets (validate, lint, security-scan)
└── ansible.cfg                      # Ansible configuration
```

---

## Deployment Pattern

Every service follows the same pattern:

1. **Playbook** (`playbooks/individual/ocean/<category>/<service>.yaml`) — targets `hosts: ocean`, loads vault + vars, defines service-specific variables
2. **Templates** (`files/<service>/`) — Jinja2 templates for `docker-compose.yml.j2`, `<service>.service.j2` (systemd), `<service>.env.j2`
3. **Execution flow**: Create directories → deploy templates → enable systemd service → handlers restart on config change

### Standard playbook structure

```yaml
- name: Configure <service>
  hosts: ocean
  become: true
  gather_facts: true

  vars_files:
    - ../../../../vault/secrets.yaml
    - ../../../../vars/vars_service_ports.yaml

  vars:
    service: <name>
    image: <docker-image>
    version: <tag>
    port: "{{ service_ports.<service>.port }}"
    home: "/data01/services/{{ service }}"
    user: media
    uid: 1001
    gid: 1001

  tasks:
    - name: Create directories
      # ...
    - name: Deploy docker-compose template
      # notify: restart handler
    - name: Deploy systemd service template
      # notify: restart handler
    - name: Enable service
      # ...

  handlers:
    - name: Reload systemd daemon
    - name: Restart <service> service
```

### Standard systemd service template

```ini
[Service]
Type=oneshot
RemainAfterExit=yes
Environment=PATH=/usr/local/bin:/usr/bin:/bin
ExecStartPre=/bin/bash -c 'cd {{ home }} && /usr/bin/docker compose down --remove-orphans'
ExecStart=/bin/bash -c 'cd {{ home }} && /usr/bin/docker compose up -d'
ExecStop=/bin/bash -c 'cd {{ home }} && /usr/bin/docker compose down'
NoNewPrivileges=true
```

---

## Services on Ocean (192.168.1.143)

### Network

| Service | Port | Playbook |
|---------|------|----------|
| nginx | 80, 443 | `ocean/network/nginx_compose.yaml` |
| cloudflared | — | `ocean/network/cloudflared.yaml` |
| cloudflare_ddns | — | `ocean/network/cloudflare_ddns.yaml` |

### AI/ML (RTX 3090 GPU)

| Service | Port | Playbook |
|---------|------|----------|
| llama.cpp | 8080 | `ocean/ai/llamacpp.yaml` |
| Open WebUI | 3000 | `ocean/ai/open_webui.yaml` |
| ComfyUI | 8188 | `ocean/ai/comfyui.yaml` |

### Media

| Service | Port | Playbook |
|---------|------|----------|
| Plex | 32400 | `ocean/media/plex.yaml` |
| Sonarr | 8902 | `ocean/media/sonarr.yaml` |
| Radarr | 8903 | `ocean/media/radarr.yaml` |
| Prowlarr | 9696 | `ocean/media/prowlarr.yaml` |
| Bazarr | 6767 | `ocean/media/bazarr.yaml` |
| NZBGet | 8901 | `ocean/media/nzbget.yaml` |
| Overseerr | 5055 | `ocean/media/overseerr.yaml` |
| Tautulli | 8905 | `ocean/media/tautulli.yaml` |
| Tdarr | 8265 | `ocean/media/tdarr.yaml` |

### Monitoring

| Service | Port | Playbook |
|---------|------|----------|
| Prometheus | 9090 | `ocean/monitoring/prometheus.yaml` |
| Grafana (+ MySQL) | 8910 | `ocean/monitoring/grafana_compose.yaml` |
| NVIDIA DCGM | 9445 | `ocean/monitoring/nvidia_dcgm.yaml` |
| UnPoller | 9130 | `ocean/monitoring/unpoller.yaml` |

### Application Services

| Service | Port | Playbook |
|---------|------|----------|
| NextCloud | 8081 | `ocean/services/nextcloud.yaml` |
| TinaCMS | 8084 | `ocean/services/tinacms.yaml` |
| Frigate | 5000 | `ocean/services/frigate.yaml` |
| Home Assistant | 8123 | `ocean/services/homeassistant_compose.yaml` |
| Audible Downloader | — | `ocean/services/audible_downloader.yaml` |

All playbook paths above are relative to `playbooks/individual/`.

---

## Port Map

All ports defined in `vars/vars_service_ports.yaml`. Access in playbooks via `{{ service_ports.<service>.port }}`.

| Category | Service | Port |
|----------|---------|------|
| **Infrastructure** | nginx | 80, 443 |
| | mysql | 3306 |
| **Monitoring** | prometheus | 9090 |
| | alertmanager | 9093 |
| | karma | 8181 |
| | grafana | 8910 |
| | loki | 3100 |
| **Exporters** | blackbox_exporter | 9115 |
| | ndt_exporter | 9140 |
| | cadvisor | 8912 |
| | node_exporter | 9100 |
| | process_exporter | 9256 |
| | nvidia_gpu_exporter | 9445 |
| | smart_exporter | 9633 |
| | zfs_exporter | 9150 |
| | fail2ban_exporter | 9191 |
| | unpoller | 9130 |
| | plex_exporter | 9594 |
| | jellyfin_exporter | 9027 |
| | tautulli_exporter | 8913 |
| | sonarr_exporter | 9708 |
| | radarr_exporter | 9707 |
| | prowlarr_exporter | 9710 |
| | nzbget_exporter | 9452 |
| **Log Anomaly** | log_anomaly_detector | 8086 |
| | log_anomaly_ml | 8087 |
| **AI/ML** | llamacpp | 8080 |
| | open_webui | 3000 |
| | comfyui | 8188 |
| | ai_corp | 8088 |
| **Media** | plex | 32400 (+ TCP 3005, 8324, 32469 / UDP 32410-32414) |
| | jellyfin | 8096 (HTTPS 8920 / UDP 7359, 1900) |
| | jellystat | 8097 |
| | tautulli | 8905 |
| | sonarr | 8902 |
| | radarr | 8903 |
| | bazarr | 6767 |
| | prowlarr | 9696 |
| | overseerr | 5055 |
| | nzbget | 8901 |
| | tdarr | 8265 |
| | navidrome | 4533 |
| | spotube | 3002 |
| **Cloud** | nextcloud | 8081 |
| | tinacms | 8084 |
| | homepage | 8089 |
| | wordpress | 8085 |
| **NVR** | frigate | 5000 (RTMP 1935 / RTSP 8554 / WebRTC 8555) |

---

## Networking

- **Flat subnet**: 192.168.1.0/24 — all hosts on one network
- **Internal DNS**: BIND on dns01 serves `.home` domain
- **External access**: Cloudflare tunnels (`*.terrac.com`) — no port forwarding
- **Traffic flow**: Internet → Cloudflare → cloudflared → nginx → service
- **Service connections**: Use host IP (`192.168.1.143:PORT`), not container hostnames
- **nginx bridge network**: Custom Docker bridge (`172.25.0.0/16`, gateway `172.25.0.1`) for container-to-host routing
- **Docker logging**: journald driver → Promtail → Loki

---

## Secrets Management

All secrets live in `vault/secrets.yaml` (ansible-vault encrypted). Structure documented in `vault/secrets.yaml.template`.

Top-level vault keys:
- `cloudflare` — API credentials, tunnel certs, access email lists
- `gitlab` — root credentials, tokens
- `databases` — MySQL/PostgreSQL passwords
- `media_services` — API keys for Plex, Sonarr, Radarr, Prowlarr, Bazarr, Tautulli, NZBGet
- `ai_services` — llama.cpp, Open WebUI, Claude API keys
- `cloud_services` — NextCloud credentials
- `monitoring` — Grafana, Prometheus passwords
- `smtp` — Gmail app password for alerts
- `development` — GitHub token, Docker registry credentials

Access in playbooks: `{{ cloudflare.api_token }}`, `{{ media_services.plex.api_key }}`, etc.

---

## Running Playbooks

```bash
# Full infrastructure
ansible-playbook -i inventories/production/hosts.ini playbooks/00_site.yaml

# Ocean services only
ansible-playbook -i inventories/production/hosts.ini playbooks/03_ocean_services.yaml

# Single service
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/media/plex.yaml

# Dry run
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/media/plex.yaml --check

# With tags
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/ai/comfyui.yaml --tags models
```

Vault password is set via environment (`ANSIBLE_VAULT_PASSWORD_FILE` or `ANSIBLE_VAULT_PASSWORD`). Do not use `--ask-vault-pass`.

---

## CI/CD

GitHub Actions workflows in `.github/workflows/`:

| Workflow | Trigger | Action |
|----------|---------|--------|
| `ci-validate.yml` | Push/PR | YAML, Ansible syntax, template validation |
| `pr-test.yml` | PR opened | Provision ephemeral VM on Proxmox, test playbooks |
| `pr-cleanup.yml` | PR closed | Destroy test VM |
| `main-apply.yml` | Merge to main | Deploy changed playbooks to production |
| `deploy-services.yml` | Manual | Deploy master playbooks |
| `deploy-ocean-service.yml` | Manual | Deploy individual ocean service |
| `deploy-critical-service.yml` | Manual | Deploy DNS/DHCP/Plex (requires approval) |

**Runners**: 4 ephemeral self-hosted Docker runners on gh-runner-01 (192.168.1.20). Labels: `self-hosted`, `homelab`, `ansible`.

**Required GitHub secrets**: `ANSIBLE_VAULT_PASSWORD` (plain text), `PROXMOX_SSH_KEY` (base64-encoded).

---

## Validation

```bash
make validate          # All checks (YAML, Ansible, templates, secrets, vault)
make validate-yaml     # YAML syntax
make validate-ansible  # Ansible --syntax-check on all playbooks
make validate-templates # Jinja2 template parsing
make security-scan     # Grep for hardcoded secrets
make check-vault       # Verify vault files are encrypted
make lint-ansible      # ansible-lint (optional)
```

---

## Conventions

- **Idempotent playbooks** — safe to run multiple times; check state before changing
- **Docker Compose + systemd** — every service uses this pattern
- **Official Docker images** — prefer upstream over community
- **IP addresses over hostnames** — avoid DNS fragility
- **Centralized ports** — all in `vars/vars_service_ports.yaml`
- **User mapping** — `media:1001`, `terrac:1002` managed via vault
- **Resource limits** — CPU, RAM, storage limits on every container
- **Health checks** — required on all containers
- **No error suppression** — avoid `ignore_errors: true` without justification
- **Handlers for restarts** — only restart services when config actually changes
- **Bridge networking** — automatic subnet assignment, no custom Docker networks except nginx bridge
- **Storage path** — `/data01/services/<service>/` for all persistent data

---

## Deployment Order

Defined in `playbooks/03_ocean_services.yaml`:

1. **Network** — cloudflare_ddns → cloudflared → nginx
2. **AI/ML** — llama.cpp → Open WebUI → ComfyUI
3. **Media** — Plex → Sonarr → Radarr → Prowlarr → Bazarr → NZBGet → Overseerr → Tautulli → Tdarr
4. **Monitoring** — Prometheus → Grafana → NVIDIA DCGM → UnPoller
5. **Services** — NextCloud → TinaCMS → Audible Downloader → Frigate → Home Assistant
