# Grafana Monitoring Dashboard with Docker Compose

This deployment configures Grafana as a monitoring and visualization platform using Docker Compose, replacing the previous systemd-based container deployment.

## Key Benefits

- **Idempotent Deployment**: Safe to run multiple times without side effects
- **Health Monitoring**: Built-in health checks for service monitoring
- **Resource Management**: CPU and memory limits for stable operation
- **Security Hardening**: No new privileges and proper file permissions

## Architecture

### Service Configuration
- **Image**: grafana/grafana:11.5.4
- **Port**: 8910:3000 (external:internal)
- **Database**: MySQL at 192.168.1.143:3306
- **Health Check**: Built-in API health endpoint
- **Resources**: 1 CPU core, 1GB memory limit

### Directory Structure
```
/data01/services/grafana/
├── docker-compose.yml          # Docker Compose configuration
├── .env                       # Environment variables
├── grafana.ini               # Main Grafana configuration
├── data/                     # Grafana database and runtime data
├── plugins/                  # Grafana plugins (auto-installed)
└── logs/                     # Grafana logs
```

### Database Configuration
Grafana uses MySQL database for persistent storage:
- **Host**: 192.168.1.143:3306
- **Database**: grafana
- **User**: grafana
- **Connection**: Configured in `grafana.ini`

### Pre-installed Plugins
- **marcusolsson-treemap-panel**: Version 2.0.0 for hierarchical data visualization

## Network Integration

The Grafana container automatically joins the `web_proxy` Docker network, enabling:
- **Container Name Resolution**: nginx can access via `http://grafana:3000`
- **Internal Communication**: Direct container-to-container traffic
- **Service Discovery**: Automatic discovery by other services on the same network

## Configuration

### Access Credentials
- **URL**: http://grafana.home (via nginx proxy)
- **Direct Access**: http://192.168.1.143:8910

### Key Configuration Features
- **Domain**: grafana.home
- **MySQL Backend**: Persistent storage with mysql database
- **Plugin Auto-install**: Treemap panel for enhanced visualization
- **Security**: Admin password protection

## Deployment

### Prerequisites
1. Docker and Docker Compose installed
2. ZFS mounts available at `/data01`
3. MySQL database accessible at `192.168.1.143:3306`
4. web_proxy Docker network (created by nginx)

### Deploy
```bash
ansible-playbook playbook_ocean_grafana_compose.yaml
```

### Service Management
```bash
# Start service
sudo systemctl start grafana.service

# Stop service  
sudo systemctl stop grafana.service

# Restart service
sudo systemctl restart grafana.service

# Check status
sudo systemctl status grafana.service

# View logs
journalctl -u grafana.service -f
docker-compose -f /data01/services/grafana/docker-compose.yml logs -f
```

### Health Monitoring
- **Systemd Status**: `systemctl status grafana.service`
- **Container Health**: `docker ps` (healthy/unhealthy status)
- **Grafana API**: `curl http://localhost:3000/api/health`

## Troubleshooting

### Common Issues

1. **Database Connection Failed**
   - Check MySQL service: `systemctl status mysql`
   - Verify database credentials in `grafana.ini`
   - Test database connectivity: `mysql -h 192.168.1.143 -u grafana -p`

2. **Plugin Installation Failed**
   - Check plugin directory permissions: `ls -la /data01/services/grafana/plugins/`
   - Manually install: `docker exec grafana grafana-cli plugins install marcusolsson-treemap-panel`

3. **Container Network Issues**
   - Verify network membership: `docker network inspect web_proxy`
   - Test nginx connectivity: `docker exec nginx curl -I http://grafana:3000`

### Network Debugging
```bash
# List Docker networks
docker network ls

# Inspect network members  
docker network inspect web_proxy

# Test container connectivity from nginx
docker exec nginx curl -I http://grafana:3000/api/health
docker exec nginx nslookup grafana
```

## Migration from Legacy Setup

If migrating from the previous systemd-based deployment:

1. **Stop old service**: `sudo systemctl stop grafana.service && sudo systemctl disable grafana.service`
2. **Backup data**: `cp -r /data01/services/grafana /data01/services/grafana.backup`
3. **Deploy new setup**: `ansible-playbook playbook_ocean_grafana_compose.yaml`
4. **Verify functionality**: Test dashboard access and database connectivity
5. **Import dashboards**: Restore any custom dashboards if needed

## Security

- **Container Security**: No new privileges, restricted capabilities
- **File Permissions**: Read-only configuration files, restricted write access
- **Network Isolation**: Only exposed ports accessible externally
- **Resource Limits**: CPU and memory constraints prevent resource exhaustion
- **Admin Access**: Password-protected admin interface

## Integration

### Prometheus Data Source
Grafana is pre-configured to connect to Prometheus for metrics visualization:
- **URL**: http://prometheus:9090 (container name resolution)
- **Network**: Shared web_proxy network enables direct communication

### Dashboard Management
- **Default Dashboards**: Import monitoring dashboards for infrastructure services
- **Custom Dashboards**: Create dashboards for homelab metrics
- **Plugin Support**: Treemap panel for hierarchical service visualization
