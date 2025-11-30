# Loki

Log aggregation system with Promtail agents.

---

## Quick Reference

| Property | Value |
|----------|-------|
| Host | ocean (192.168.1.143) |
| Port | 3100 |
| Image | grafana/loki:2.9.3 |
| Storage | /data01/services/loki |
| Retention | 30 days |
| Resources | 4 CPU, 4GB RAM |

---

## Architecture

```text
Promtail (all hosts) → Loki (:3100) → Grafana (:8910)
                           ↓
                   /data01/services/loki/data
```

---

## Deploy

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/loki.yaml --ask-vault-pass
```

---

## Access

| Method | URL |
|--------|-----|
| API | `http://192.168.1.143:3100` |
| Ready | `http://192.168.1.143:3100/ready` |
| Metrics | `http://192.168.1.143:3100/metrics` |

---

## LogQL Examples

```logql
# All logs from ocean
{host="ocean"}

# Filter by level
{host="ocean"} |= "error"

# Docker container logs
{container="plex"}

# Systemd unit logs
{unit="docker.service"}

# Error rate
rate({level="error"}[5m])
```

---

## Management

```bash
# Service
systemctl status loki
systemctl restart loki

# Logs
docker logs loki --tail 50

# Health check
curl http://localhost:3100/ready

# Disk usage
du -sh /data01/services/loki/data
```

---

## Promtail

Agents run on all hosts pushing logs to Loki.

| Property | Value |
|----------|-------|
| Config | /etc/promtail/config.yml |
| Port | 9080 (metrics) |
| Positions | /var/lib/promtail/positions.yaml |

Labels: `host`, `unit`, `level`, `container`

---

## Troubleshooting

| Issue | Check |
|-------|-------|
| Service won't start | `docker logs loki` |
| High memory | Reduce retention, check ingestion rate |
| Storage growing | Verify compactor running |
| Slow queries | Use specific labels, reduce time range |

---

## Related Documentation

- [playbooks/individual/ocean/loki.yaml](/playbooks/individual/ocean/loki.yaml)
- [playbooks/individual/base/logging.yaml](/playbooks/individual/base/logging.yaml)
- [DASHBOARD_README.md](DASHBOARD_README.md)
