# Loki Log Aggregation System

Loki is a horizontally-scalable, highly-available log aggregation system designed for efficient log storage and querying.

## Overview

- **Image**: grafana/loki:2.9.3
- **Port**: 3100 (internal and external)
- **Storage**: /data01/services/loki
- **Resources**: 4 CPU cores, 4GB RAM
- **Retention**: 30 days
- **Storage Limit**: 20GB (enforced via retention policy)
- **Network**: loki_network (172.26.0.0/16)

## Architecture

```
Promtail Agents → Loki → Grafana
                    ↓
            /data01/services/loki/data
```

## Deployment

```bash
# Deploy Loki
ansible-playbook playbooks/individual/ocean/loki.yaml

# Check service status
systemctl status loki

# View logs
docker logs loki

# Query service health
curl http://localhost:3100/ready
curl http://localhost:3100/metrics
```

## Access URLs

- **API**: http://192.168.1.143:3100
- **Internal**: http://loki.home (via nginx)
- **Ready Endpoint**: http://192.168.1.143:3100/ready
- **Metrics**: http://192.168.1.143:3100/metrics

## Configuration

### Retention Policy
- **Period**: 720 hours (30 days)
- **Compaction**: Every 10 minutes
- **Delete Delay**: 2 hours after marking for deletion
- **Auto-cleanup**: Enabled via compactor

### Storage Management
- **Chunks**: /loki/chunks (time-series log data)
- **Index**: /loki/boltdb-shipper-active (log metadata)
- **Rules**: /loki/rules (alerting rules)
- **Compactor**: /loki/compactor (retention worker)

### Query Limits
- **Max Query Length**: 721 hours (30 days + 1 hour)
- **Max Query Parallelism**: 32 concurrent queries
- **Max Entries**: 10,000 per query
- **Timeout**: 90 seconds (nginx proxy setting)

### Ingestion Limits
- **Rate**: 16 MB/s per stream
- **Burst**: 32 MB
- **Per Stream Rate**: 8 MB/s
- **Per Stream Burst**: 16 MB

## Grafana Integration

Loki datasource is automatically provisioned in Grafana:

```yaml
Name: Loki
Type: loki
URL: http://192.168.1.143:3100
```

### LogQL Query Examples

```logql
# View all logs from ocean host
{host="ocean"}

# Filter by log level
{host="ocean"} |= "error"

# View Docker container logs
{container="comfyui"}

# View systemd unit logs
{unit="docker.service"}

# Rate of errors over 5 minutes
rate({level="error"}[5m])

# Count logs by hostname
sum(count_over_time({job="systemd-journal"}[1h])) by (hostname)
```

## Promtail Integration

All hosts run Promtail agents that push logs to Loki:

- **Agent Config**: /etc/promtail/config.yml
- **Journal Source**: /run/log/journal (volatile storage)
- **Syslog Source**: Port 1514 (ocean only)
- **Positions File**: /var/lib/promtail/positions.yaml

Labels added by Promtail:
- `job`: systemd-journal or syslog
- `host`: inventory hostname
- `unit`: systemd unit name
- `level`: log priority (error, warning, info)
- `container`: Docker container name (if applicable)
- `hostname`: Source hostname from log

## Maintenance

### View Container Logs
```bash
docker logs loki
docker logs -f loki --tail 100
```

### Check Disk Usage
```bash
du -sh /data01/services/loki/data
```

### Manual Cleanup (if needed)
```bash
# Stop Loki
systemctl stop loki

# Clean old chunks
find /data01/services/loki/data/chunks -mtime +30 -delete

# Start Loki
systemctl start loki
```

### Configuration Changes
```bash
# Edit config
vim /data01/services/loki/config/loki-config.yaml

# Restart service
systemctl restart loki
```

## Troubleshooting

### Service Won't Start
```bash
# Check systemd status
systemctl status loki
journalctl -u loki -n 50

# Check container status
docker ps -a | grep loki
docker logs loki
```

### High Memory Usage
- Check ingestion rate: curl http://localhost:3100/metrics | grep loki_ingester
- Reduce retention period if needed
- Lower query parallelism limits

### Storage Growing Too Fast
- Verify retention is working: Check compactor logs
- Reduce ingestion rate limits
- Check for log spam from specific sources

### Query Performance Issues
- Use more specific label filters
- Reduce time range
- Limit entries returned
- Check query parallelism settings

## Security

- **No Authentication**: Internal service only
- **Network**: Bridge network isolation
- **User**: Runs as uid/gid 1001 (media user)
- **Security Options**: no-new-privileges enabled

## Related Services

- **Promtail**: Log collector (all hosts)
- **Grafana**: Visualization and querying
- **nginx**: Reverse proxy (loki.home)

## References

- [Loki Documentation](https://grafana.com/docs/loki/latest/)
- [LogQL Query Language](https://grafana.com/docs/loki/latest/logql/)
- [Storage Configuration](https://grafana.com/docs/loki/latest/operations/storage/)
