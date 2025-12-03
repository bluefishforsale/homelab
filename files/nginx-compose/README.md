# Nginx

Reverse proxy for all ocean services.

---

## Quick Reference

| Property | Value |
|----------|-------|
| Host | ocean (192.168.1.143) |
| Ports | 80, 443 |
| Image | nginx:1.27.3-alpine |
| Network | homelab_bridge (172.25.0.0/16) |
| Storage | /data01/services/nginx |

---

## Architecture

```text
Internet → cloudflared → nginx (:80) → services
                              ↓
                      172.25.0.1:PORT (host services)
```

All services accessed via fixed gateway IP 172.25.0.1.

---

## Deploy

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/network/nginx_compose.yaml --ask-vault-pass
```

---

## Directory Structure

```text
/data01/services/nginx/
├── nginx.conf          # Main configuration
├── conf.d/
│   └── proxy_hostname.conf  # Virtual hosts
├── logs/
├── docker-compose.yml
└── .env
```

---

## Management

```bash
# Service
systemctl status nginx.service
systemctl restart nginx.service

# Logs
docker logs nginx --tail 50

# Test config
docker exec nginx nginx -t

# Reload config
docker exec nginx nginx -s reload
```

---

## Proxy Pattern

Services configured in `conf.d/proxy_hostname.conf`:

```nginx
server {
    listen 80;
    server_name service.home;
    location / {
        proxy_pass http://172.25.0.1:PORT;
    }
}
```

---

## Troubleshooting

| Issue | Check |
|-------|-------|
| 504 Gateway Timeout | Target service running, port correct |
| 502 Bad Gateway | `docker logs nginx` |
| Config errors | `docker exec nginx nginx -t` |

---

## Related Documentation

- [playbooks/individual/ocean/network/nginx_compose.yaml](/playbooks/individual/ocean/network/nginx_compose.yaml)
- [docs/architecture/ocean-services.md](/docs/architecture/ocean-services.md)
