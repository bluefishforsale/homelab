# Web Proxy Network Architecture

This document outlines the new `web_proxy` network architecture for nginx reverse proxy integration.

## Network Overview

### `web_proxy` Network
- **Purpose**: Dedicated network for nginx reverse proxy communication
- **Subnet**: `172.20.0.0/16`
- **Driver**: Bridge
- **Created by**: nginx docker-compose deployment
- **DNS Resolution**: Automatic container name resolution (127.0.0.11)

### Benefits
- **Clean Architecture**: Dedicated network for proxy services
- **Container Name Resolution**: Use `http://service:port` instead of IP addresses
- **Network Isolation**: Separate from application-specific networks
- **Scalability**: Easy to add new services to proxy network

## Current Status

### ✅ Services Using `web_proxy` Network
1. **nginx** - Creates and joins the network
2. **grafana** - Joins network, accessible as `http://grafana:3000`

### 🔄 Services Ready to Migrate
These services have updated proxy configurations but need to join the network:

1. **ComfyUI** - `http://comfyui:8188`
2. **n8n** - `http://n8n:5678`
3. **Prometheus** - `http://prometheus:9090`
4. **NextCloud** - `http://nextcloud:80`

### 📍 Services Using IP Addresses
These services remain on host networking or need containerization:

- **Plex** - `http://192.168.1.143:32400` (host service)
- **Media Stack** - NZBGet, Sonarr, Radarr, Prowlarr, Bazarr (arr stack)
- **Other Services** - Tautulli, Home Assistant, Overseerr, Tdarr, etc.

## Migration Guide

### Step 1: Deploy nginx with web_proxy Network
```bash
ansible-playbook playbook_ocean_nginx_compose.yaml
```
This creates the `web_proxy` network automatically.

### Step 2: Update Service docker-compose.yml

Add `web_proxy` network to existing services. Example for ComfyUI:

```yaml
# In docker-compose.yml
services:
  comfyui:
    # ... existing configuration ...
    networks:
      - n8n_n8n_network  # Keep existing networks
      - web_proxy        # Add web proxy network

networks:
  n8n_n8n_network:
    external: true
  web_proxy:           # Add web proxy network
    external: true
```

### Step 3: Redeploy Services
```bash
# Restart the service to join new network
sudo systemctl restart comfyui.service
```

### Step 4: Verify Network Connectivity
```bash
# Test from nginx container
docker exec nginx curl -I http://comfyui:8188
docker exec nginx nslookup comfyui
```

## Service-Specific Migration Examples

### ComfyUI Migration
```yaml
# Update files/comfyui/docker-compose.yml.j2
networks:
  - n8n_n8n_network
  - web_proxy  # Add this line

networks:
  n8n_n8n_network:
    external: true
  web_proxy:     # Add this section
    external: true
```

### n8n Migration
```yaml
# Update files/n8n/docker-compose.yml.j2
networks:
  - n8n_n8n_network
  - web_proxy  # Add this line
```

### New Service Template
For new services, always include both networks if they need:
- **n8n_n8n_network**: For n8n workflow integration
- **web_proxy**: For nginx reverse proxy access

```yaml
version: '3.8'

services:
  my-service:
    image: my-service:latest
    container_name: my-service
    networks:
      - web_proxy        # For nginx access
      - n8n_n8n_network  # For n8n integration (if needed)

networks:
  web_proxy:
    external: true
  n8n_n8n_network:
    external: true
```

## Network Debugging

### Inspect Networks
```bash
# List all networks
docker network ls

# Inspect web_proxy network
docker network inspect web_proxy

# See which containers are connected
docker network inspect web_proxy --format='{{range .Containers}}{{.Name}} {{.IPv4Address}}{{"\n"}}{{end}}'
```

### Test Connectivity
```bash
# From nginx container
docker exec nginx ping comfyui
docker exec nginx curl -I http://grafana:3000/api/health

# Check DNS resolution
docker exec nginx nslookup grafana
docker exec nginx nslookup comfyui
```

### Common Issues
1. **Service not reachable**: Ensure service joined web_proxy network
2. **DNS resolution failed**: Check container name matches service name
3. **Connection refused**: Verify service is listening on all interfaces (0.0.0.0)

## Proxy Configuration Patterns

### Standard Service
```nginx
server {
    listen 80;
    server_name service.home;
    
    location / {
        proxy_redirect off;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        proxy_pass http://container-name:port;
        
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

### WebSocket Support
```nginx
# Add these headers for WebSocket support
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
proxy_http_version 1.1;
```

### Long-Running Operations
```nginx
# For services like ComfyUI with long operations
proxy_connect_timeout 300s;
proxy_send_timeout 300s;
proxy_read_timeout 300s;
```

## Deployment Order

1. **nginx** (creates web_proxy network)
2. **grafana** (already configured)
3. **Infrastructure services** (ComfyUI, n8n, Prometheus)
4. **Application services** as they're containerized

## Future Improvements

### Container Migration Priority
1. **High Priority**: ComfyUI, n8n (already docker-compose)
2. **Medium Priority**: Prometheus, NextCloud
3. **Low Priority**: Media stack (arr services) - can remain IP-based

### Network Segmentation
Consider additional networks for:
- **database_network**: For database services (MySQL, PostgreSQL)
- **monitoring_network**: For metrics collection (Prometheus, Grafana)
- **media_network**: For media stack services

This approach provides clean network segmentation while maintaining the primary `web_proxy` network for reverse proxy access.
