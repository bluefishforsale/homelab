# Tautulli Docker Compose Deployment

Plex statistics and monitoring with Prometheus exporter.

---

## Quick Reference

| Service | Image | External Port | Internal Port |
|---------|-------|---------------|---------------|
| Tautulli | linuxserver/tautulli:latest | 8905 | 8181 |
| Exporter | nwalke/tautulli_exporter:latest | 8913 | 9487 |

---

## Deployment

```bash
# Deploy (requires vault for API key)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/tautulli.yaml --ask-vault-pass
```

---

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml.j2` | Tautulli + Exporter containers |
| `tautulli-compose.service.j2` | Systemd service |
| `tautulli.env.j2` | Environment variables |
| `config.ini.j2` | Tautulli configuration with Plex connection |

---

## Services

### Tautulli

- **User**: PUID/PGID 1001 (media)
- **Resources**: 4 CPU, 2GB RAM
- **Health Check**: HTTP on port 8181 (120s start period)
- **Volumes**:
  - `/data01/services/tautulli/config` → `/config`
  - Plex logs → `/logs:ro`

### Tautulli Exporter

- **Resources**: 0.5 CPU, 256MB RAM
- **Health Check**: Disabled (no shell in container)
- **Depends on**: Tautulli healthy
- **Connects via**: Host IP to Tautulli port

---

## Management

### Service Control

```bash
# Restart both services
sudo systemctl restart tautulli.service

# Check status
sudo systemctl status tautulli.service

# View logs
sudo journalctl -u tautulli.service -f

# View container logs
docker logs -f tautulli
docker logs -f tautulli-exporter
```

### Container Operations

```bash
# View running containers
docker compose ps

# Stop containers
docker compose down

# Update images and recreate
docker compose pull
docker compose up -d --force-recreate

# View logs
docker compose logs -f
```

## Access URLs

- **Tautulli Web**: `http://192.168.1.143:8905`
- **Internal**: `http://tautulli.home`
- **External**: `https://tautulli.terrac.com` (via Cloudflare tunnel)
- **Metrics**: `http://192.168.1.143:8913/metrics`

## Configuration

### First-Time Setup

1. Navigate to `http://192.168.1.143:8905`
2. Complete the Tautulli setup wizard
3. Configure Plex Media Server connection
4. Set up admin account and notifications

### Environment Variables

Environment variables are stored in `/data01/services/tautulli/.env`:

```bash
COMPOSE_PROJECT_NAME=tautulli
TZ=America/Los_Angeles
TAUTULLI_PORT=8905
EXPORTER_PORT=8913
TAUTULLI_API_KEY=<from vault>
```

### API Key

The Tautulli API key is stored in vault secrets under `media_services.tautulli.api_key` and used for:

- Prometheus exporter integration
- API access for automation
- External integrations

## Monitoring

### Prometheus Integration

The exporter provides metrics for Prometheus at port 8913:

```yaml
# prometheus.yml configuration
- job_name: 'tautulli'
  static_configs:
    - targets: ['192.168.1.143:8913']
```

### Grafana Dashboard

Import Tautulli exporter dashboard for visualization:

- Dashboard ID: 12651 (from grafana.com)
- Data source: Prometheus

## Troubleshooting

### Container won't start

```bash
# Check container logs
docker logs tautulli

# Verify permissions
ls -la /data01/services/tautulli

# Check if port is already in use
sudo netstat -tlnp | grep 8905
```

### Exporter not collecting metrics

```bash
# Verify API key is correct
docker logs tautulli-exporter

# Test API connection manually
curl "http://192.168.1.143:8905/api/v2?apikey=YOUR_API_KEY&cmd=get_activity"

# Check if Tautulli is healthy
docker compose ps
```

### High memory usage

```bash
# Check resource limits
docker stats tautulli

# Review database size
du -sh /data01/services/tautulli/config/tautulli.db

# Consider database cleanup in Tautulli settings
```

## Maintenance

### Backup

Important files to backup:

- `/data01/services/tautulli/config/tautulli.db` (main database)
- `/data01/services/tautulli/config/config.ini` (configuration)

```bash
# Backup script
cd /data01/services/tautulli/config
tar czf /data01/backups/tautulli-backup-$(date +%Y%m%d).tar.gz tautulli.db config.ini
```

### Updates

```bash
# Pull latest images
docker compose pull

# Recreate containers with new images
docker compose up -d --force-recreate
```

### Database Maintenance

Access Tautulli settings → Database → Database Maintenance:

- Clear table row counts
- Clear watched/streamed history
- Backup database

## Migration from Systemd Services

The playbook automatically handles migration:

1. Stops old `tautulli.service` and `tautulli-exporter.service`
2. Removes old systemd service files
3. Creates new docker-compose based deployment
4. Preserves all configuration and data

Existing configuration and database are maintained in `/data01/services/tautulli/config/`.

## Dependencies

- Docker Engine 20.10+
- Docker Compose 2.0+
- ZFS mounts at /data01
- Plex Media Server running on same host
- Vault secrets configured with API key

## Security

- Containers run with no-new-privileges
- Media user (1001:1001) for proper file permissions
- API key stored in encrypted vault
- Plex logs mounted read-only
- Default bridge network isolation

## References

- [Tautulli Documentation](https://github.com/Tautulli/Tautulli/wiki)
- [Tautulli API Reference](https://github.com/Tautulli/Tautulli/wiki/Tautulli-API-Reference)
- [Exporter Repository](https://github.com/nwalke/tautulli_exporter)
- [LinuxServer.io Image](https://docs.linuxserver.io/images/docker-tautulli)
