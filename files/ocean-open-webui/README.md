# Open WebUI

Chat interface for llama.cpp with GPU-accelerated inference.

---

## Quick Reference

| Property | Value |
|----------|-------|
| Host | ocean (192.168.1.143) |
| Port | 3000 |
| Image | ghcr.io/open-webui/open-webui |
| Backend | llama.cpp (:8080) |
| Storage | /data01/services/open-webui |
| GPU | RTX 3090 (via llama.cpp) |

---

## Architecture

```text
Browser → nginx → Open WebUI (:3000) → llama.cpp (:8080) → RTX 3090
```

---

## Deploy

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/open_webui.yaml --ask-vault-pass
```

---

## Access

| Method | URL |
|--------|-----|
| Local | `http://192.168.1.143:3000` |
| Internal | `http://open-webui.home` |
| External | `https://open-webui.terrac.com` |

First user to sign up becomes admin.

---

## Directory Structure

```text
/data01/services/open-webui/
├── data/           # Database and uploads
├── docker-compose.yml
└── .env
```

---

## Management

```bash
# Service
systemctl status open-webui
systemctl restart open-webui

# Logs
docker logs open-webui --tail 50

# Health check
curl http://localhost:3000/
```

---

## Troubleshooting

| Issue | Check |
|-------|-------|
| Can't connect to API | `systemctl status llamacpp` |
| UI not accessible | `docker logs open-webui` |
| Database issues | Reset: `rm /data01/services/open-webui/data/webui.db` |

---

## Related Documentation

- [playbooks/individual/ocean/ai/open_webui.yaml](/playbooks/individual/ocean/ai/open_webui.yaml)
- [playbooks/individual/ocean/ai/llamacpp.yaml](/playbooks/individual/ocean/ai/llamacpp.yaml)
- [docs/architecture/ocean-services.md](/docs/architecture/ocean-services.md)
