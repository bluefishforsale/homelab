# WordPress

Blog platform with MySQL backend.

---

## Quick Reference

| Property | Value |
|----------|-------|
| Host | ocean (192.168.1.143) |
| Port | 8085 |
| Image | wordpress:latest |
| Database | MySQL 8.0 |
| Storage | /data01/services/wordpress |

---

## Architecture

```text
Internet → cloudflared → WordPress (:8085) → MySQL
                              ↑
nginx (saetnere.home) ────────┘
```

Private network: 172.26.0.0/16

---

## Deploy

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/services/wordpress.yaml --ask-vault-pass
```

---

## Vault Variables

Required in `vault/secrets.yaml`:

```yaml
cloud_services:
  wordpress:
    database_password: "<password>"
    database_root_password: "<password>"
    domain: "blog.saetnere.com"
```

---

## Access

| Method | URL |
|--------|-----|
| Local | `http://192.168.1.143:8085` |
| Internal | `http://saetnere.home` |
| External | `https://blog.saetnere.com` |

Public access (no Cloudflare Access authentication).

---

## Directory Structure

```text
/data01/services/wordpress/
├── wp-content/     # Themes, plugins, uploads (persistent)
├── mysql-data/     # Database
├── docker-compose.yml
└── .env
```

WordPress core files are in the container (ephemeral).

---

## Management

```bash
# Service
systemctl status wordpress.service
systemctl restart wordpress.service

# Logs
docker logs wordpress --tail 50
docker logs wordpress-db --tail 50

# Backup content
tar -czf wordpress-backup.tar.gz /data01/services/wordpress/wp-content

# Backup database
docker exec wordpress-db mysqldump -u wordpress -p wordpress > backup.sql
```

---

## Troubleshooting

| Issue | Check |
|-------|-------|
| Service down | `docker logs wordpress` |
| DB connection | `docker logs wordpress-db` |
| Permissions | `chown -R 1001:1001 /data01/services/wordpress/` |

---

## Related Documentation

- [playbooks/individual/ocean/services/wordpress.yaml](/playbooks/individual/ocean/services/wordpress.yaml)
- [docs/architecture/ocean-services.md](/docs/architecture/ocean-services.md)
