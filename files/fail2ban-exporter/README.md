# fail2ban-prometheus-exporter

Prometheus exporter for fail2ban metrics across all homelab nodes.

---

## Quick Reference

| Setting | Value |
|---------|-------|
| Image | registry.gitlab.com/hctrdev/fail2ban-prometheus-exporter:latest |
| Port | 9191 |
| Network | fail2ban_net (bridge) |
| Storage | /var/lib/services/fail2ban-exporter |

---

## Deployment

```bash
# Deploy to all nodes (no vault required)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/fail2ban_exporter.yaml

# Deploy to specific host
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/fail2ban_exporter.yaml -l ocean
```

**Auto-detection**: Playbook checks for Docker AND fail2ban before deploying. Skips hosts without both.

---

## Files

| File | Purpose |
|------|---------|
| `fail2ban-exporter-compose.yml.j2` | Docker Compose configuration |
| `fail2ban-exporter.service.j2` | Systemd service |
| `fail2ban-exporter.env.j2` | Environment variables |
| `fail2ban-grafana-dahsboard.json` | Grafana dashboard |

---

## Directory Structure

```text
/var/lib/services/fail2ban-exporter/
├── docker-compose.yml
├── .env
└── README.md
```

**Note**: Uses `/var/lib/services/` (not `/data01/`) so it works on all nodes.

---

## Metrics Exposed

- `fail2ban_up` - fail2ban service status
- `fail2ban_jail_banned_current` - Currently banned IPs per jail
- `fail2ban_jail_banned_total` - Total bans per jail
- `fail2ban_jail_failed_current` - Current failed attempts per jail
- `fail2ban_jail_failed_total` - Total failed attempts per jail

---

## Access

- **Metrics**: `http://<hostname>:9191/metrics`
- **Example**: `http://ocean.home:9191/metrics`

---

## Prometheus Configuration

```yaml
- job_name: 'fail2ban-exporter'
  static_configs:
    - targets:
      - 'ocean.home:9191'
      - 'node005.home:9191'
      - 'node006.home:9191'
```

---

## Service Management

```bash
# Status
systemctl status fail2ban-exporter

# Restart
systemctl restart fail2ban-exporter

# Logs
journalctl -u fail2ban-exporter -f
docker logs fail2ban-exporter
```

---

## Troubleshooting

### Test metrics endpoint

```bash
curl http://localhost:9191/metrics
```

### Health check

```bash
docker inspect fail2ban-exporter --format='{{.State.Health.Status}}'
```

### Verify fail2ban socket

```bash
# Check fail2ban socket directory (mounted read-only)
ls -la /var/run/fail2ban/

# Test fail2ban is working
fail2ban-client status
```

### Container not starting

```bash
# Check if fail2ban is installed
which fail2ban-client

# Check if socket exists
ls -la /var/run/fail2ban/fail2ban.sock
```

---

## Requirements

- fail2ban installed and running on host
- Docker installed on host
- fail2ban socket at `/var/run/fail2ban/`

---

## Security

- **no-new-privileges**: Enabled
- **Read-only socket**: `/var/run/fail2ban:ro`
- **Resource limits**: 128M memory, 0.5 CPU
- **Health check**: wget-based metrics endpoint check
