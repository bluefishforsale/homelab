# Nginx Reverse Proxy with Docker Compose

This deployment configures nginx as a reverse proxy using Docker Compose, replacing the previous systemd-based container deployment.

## Key Benefits

- **Network Integration**: Creates and manages the `web_proxy` network for seamless reverse proxy communication
- **Idempotent Deployment**: Safe to run multiple times without side effects
- **Health Monitoring**: Built-in health checks for service monitoring
- **Resource Management**: CPU and memory limits for stable operation
- **Security Hardening**: No new privileges and proper file permissions

## Architecture

### Service Configuration
- **Image**: nginx:1.27.3-alpine
- **Ports**: 80 (HTTP), 443 (HTTPS)
- **Network**: web_proxy (created and managed by nginx)
- **Health Check**: Built-in nginx configuration test
- **Resources**: 2 CPU cores, 512MB memory limit

### Directory Structure
```
/data01/services/nginx/
├── docker-compose.yml          # Docker Compose configuration
├── .env                       # Environment variables
├── nginx.conf                 # Main nginx configuration
├── conf.d/                   # Virtual host configurations
│   └── proxy_hostname.conf   # Service proxy definitions
└── logs/                     # Nginx access and error logs
```

### Network Integration
The nginx container creates and manages the `web_proxy` Docker network, enabling:
- **Container Name Resolution**: Access services via container names (e.g., `http://comfyui:8188`)
- **Internal Communication**: Direct container-to-container traffic without external routing
- **Service Discovery**: Automatic discovery of other services on the same network

## Proxy Configuration

Services are configured in `/conf.d/proxy_hostname.conf` with the following pattern:

```nginx
server {
    listen 80;
    server_name service.home;
    
    location / {
        proxy_pass http://container_name:port;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket support
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_http_version 1.1;
        
        # Timeouts
        proxy_connect_timeout 300s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }
}
```

## Deployment

### Prerequisites
1. Docker and Docker Compose installed
2. ZFS mounts available at `/data01`
3. web_proxy Docker network (created by nginx deployment)

### Deploy
```bash
ansible-playbook playbook_ocean_nginx_compose.yaml
```

### Service Management
```bash
# Start service
sudo systemctl start nginx.service

# Stop service  
sudo systemctl stop nginx.service

# Restart service
sudo systemctl restart nginx.service

# Check status
sudo systemctl status nginx.service

# View logs
journalctl -u nginx.service -f
docker-compose -f /data01/services/nginx/docker-compose.yml logs -f
```

### Health Monitoring
- **Systemd Status**: `systemctl status nginx.service`
- **Container Health**: `docker ps` (healthy/unhealthy status)
- **Nginx Status**: `curl -I http://localhost/health`

## Troubleshooting

### Common Issues

1. **504 Gateway Timeout**
   - Check if target service is running: `docker ps | grep service_name`
   - Verify network connectivity: `docker exec nginx curl -I http://service:port`
   - Check proxy configuration in `conf.d/proxy_hostname.conf`

2. **Container Name Resolution Failed**
   - Ensure services are on the same Docker network: `docker network inspect web_proxy`
   - Verify container names match proxy configuration

3. **Permission Denied**
   - Check file ownership: `ls -la /data01/services/nginx/`
   - Verify systemd service permissions

### Network Debugging
```bash
# List Docker networks
docker network ls

# Test container connectivity from nginx
docker exec nginx curl -I http://comfyui:8188
docker exec nginx nslookup comfyui
```

## Migration from Legacy Setup

If migrating from the previous systemd-based deployment:

1. **Stop old service**: `sudo systemctl stop nginx.service && sudo systemctl disable nginx.service`
2. **Backup configuration**: `cp -r /data01/services/nginx /data01/services/nginx.backup`
3. **Deploy new setup**: `ansible-playbook playbook_ocean_nginx_compose.yaml`
4. **Verify functionality**: Test all proxy endpoints
5. **Clean up**: Remove old systemd service file if desired

## Security

- **Container Security**: No new privileges, restricted capabilities
- **File Permissions**: Read-only configuration files, restricted write access
- **Network Isolation**: Only exposed ports accessible externally
- **Resource Limits**: CPU and memory constraints prevent resource exhaustion
