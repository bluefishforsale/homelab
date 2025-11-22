# WordPress Blog Deployment

WordPress blog platform deployment for ocean server with MySQL backend.

## Architecture

- **WordPress**: Latest official WordPress image with Apache
- **MySQL 8.0**: Database backend with WordPress-optimized configuration
- **Private Network**: 172.26.0.0/16 subnet for container communication

## Features

- Full WordPress installation with MySQL database
- Optimized PHP settings (512M memory, 100M uploads, 300s execution time)
- MySQL tuned for WordPress workload
- User mapping (uid/gid 1001:1001) following homelab standards
- Resource limits: WordPress (2 CPU, 2GB), MySQL (2 CPU, 2GB)
- Health checks for both services
- Security hardening with no-new-privileges

## Access URLs

- **Local**: http://192.168.1.143:8085
- **Internal**: http://saetnere.home (via nginx proxy)
- **External**: https://blog.saetnere.com (via Cloudflare tunnel)

## Special Configuration

### nginx Proxy Host Header Rewrite

The nginx proxy at `saetnere.home` is configured to rewrite the Host header to `blog.saetnere.com`. This ensures WordPress generates correct URLs and redirects even when accessed via the internal domain.

```nginx
proxy_set_header Host blog.saetnere.com;
proxy_set_header X-Forwarded-Proto https;
```

### Cloudflare Access - Public (No Authentication)

The blog is configured with a **bypass policy** in Cloudflare Access, meaning:
- ✅ No authentication required
- ✅ Accessible to everyone on the internet
- ✅ Direct access without login page

This is different from other services that require email authentication.

## Required Vault Variables

Add to `vault_secrets.yaml` under `cloud_services.wordpress`:

```yaml
cloud_services:
  wordpress:
    database_password: "secure-database-password"
    database_root_password: "secure-root-password"
    domain: "blog.saetnere.com"
```

## Deployment

```bash
# Deploy WordPress
ansible-playbook playbooks/individual/ocean/services/wordpress.yaml

# Update nginx proxy (if needed)
ansible-playbook playbooks/individual/ocean/network/nginx_compose.yaml

# Update Cloudflare tunnel and access policies
ansible-playbook playbooks/individual/ocean/network/cloudflared.yaml
```

## Initial Setup

1. Access https://blog.saetnere.com
2. Complete WordPress installation wizard:
   - Select language
   - Set site title
   - Create admin user
   - Set admin password
   - Enter admin email

## Directory Structure

```
/data01/services/wordpress/
├── wp-content/            # USER CONTENT - PERSISTENT ON HOST
│   ├── themes/           # All WordPress themes
│   ├── plugins/          # All WordPress plugins  
│   ├── uploads/          # All media uploads (images, videos, files)
│   └── upgrade/          # Temporary update files
├── mysql-data/           # MySQL database files
├── mysql-logs/           # MySQL logs
├── mysql-conf/           # MySQL configuration
│   └── custom.cnf       # Custom MySQL settings
├── docker-compose.yml   # Container definitions
└── .env                 # Environment variables (sensitive)
```

**Important:** WordPress core files (`wp-admin/`, `wp-includes/`, `wp-config.php`) stay **inside the container** and are ephemeral. This is by design:
- ✅ Core files update automatically when you update the Docker image
- ✅ All user content (themes, plugins, uploads) persists on host disk
- ✅ No risk of losing customizations or media files
- ✅ Clean separation between WordPress core and user data

## Maintenance

### Backup

```bash
# Backup WordPress user content (themes, plugins, uploads)
sudo tar -czf wordpress-content-$(date +%Y%m%d).tar.gz /data01/services/wordpress/wp-content

# Backup MySQL database
docker exec wordpress-db mysqldump -u wordpress -p'PASSWORD' wordpress > wordpress-db-$(date +%Y%m%d).sql

# Full backup (content + database)
sudo tar -czf wordpress-full-$(date +%Y%m%d).tar.gz /data01/services/wordpress/wp-content /data01/services/wordpress/mysql-data
```

### Update WordPress

**Core Updates:** WordPress core is in the container. To update:
```bash
# Pull latest WordPress image
docker pull wordpress:latest

# Restart service to use new image
sudo systemctl restart wordpress.service
```

**Plugin/Theme Updates:** Apply directly through the WordPress admin dashboard at `https://blog.saetnere.com/wp-admin/`

### Update MySQL

```bash
# Update docker-compose.yml to specify new MySQL version
# Then restart service
sudo systemctl restart wordpress.service
```

## Troubleshooting

### Check service status

```bash
sudo systemctl status wordpress.service
```

### View logs

```bash
# WordPress container logs
docker logs wordpress

# MySQL container logs
docker logs wordpress-db

# All logs
sudo journalctl -u wordpress.service -f
```

### Container health

```bash
# Check container status
docker ps | grep wordpress

# Check container health
docker inspect wordpress | grep -A 10 Health
docker inspect wordpress-db | grep -A 10 Health
```

### Database connection issues

```bash
# Verify MySQL is running
docker exec wordpress-db mysqladmin ping -u wordpress -p'PASSWORD'

# Test database connection from WordPress container
docker exec wordpress mysql -h wordpress-db -u wordpress -p'PASSWORD' wordpress -e "SELECT 1"
```

### Permission issues

```bash
# Fix ownership (run as root)
sudo chown -R 1001:1001 /data01/services/wordpress/
sudo chown -R www-data:www-data /data01/services/wordpress/html/
```

## Resource Usage

- **WordPress**: 2 CPU cores max, 2GB RAM max (512MB reserved)
- **MySQL**: 2 CPU cores max, 2GB RAM max (512MB reserved)
- **Disk**: ~500MB base installation, grows with content/uploads

## Security

- No new privileges in containers
- Private tmp for systemd service
- Protected system directories
- Read-only docker socket access
- Database credentials stored in vault
- MySQL not exposed to host network (internal only)

## Network Architecture

```
Internet → Cloudflare Tunnel → WordPress (8085)
Internal → nginx (saetnere.home) → WordPress (8085)
WordPress ↔ MySQL (wordpress_internal network)
```
