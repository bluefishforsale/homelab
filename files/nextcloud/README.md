# NextCloud File Sharing Platform

NextCloud deployment with MariaDB backend and Redis caching using Docker Compose.

## Architecture

- **NextCloud**: Apache-based file sharing and collaboration platform
- **MariaDB**: Database backend for NextCloud data and metadata  
- **Redis**: Caching layer for improved performance
- **Network**: Private bridge network (172.22.0.0/16) for inter-service communication

## Services

### NextCloud (Port 8081)
- Image: `nextcloud:28-apache`
- Resources: 2 CPU cores, 2GB memory limit
- Volumes: data, config, apps, themes
- Media access: Read-only mount to /data01/media
- Backup access: Read-write mount to /data01/backups

### MariaDB Database  
- Image: `mariadb:11`
- Database: nextcloud
- User: nextcloud
- Custom configuration for NextCloud optimization
- Resources: 1 CPU core, 1GB memory limit

### Redis Cache
- Image: `redis:7-alpine`  
- Memory limit: 256MB with LRU eviction
- Password protected
- Resources: 0.5 CPU cores, 512MB memory limit

## Configuration

### Required Vault Variables
Add to `vault_secrets.yaml`:

```yaml
cloud_services:
  nextcloud:
    admin_user: "admin"
    admin_password: "secure-admin-password"
    database_password: "secure-db-password"
    database_root_password: "secure-root-password"
    redis_password: "secure-redis-password"
    domain: "nextcloud.terrac.com"  # Optional, defaults shown
```

### Directory Structure
```
/data01/services/nextcloud/
├── data/              # NextCloud data files
├── config/            # NextCloud configuration
├── apps/              # Custom NextCloud apps
├── themes/            # Custom NextCloud themes
├── mariadb-data/      # Database files
├── mariadb-logs/      # Database logs
├── mariadb-conf/      # Database configuration
├── redis-data/        # Redis persistence
├── docker-compose.yml # Container configuration
└── .env               # Environment variables
```

## Features

### Security
- No new privileges for all containers
- User mapping (uid:gid 1001:1001)
- Private network isolation
- Password-protected Redis cache

### Performance
- PHP memory limit: 1GB
- Upload limit: 10GB  
- Max file uploads: 100
- Redis caching for database queries and file locks
- MariaDB optimized for NextCloud workload

### Health Checks
- NextCloud: HTTP status endpoint
- MariaDB: Database connectivity check
- Redis: Ping response with authentication

### Integration
- Read-only access to media library (/data01/media)
- Backup storage access (/data01/backups)
- Trusted domains configured for external access
- HTTPS overrides for proper proxy integration

## Deployment

```bash
# Deploy NextCloud
ansible-playbook playbook_ocean_nextcloud.yaml

# Verify deployment
systemctl status nextcloud.service
docker-compose -f /data01/services/nextcloud/docker-compose.yml ps
```

## Access

- **Local**: http://192.168.1.143:8081
- **Domain**: https://nextcloud.terrac.com (via Cloudflare tunnel)
- **Admin**: Login with admin credentials from vault

## Initial Setup

After deployment, NextCloud will automatically:
1. Initialize the database schema
2. Configure Redis caching
3. Set up admin user account
4. Configure trusted domains

## Management

```bash
# Service management
systemctl start nextcloud.service
systemctl stop nextcloud.service
systemctl restart nextcloud.service
systemctl reload nextcloud.service

# Container management
cd /data01/services/nextcloud
docker-compose ps
docker-compose logs -f nextcloud
docker-compose exec nextcloud su -s /bin/bash www-data

# Database management
docker-compose exec nextcloud-db mysql -u nextcloud -p nextcloud

# Redis management  
docker-compose exec nextcloud-redis redis-cli -a <password>
```

## Maintenance

### Backups
- Database: Regular mysqldump to /data01/backups
- Data: File-level backup of /data01/services/nextcloud/data
- Configuration: Backup of config directory

### Updates
- NextCloud: Update version tag in vars and redeploy
- Apps: Use NextCloud web interface or occ command
- Database: MariaDB updates via image tag changes

### Performance Tuning
- Monitor Redis memory usage
- Adjust PHP limits based on usage
- MariaDB buffer pool sizing in custom.cnf

## Troubleshooting

### Common Issues
- **503 errors**: Check container health and logs
- **Database connection**: Verify MariaDB container status
- **Upload failures**: Check PHP limits and disk space
- **Performance**: Monitor Redis hit rates and database queries

### Log Files
- NextCloud: `docker-compose logs nextcloud`
- MariaDB: `docker-compose logs nextcloud-db`
- Redis: `docker-compose logs nextcloud-redis`
- System: `journalctl -u nextcloud.service`
