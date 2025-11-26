# fail2ban-prometheus-exporter

Prometheus exporter for fail2ban metrics across all homelab nodes.

## Overview

Exports fail2ban ban statistics and jail information to Prometheus for monitoring SSH protection and security events.

## Architecture

- **Image**: registry.gitlab.com/hctrdev/fail2ban-prometheus-exporter:latest
- **Port**: 9191 (default)
- **Network**: Bridge network with exposed port
- **Storage**: /var/lib/services/fail2ban-exporter (not /data01 - works on all nodes)

## Metrics Exposed

- `fail2ban_up` - fail2ban service status
- `fail2ban_jail_banned_current` - Currently banned IPs per jail
- `fail2ban_jail_banned_total` - Total bans per jail
- `fail2ban_jail_failed_current` - Current failed attempts per jail
- `fail2ban_jail_failed_total` - Total failed attempts per jail

## Deployment

```bash
# Deploy to all nodes
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/infrastructure/fail2ban_exporter.yaml

# Deploy to specific host
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/infrastructure/fail2ban_exporter.yaml -l ocean

# Check status on all hosts
ansible all -i inventories/production/hosts.ini -m shell -a "systemctl status fail2ban-exporter"
```

## Access

- **Metrics**: `http://<hostname>:9191/metrics`
- **Example**: `http://ocean.home:9191/metrics`

## Prometheus Configuration

Scrape config in prometheus.yml:

```yaml
- job_name: 'fail2ban-exporter'
  relabel_configs: *dropPortNumber
  static_configs:
{% for host in groups['all'] %}
  - targets: ['{{ host }}.home:9191']
{% endfor %}
```

## Troubleshooting

### Check exporter is running

```bash
systemctl status fail2ban-exporter
docker ps | grep fail2ban-exporter
```

### View logs

```bash
docker logs fail2ban-exporter
journalctl -u fail2ban-exporter -f
```

### Test metrics endpoint

```bash
curl http://localhost:9191/metrics
```

### Verify fail2ban integration

```bash
# Check fail2ban socket directory
ls -la /var/run/fail2ban/

# Test fail2ban client
fail2ban-client status
```

## Requirements

- fail2ban installed and running on host
- Docker installed on host
- Ports:
  - 9191 exposed for Prometheus scraping

## Security

- Read-only access to fail2ban socket and database
- No new privileges security option
- Minimal resource limits (128M memory, 0.5 CPU)
