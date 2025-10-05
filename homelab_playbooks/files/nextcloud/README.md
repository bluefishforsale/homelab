# NextCloud File Sharing Service

This deploys NextCloud file sharing service using LinuxServer.io NextCloud (latest) with Docker Compose, MariaDB, and Redis.

## Architecture

- **NextCloud**: Web-based file sharing and collaboration platform
- **MariaDB**: Database backend for user data and metadata
- **Redis**: Caching layer for improved performance

## Services

| Service | Container | Port | Purpose |
|---------|-----------|------|---------|
| NextCloud | nextcloud | 8081 | File sharing web interface |
| MariaDB | nextcloud-db | Internal | Database backend |
| Redis | nextcloud-redis | Internal | Caching layer |

## Configuration

### Environment Variables
- `NEXTCLOUD_ADMIN_USER`: Administrator username
- `NEXTCLOUD_ADMIN_PASSWORD`: Administrator password
- `MYSQL_PASSWORD`: Database password
- `REDIS_PASSWORD`: Redis cache password

### Trusted Domains
NextCloud is configured with:
- `nextcloud.home` (local network access)
- `nextcloud.terrac.com` (external access via Cloudflare tunnel)
- `192.168.1.143` (direct IP access)

### File Storage
- Configuration: `/data01/services/nextcloud/config` (LinuxServer.io /config mount)
- User files: `/data01/nextcloud-files` (LinuxServer.io /data mount)
- Custom apps: `/data01/services/nextcloud/apps`
- Database: `/data01/services/nextcloud/database`
- Redis cache: `/data01/services/nextcloud/redis`

## Access

- **Internal**: http://192.168.1.143:8081 or http://nextcloud.home
- **External**: https://nextcloud.terrac.com (via Cloudflare tunnel)

## Management

```bash
# Check service status
sudo systemctl status nextcloud.service

# View logs
sudo journalctl -u nextcloud.service -f

# Restart service
sudo systemctl restart nextcloud.service

# Check container status
cd /data01/services/nextcloud
docker-compose ps
```

## Pre-Configuration

NextCloud is **pre-configured** to skip the setup wizard entirely:

- **Database Integration**: Automatically connects to MariaDB container
- **Redis Caching**: Pre-configured for optimal performance  
- **Admin User**: Created from vault secrets during deployment
- **Trusted Domains**: Pre-configured for local and external access
- **Security Tokens**: Automatically generated unique salt/secret keys

**No manual setup required** - NextCloud is ready to use immediately after deployment!

## Features

- **File Sharing**: Upload, download, and share files
- **Collaboration**: Real-time document editing and sharing
- **Mobile Apps**: iOS and Android applications available
- **Desktop Sync**: Desktop clients for automatic synchronization
- **Apps**: Extensive app ecosystem for additional functionality
- **Security**: Built-in encryption, two-factor authentication
- **Performance**: Redis caching for improved response times
- **LinuxServer.io**: Stable container with better defaults and automatic updates

## Troubleshooting

### Version Mismatch Error
If you see: `Can't start Nextcloud because the version of the data (X.X.X.X) is higher than the docker image version (Y.Y.Y.Y)`

**Solutions:**
1. **Pull latest image**: `docker pull linuxserver/nextcloud:latest`
2. **Reset data (if acceptable)**: 
   ```bash
   sudo systemctl stop nextcloud.service
   sudo rm -rf /data01/services/nextcloud/config
   ansible-playbook -i inventory.ini playbook_ocean_nextcloud.yaml --ask-vault-pass
   ```
3. **Check logs**: `sudo journalctl -u nextcloud.service -f`

The playbook now uses `latest` tag and auto-detects NextCloud version to avoid this issue.
