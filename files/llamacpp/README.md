# llama.cpp API Server Deployment

This deployment provides a GPU-accelerated llama.cpp server with API endpoints for model inference.

## Features

- **GPU Support**: Full Nvidia P2000 GPU acceleration using CUDA
- **API Server**: REST API endpoints for completions and embeddings
- **Docker Compose**: Robust container orchestration with health checks
- **Systemd Integration**: System service management with auto-restart
- **Security**: Restricted permissions and sandboxed execution
- **Persistence**: Models stored in `/data01/services/llamacpp/models`

## Directory Structure

```
/data01/services/llamacpp/
├── docker-compose.yml    # Generated from template
├── .env                 # Environment variables
├── models/              # Model storage directory
├── config/              # Configuration files
└── logs/                # Application logs
```

## API Endpoints

Once deployed, the server exposes these endpoints on port 8080:

- `GET /health` - Health check
- `POST /completion` - Text completion
- `POST /v1/chat/completions` - Chat completions (OpenAI-compatible)
- `POST /embedding` - Generate embeddings
- `GET /v1/models` - List loaded models

## Deployment

Run the playbook to deploy:

```bash
ansible-playbook -i inventory.ini playbook_ocean_llamacpp.yaml
```

## Service Management

```bash
# Check service status
sudo systemctl status llamacpp

# Start/stop service
sudo systemctl start llamacpp
sudo systemctl stop llamacpp

# View logs
sudo journalctl -u llamacpp -f
```

## Model Management

### Download a Model

Models should be in GGUF format. Download to the models directory:

```bash
# Example: Download Llama 3.1 8B model (4-bit quantized)
cd /data01/services/llamacpp/models
wget https://huggingface.co/bartowski/Meta-Llama-3.1-8B-Instruct-GGUF/resolve/main/Meta-Llama-3.1-8B-Instruct-Q4_K_M.gguf

# Or download Phi-3.5 Mini (smaller model for testing)
wget https://huggingface.co/bartowski/Phi-3.5-mini-instruct-GGUF/resolve/main/Phi-3.5-mini-instruct-Q4_K_M.gguf
```

### Load a Model

Once downloaded, load the model via API:

```bash
# Load Llama 3.1 8B model
curl -X POST http://localhost:8080/v1/models \
  -H "Content-Type: application/json" \
  -d '{
    "model": "/models/Meta-Llama-3.1-8B-Instruct-Q4_K_M.gguf"
  }'

# Or load Phi-3.5 Mini
curl -X POST http://localhost:8080/v1/models \
  -H "Content-Type: application/json" \
  -d '{
    "model": "/models/Phi-3.5-mini-instruct-Q4_K_M.gguf"
  }'
```

### Test the Model

Once a model is loaded, test it with a completion:

```bash
# Test chat completion
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [
      {"role": "user", "content": "What is the capital of France?"}
    ],
    "max_tokens": 100,
    "temperature": 0.7
  }'

# Test simple completion
curl -X POST http://localhost:8080/completion \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "The capital of France is",
    "n_predict": 10
  }'
```

### Check Model Status

```bash
# List currently loaded models
curl http://localhost:8080/v1/models

# Check server health
curl http://localhost:8080/health
```

## Troubleshooting

### Check GPU Access
```bash
# Verify GPU is available in container
docker exec llamacpp nvidia-smi

# Check CUDA availability
docker exec llamacpp nvidia-ml-py --query-gpu=name,memory.total,memory.used --format=csv
```

### View Container Logs
```bash
# Follow container logs
docker logs -f llamacpp

# Check docker-compose logs
cd /data01/services/llamacpp
docker-compose logs -f
```

### Common Issues

1. **GPU not detected**: Ensure nvidia-docker2 is installed and docker daemon restarted
2. **Model loading fails**: Check model file permissions and path
3. **Out of memory**: Reduce context size or use smaller model quantization
4. **API timeouts**: Increase timeout values in environment file

## Performance Tuning

Edit `/data01/services/llamacpp/.env` to adjust performance parameters:

- `LLAMA_ARG_N_THREADS`: CPU threads to use
- `LLAMA_ARG_N_PARALLEL`: Parallel sequences
- `LLAMA_ARG_BATCH_SIZE`: Batch size for processing
- `LLAMA_ARG_CTX_SIZE`: Context window size

Restart service after changes:
```bash
sudo systemctl restart llamacpp
```
