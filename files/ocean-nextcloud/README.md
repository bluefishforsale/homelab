# NextCloud

File sharing platform with MariaDB and Redis.

---

## Quick Reference

| Property | Value |
|----------|-------|
| Host | ocean (192.168.1.143) |
| Port | 8081 |
| Image | nextcloud:28-apache |
| Database | MariaDB 11 |
| Cache | Redis 7 |

---

## Architecture

```text
NextCloud (:8081) → MariaDB → Redis
     ↑
   nginx → cloudflared → internet
```

Private network: 172.22.0.0/16

---

## Deploy

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/services/nextcloud.yaml --ask-vault-pass
```

---

## Vault Variables

Required in `vault/secrets.yaml`:

```yaml
cloud_services:
  nextcloud:
    admin_user: "admin"
    admin_password: "<password>"
    database_password: "<password>"
    database_root_password: "<password>"
    redis_password: "<password>"
```

---

## Directory Structure

```text
/data01/services/nextcloud/
├── data/           # NextCloud files
├── config/         # Configuration
├── apps/           # Custom apps
├── mariadb-data/   # Database
├── redis-data/     # Cache
└── docker-compose.yml
```

---

## Access

| Method | URL |
|--------|-----|
| Local | `http://192.168.1.143:8081` |
| Internal | `http://nextcloud.home` |
| External | `https://nextcloud.terrac.com` |

---

## Management

```bash
# Service
systemctl status nextcloud.service
systemctl restart nextcloud.service

# Logs
docker logs nextcloud --tail 50
docker logs nextcloud-db --tail 50

# Shell
docker exec -it nextcloud bash

# Database
docker exec -it nextcloud-db mysql -u nextcloud -p nextcloud
```

---

## Troubleshooting

| Issue | Check |
|-------|-------|
| 503 errors | Container health, logs |
| DB connection | MariaDB container status |
| Upload failures | PHP limits, disk space |
| Slow performance | Redis memory, DB queries |

---

## Related Documentation

- [playbooks/individual/ocean/services/nextcloud.yaml](/playbooks/individual/ocean/services/nextcloud.yaml)
- [docs/architecture/ocean-services.md](/docs/architecture/ocean-services.md)
