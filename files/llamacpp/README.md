# llama.cpp API Server

GPU-accelerated LLM inference server with OpenAI-compatible API.

---

## Quick Reference

| Setting | Value |
|---------|-------|
| Image | ghcr.io/ggerganov/llama.cpp:server-cuda |
| Port | 8080 |
| GPU | RTX 3090 (CUDA) |
| Default Model | Qwen3-14B-Q4_K_M.gguf |
| API Key | `llamacpp-homelab-key` |

---

## Deployment

```bash
# Deploy with automatic model download (requires vault for HF token)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/llamacpp.yaml --ask-vault-pass

# Skip model downloads
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/llamacpp.yaml --ask-vault-pass --skip-tags models
```

Models are automatically downloaded from HuggingFace based on `vars/vars_llamacpp_models.yaml`.

---

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml.j2` | Container with GPU config |
| `llamacpp.service.j2` | Systemd service |
| `llamacpp.env.j2` | Environment variables |

---

## Directory Structure

```text
/data01/services/llamacpp/
├── docker-compose.yml
├── .env
├── models/              # GGUF model files
├── config/
└── logs/
```

---

## Default Configuration (RTX 3090 Optimized)

```yaml
model: Qwen3-14B-Q4_K_M.gguf
n-gpu-layers: 41
ctx-size: 40960
parallel: 1
batch-size: 2048
ubatch-size: 512
flash-attn: enabled
```

---

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/v1/models` | GET | List loaded models |
| `/v1/chat/completions` | POST | Chat completions (OpenAI-compatible) |
| `/completion` | POST | Text completion |
| `/embedding` | POST | Generate embeddings |

**Authentication**: Include `Authorization: Bearer llamacpp-homelab-key` header.

---

## Service Management

```bash
# Status
systemctl status llamacpp

# Restart
systemctl restart llamacpp

# Logs
journalctl -u llamacpp -f
docker logs -f llamacpp
```

---

## Testing

```bash
# Health check
curl http://localhost:8080/health

# List models
curl -H "Authorization: Bearer llamacpp-homelab-key" \
  http://localhost:8080/v1/models

# Chat completion
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer llamacpp-homelab-key" \
  -d '{
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 100
  }'
```

---

## Troubleshooting

### Check GPU Access

```bash
docker exec llamacpp nvidia-smi
```

### View Logs

```bash
docker logs -f llamacpp
```

### Common Issues

- **GPU not detected**: Ensure NVIDIA Container Toolkit installed
- **Out of memory**: Reduce `ctx-size` or use smaller quantization
- **Model loading fails**: Check model permissions and path

---

## Model Management

Models defined in `vars/vars_llamacpp_models.yaml`. Playbook downloads priority 1-2 models automatically.

### Manual Model Download

```bash
cd /data01/services/llamacpp/models
# Public models
wget https://huggingface.co/Qwen/Qwen3-14B-GGUF/resolve/main/Qwen3-14B-Q4_K_M.gguf

# Authenticated models (need HF token)
curl -H "Authorization: Bearer YOUR_HF_TOKEN" \
  -L -o model.gguf \
  "https://huggingface.co/repo/resolve/main/model.gguf"
```

---

## Performance Tuning

Environment variables in `.env`:

| Variable | Default | Description |
|----------|---------|-------------|
| `LLAMA_ARG_CTX_SIZE` | 4096 | Context window |
| `LLAMA_ARG_N_GPU_LAYERS` | -1 | GPU layers (-1 = all) |
| `LLAMA_ARG_N_THREADS` | 8 | CPU threads |
| `LLAMA_ARG_BATCH_SIZE` | 512 | Batch size |
| `LLAMA_ARG_FLASH_ATTN` | 1 | Flash attention |

**Note**: Command-line args in docker-compose.yml override .env for loaded model.
