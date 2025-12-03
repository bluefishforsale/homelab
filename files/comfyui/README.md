# ComfyUI

GPU-accelerated image generation with Stable Diffusion and Flux models.

---

## Quick Reference

| Property | Value |
|----------|-------|
| Host | ocean (192.168.1.143) |
| Port | 8188 |
| Image | yanwk/comfyui-boot:cu126-slim |
| GPU | RTX 3090 (24GB VRAM) |
| Storage | /data01/services/comfyui |

---

## Architecture

```text
Browser → nginx → ComfyUI (:8188) → RTX 3090
                       ↓
              /data01/services/comfyui/ComfyUI/models/
```

---

## Deploy

```bash
# Full deployment with models
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/comfyui.yaml --ask-vault-pass

# Skip model downloads
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/comfyui.yaml --ask-vault-pass --skip-tags models
```

---

## Access

| Method | URL |
|--------|-----|
| Local | `http://192.168.1.143:8188` |
| Internal | `http://comfyui.home` |
| External | `https://comfyui.terrac.com` |

---

## Directory Structure

```text
/data01/services/comfyui/
├── ComfyUI/
│   ├── models/         # Checkpoints, VAE, LoRA, etc.
│   ├── input/          # Input images
│   ├── output/         # Generated images
│   └── custom_nodes/   # Extensions
├── docker-compose.yml
└── .env
```

---

## Models

Models defined in `vars_comfyui_models.yaml`:

| Model | Size | Directory |
|-------|------|-----------|
| flux1-dev-fp8 | 11.9GB | diffusion_models |
| ae.safetensors (VAE) | 0.3GB | vae |
| t5xxl_fp8 | 4.9GB | clip |
| clip_l | 0.25GB | clip |

Add models by editing `vars_comfyui_models.yaml`.

---

## Management

```bash
# Service
systemctl status comfyui.service
systemctl restart comfyui.service

# Logs
docker logs comfyui --tail 50

# GPU status
nvidia-smi
```

---

## Troubleshooting

| Issue | Check |
|-------|-------|
| Container won't start | `nvidia-smi`, GPU drivers |
| Out of memory | Use LOWVRAM mode in .env |
| Custom nodes not loading | Restart service |

---

## Related Documentation

- [playbooks/individual/ocean/ai/comfyui.yaml](/playbooks/individual/ocean/ai/comfyui.yaml)
- [vars_comfyui_models.yaml](/playbooks/individual/ocean/ai/vars_comfyui_models.yaml)
- [docs/architecture/ocean-services.md](/docs/architecture/ocean-services.md)
