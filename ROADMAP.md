# Roadmap

Status and future plans for the homelab infrastructure.

---

## Vision

| Status | Goal |
|--------|------|
| In Progress | Fully automated - Ansible playbooks + GitHub Actions |
| Done | SSH free for 99% of tasks - Cloudflare tunnels |
| Done | Publicly signed certs - Let's Encrypt via Cloudflare |
| Done | Git driven IaC - All playbooks in Git, idempotent |
| Done | Git driven builds - GitHub Actions with self-hosted runners |
| Done | Local LLM - llama.cpp + Open WebUI with RTX 3090 |
| Done | Log aggregation - Loki + Promtail + custom dashboards |
| Done | Service discovery - Docker DNS via web_proxy network |
| Done | Intelligent alerting - Go-based log anomaly detector |
| Todo | Control plane UI - Semaphore or Rundeck |
| Todo | HA for critical components |
| Todo | DR plan with recovery procedures |
| Todo | Backup automation - restic or borgbackup |

---

## Completed

### Infrastructure

- **Proxmox cluster**: node005 (56 cores, 128GB) + node006 (40 cores, 680GB, RTX 3090)
- **Ocean VM**: Primary service host on node006 with GPU passthrough
- **Storage**: 64TB ZFS RAID2 pool (data01) with automated permissions
- **DNS**: BIND9 (dns01) + Pi-hole for ad blocking
- **Reverse proxy**: nginx with Docker Compose + web_proxy network
- **Tunnels**: Cloudflare tunnels with Zero Trust Access policies
- **Dynamic DNS**: Cloudflare DDNS for home IP updates
- **GitHub runners**: Self-hosted ephemeral Docker runners (gh-runner-01)
- **Network optimization**: Multiqueue (128 queues) for VMs and interfaces

### Media Stack

- **Plex**: Media server with RTX 3090 hardware transcoding (LD_LIBRARY_PATH fix)
- **Sonarr/Radarr**: TV and movie automation
- **Prowlarr**: Indexer management
- **Bazarr**: Subtitle automation
- **NZBGet**: Usenet downloader
- **Overseerr**: Media request management
- **Tautulli**: Plex statistics and monitoring
- **Tdarr**: Automated media transcoding
- **Plex Meta Manager**: Library metadata automation
- **Audible Downloader**: Audiobook acquisition

### AI/ML Stack

- **llama.cpp**: GPU-accelerated LLM API (Qwen3-14B, 40K context, full GPU offload)
- **Open WebUI**: Chat interface auto-configured for llama.cpp
- **ComfyUI**: AI image generation with YanWenKun image (cu126-slim)

### Monitoring Stack

- **Prometheus**: Metrics collection with node-exporter, NVIDIA DCGM
- **Grafana**: Dashboards with consolidated MySQL backend
- **Loki**: Log aggregation with custom homelab dashboards
- **Promtail**: Log shipping from systemd journal
- **Log Anomaly Detector**: Go-based intelligent alerting with pattern matching
- **UnPoller**: UniFi network metrics
- **NVIDIA DCGM**: GPU metrics exporter

### Cloud/CMS Services

- **NextCloud**: File sharing with MariaDB + Redis backend
- **TinaCMS**: Git-backed CMS (Docker Hub private image)
- **WordPress**: Blog platform with dedicated database
- **Frigate**: NVR with object detection
- **Home Assistant**: Smart home automation

### Automation & Security

- **Ansible vault**: Centralized secrets with uid/gid management
- **Docker Compose + systemd**: Standard deployment pattern
- **GitHub Actions CI/CD**: PR testing on ephemeral VMs, auto-deploy on merge
- **Cloudflare Access**: Reusable policies (admin, plex-users, public)
- **Docker journald logging**: Centralized container logs
- **Idempotent playbooks**: Safe multiple runs

---

## In Progress

| Task | Status |
|------|--------|
| AlertManager | Rules defined, needs deployment |
| Strapi CMS | Playbook ready, needs vault secrets |
| PayloadCMS | Playbook ready, needs vault secrets |

---

## Todo

### Phase 1: Alerting & Notifications

- **AlertManager deployment**: Route alerts to Discord/Slack/email
- **Karma dashboard**: Alert visualization and silencing
- **Uptime Kuma**: External uptime monitoring
- **Healthchecks.io integration**: Cron job monitoring

### Phase 2: Control Plane

- **Ansible Semaphore**: Web UI for playbook execution
- **Portainer**: Docker management UI
- **Homepage dashboard**: Service status overview

### Phase 3: Backup & DR

- **Restic/Borgbackup**: Automated backups to offsite storage
- **Proxmox Backup Server**: VM backup automation
- **DR playbook**: Documented recovery procedures
- **Config backup**: Automated service config exports

### Phase 4: Security Hardening

- **Fail2ban**: SSH and service protection with Prometheus exporter
- **CrowdSec**: Collaborative security
- **Trivy**: Container vulnerability scanning
- **Gatus**: Endpoint health monitoring

### Phase 5: Network & HA

- **WireGuard VPN**: Secure remote access
- **Proxmox HA**: Automatic VM failover
- **Keepalived**: Virtual IPs for critical services
- **DNS HA**: Secondary DNS with zone transfers

### Phase 6: Advanced Features

- **Kubernetes cluster**: K3s on dedicated VMs
- **Gitea/Forgejo**: Self-hosted Git (GitLab alternative)
- **Navidrome**: Music streaming server
- **Immich**: Photo management (Google Photos replacement)
- **Paperless-ngx**: Document management
- **Vaultwarden**: Password manager
- **Authentik**: SSO and identity provider
- **Arr Discord bot**: Media request notifications

---

## Service Quick Reference

| Service | Internal URL | External URL |
|---------|--------------|--------------|
| Plex | http://192.168.1.143:32400 | https://plex.terrac.com |
| Grafana | http://grafana.home | https://grafana.terrac.com |
| Prometheus | http://192.168.1.143:9090 | - |
| llama.cpp API | http://192.168.1.143:8080 | - |
| Open WebUI | http://192.168.1.143:3000 | https://chat.terrac.com |
| ComfyUI | http://comfyui.home | https://comfyui.terrac.com |
| NextCloud | http://192.168.1.143:8081 | https://nextcloud.terrac.com |
| TinaCMS | http://tina.home | https://tina.terrac.com |
| Overseerr | http://192.168.1.143:5055 | https://requests.terrac.com |
| Tautulli | http://192.168.1.143:8181 | - |
| Home Assistant | http://192.168.1.143:8123 | https://ha.terrac.com |
| Frigate | http://192.168.1.143:5000 | - |

---

## GitHub Actions CI/CD

### Runners (gh-runner-01)

- 4 ephemeral Docker runners at 192.168.1.250
- Labels: `self-hosted`, `homelab`, `ansible`
- Auto-registers with GitHub on container start

### Workflows

| Workflow | Trigger | Action |
|----------|---------|--------|
| `pr-test.yml` | PR opened/updated | Provision test VM, run playbooks, post results |
| `pr-cleanup.yml` | PR closed | Destroy test VM |
| `main-apply.yml` | Merge to main | Deploy changed playbooks to production |

### Required Secrets (Repository)

- `ANSIBLE_VAULT_PASSWORD`: Plain text vault password
- `PROXMOX_SSH_KEY`: Base64-encoded SSH key for root@node005.home

### Example Workflow

```yaml
jobs:
  deploy:
    runs-on: [self-hosted, homelab, ansible]
    steps:
      - uses: actions/checkout@v4
      - name: Deploy
        run: |
          ansible-playbook -i inventories/production/hosts.ini \
            playbooks/individual/ocean/media/plex.yaml
        env:
          ANSIBLE_VAULT_PASSWORD: ${{ secrets.ANSIBLE_VAULT_PASSWORD }}
```

---

## Architecture Decisions

| Decision | Rationale |
|----------|-----------|
| IP addresses over hostnames | DNS fragility avoidance |
| Docker Compose + systemd | Standard lifecycle management |
| Official images preferred | Better support and security |
| Bridge networks | Automatic subnet assignment |
| Consolidated databases | MySQL in Grafana stack, not standalone |
| Go over Python | Performance for log-anomaly-detector |
| Reusable Cloudflare policies | Clean access management |

---

## Related Documentation

- [docs/architecture/overview.md](docs/architecture/overview.md)
- [docs/operations/ocean-migration-plan.md](docs/operations/ocean-migration-plan.md)
- [.github/SETUP.md](.github/SETUP.md)
- [.github/CI_AUTOMATION_GUIDE.md](.github/CI_AUTOMATION_GUIDE.md)