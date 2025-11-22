# n8n Workflow Automation with GPU Support

This playbook deploys n8n workflow automation platform on the ocean server with Nvidia P2000 GPU support.

## Features

- **GPU Support**: Configured for Nvidia P2000 GPU access
- **Docker Compose**: Clean containerized deployment 
- **Systemd Integration**: Managed as a systemd service
- **Security**: Runs as dedicated user with restricted permissions
- **Health Checks**: Built-in health monitoring
- **Persistent Data**: SQLite database with persistent volumes

## Configuration

### Environment Variables

Key environment variables in `.env` file:

- `N8N_ENCRYPTION_KEY`: Encryption key for sensitive data (change from default!)
- `N8N_BASIC_AUTH_USER/PASSWORD`: Basic auth credentials (change from defaults!)
- `WEBHOOK_URL`: External webhook URL for n8n
- `NVIDIA_VISIBLE_DEVICES`: GPU visibility (set to 'all')

### GPU Access

The deployment includes:

- Nvidia runtime configuration
- GPU device reservations
- CUDA library mounts
- Environment variables for GPU detection

### Security

- Dedicated user account (uid/gid 1000)
- No new privileges
- Private tmp directory  
- Protected system directories
- Read-only Docker socket access

## Usage

### Deploy

```bash
ansible-playbook -i inventory.ini playbook_ocean_n8n.yaml
```

### Access

- Web UI: http://ocean-ip:5678
- Health check: http://ocean-ip:5678/healthz

### Management

```bash
# Check status
sudo systemctl status n8n

# View logs
sudo journalctl -u n8n -f

# Restart service
sudo systemctl restart n8n

# Check Docker containers
sudo docker-compose -f /data01/services/n8n/docker-compose.yml ps
```

## GPU Verification

After deployment, verify GPU access:

```bash
# Check if GPU is visible in container
sudo docker exec n8n nvidia-smi

# Check CUDA device access
sudo docker exec n8n ls -la /dev/nvidia*
```

## Notes

- Service runs on port 5678
- Data persists in `/data01/services/n8n/data`
- Logs available via journalctl and container logs
- Requires Nvidia drivers and docker runtime on host
