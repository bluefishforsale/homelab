# Roadmap

Status and future plans for the homelab infrastructure.

---

## Vision

| Status | Goal |
|--------|------|
| Done | Fully automated - Ansible playbooks + GitHub Actions |
| Done | SSH free for 99% of tasks - Cloudflare tunnels |
| Done | Publicly signed certs - Let's Encrypt via Cloudflare |
| Done | Git driven IaC - All playbooks in Git, idempotent |
| Done | Git driven builds - GitHub Actions with self-hosted runners |
| Done | Local LLM - llama.cpp + Open WebUI with RTX 3090 |
| Partial | Service discovery - Docker DNS via container names |
| Todo | Log aggregation - Loki |
| Todo | Control plane UI - Semaphore or Rundeck |
| Todo | HA for critical components |
| Todo | DR plan with recovery procedures |

---

## Completed

### Infrastructure

- Proxmox cluster: node005 + node006
- Ocean VM migration to node006 with GPU passthrough
- ZFS pool (data01) imported in ocean VM
- BIND DNS (dns01) + Pi-hole
- GitHub Actions self-hosted runners (gh-runner-01)
- Cloudflare tunnels + nginx reverse proxy
- All playbooks idempotent

### Services on Ocean

- Media: Plex, Sonarr, Radarr, Prowlarr, Bazarr, NZBGet, Overseerr, Tautulli, Tdarr
- AI/ML: llama.cpp, Open WebUI, ComfyUI (RTX 3090)
- Monitoring: Prometheus, Grafana, NVIDIA DCGM, UnPoller
- Services: NextCloud, TinaCMS, Frigate, Home Assistant, Audible Downloader

### Automation

- Ansible vault for secrets
- Docker Compose + systemd integration
- GitHub Actions CI/CD workflows

---

## In Progress

| Task | Status |
|------|--------|
| DNS Prometheus exporter | Ready, needs testing |
| Service discovery | Docker DNS only |

---

## Todo

### Phase 1: Monitoring Improvements

- AlertManager and alerts
- Loki log aggregation
- Complete DNS exporter testing

### Phase 2: Control Plane UI

- Ansible Semaphore or Rundeck
- GitLab CI integration (optional)

### Phase 3: HA and DR

- Proxmox HA between nodes
- DR plan with recovery procedures
- HA for DNS/DHCP

### Phase 4: Advanced Features

- Consul for service discovery
- Auto-scaling runners
- PXE boot automation

---

## GitHub Actions Runners

Deployed on gh-runner-01 (192.168.1.250):

- 4 ephemeral Docker runners
- Ansible pre-installed
- Labels: `self-hosted`, `homelab`, `ansible`

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

## Related Documentation

- [docs/architecture/overview.md](docs/architecture/overview.md)
- [docs/operations/ocean-migration-plan.md](docs/operations/ocean-migration-plan.md)
- [.github/SETUP.md](.github/SETUP.md)