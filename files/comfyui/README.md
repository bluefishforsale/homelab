# ComfyUI Deployment

ComfyUI is a powerful and modular Stable Diffusion GUI and backend. This deployment provides GPU-accelerated image generation with CUDA support.

## Architecture

- **Docker Image**: `yanwk/comfyui-boot:cu126-slim`
- **Port**: 8188 (web interface)
- **User**: root (container default)
- **Data Directory**: `/data01/services/comfyui`
- **GPU Support**: NVIDIA P2000 with CUDA 12.6 runtime

## Directory Structure

```text
/data01/services/comfyui/                # Host storage
├── ComfyUI/                           # ComfyUI user data
│   ├── models/                        # All model types → /root/ComfyUI/models/
│   │   ├── checkpoints/               # Model checkpoints  
│   │   ├── vae/                       # VAE models
│   │   ├── clip/                      # Text encoders
│   │   ├── unet/                      # UNET models
│   │   ├── workflows/                 # Example workflows with embedded data
│   │   └── ...                        # 15+ other model type directories
│   ├── input/                         # Input images → /root/ComfyUI/input/
│   ├── output/                        # Generated images → /root/ComfyUI/output/
│   └── custom_nodes/                  # Custom nodes → /root/ComfyUI/custom_nodes/
├── .cache/                            # Python cache → /root/.cache/
├── .local/                            # Python packages → /root/.local/
├── docker-compose.yml
└── .env

Container (/root/ComfyUI/):            # Built-in ComfyUI installation preserved
├── comfy/                             # Core ComfyUI files (sd1_clip_config.json, etc.)
├── models/ → mounted from host        # User models
├── input/ → mounted from host         # User inputs  
├── output/ → mounted from host        # User outputs
└── custom_nodes/ → mounted from host  # User extensions
```

## Usage

### Deployment

```bash
ansible-playbook playbook_ocean_comfyui.yaml
```

### Service Management

```bash
# Status
sudo systemctl status comfyui.service

# Start/Stop
sudo systemctl start comfyui.service
sudo systemctl stop comfyui.service

# Restart
sudo systemctl restart comfyui.service

# View logs
sudo journalctl -u comfyui.service -f
```

### Web Interface
Access ComfyUI at: `http://ocean:8188`

## Models

### Automatic Downloads

The playbook automatically downloads models defined in `vars_comfyui_models.yaml`:

**Stable Diffusion 1.5 Models:**
- **SD 1.5 Checkpoint**: `v1-5-pruned-emaonly-fp16.safetensors` (~2.1GB)

**Flux Development Models (Complete Set):**
- **Flux Diffusion Model**: `FLUX1/flux1-dev-fp8.safetensors` (~11.9GB) in `diffusion_models/` (primary)
- **Flux Checkpoint**: `flux1-dev-fp8.safetensors` in `checkpoints/` (symlinked for compatibility)
- **Flux UNET**: `flux1-dev.safetensors` (~23.8GB) in `unet/`
- **Flux VAE**: `ae.safetensors` (~0.3GB) in `vae/`
- **T5XXL Text Encoders**: `t5xxl_fp8.safetensors` (~4.9GB) + `t5xxl_fp16.safetensors` (~9.8GB) in `clip/`
- **CLIP-L Text Encoder**: `clip_l.safetensors` (~0.25GB) in `clip/`

**Example Workflows:**
- **Flux Dev Example**: `flux_dev_example.png` (contains embedded workflow data)

**Total Download Size**: ~41GB (space-efficient with symlinks)

### Model Management

Add new models by editing `vars_comfyui_models.yaml`:

```yaml
comfyui_models:
  checkpoints:
    - name: your-model.safetensors
      url: https://example.com/model.safetensors
      size_gb: 2.0
      timeout: 300
```

### Download Reliability

The model downloader includes robust error handling:

- **Independent Downloads**: Each model downloads independently - one failure won't stop others
- **Detailed Reporting**: Shows status for each model (DOWNLOADED/ALREADY EXISTS/FAILED)
- **Error Details**: Failed downloads show URL and specific error message
- **Graceful Failures**: Bad URLs or 404s won't break the entire deployment

Example output:

```text
Model Download Summary:
- checkpoints/flux1-dev-fp8.safetensors: DOWNLOADED
- vae/ae.safetensors: ALREADY EXISTS  
- clip/bad-model.safetensors: FAILED (HTTP Error 404: Not Found)
```

### Model Directories

All supported model types are automatically created:

- **Checkpoints**: `/data01/services/comfyui/ComfyUI/models/checkpoints/`
- **VAE**: `/data01/services/comfyui/ComfyUI/models/vae/`
- **LoRA**: `/data01/services/comfyui/ComfyUI/models/loras/`
- **Embeddings**: `/data01/services/comfyui/ComfyUI/models/embeddings/`
- **ControlNet**: `/data01/services/comfyui/ComfyUI/models/controlnet/`
- **Upscale**: `/data01/services/comfyui/ComfyUI/models/upscale_models/`

## GPU Configuration

The service is configured for GPU acceleration with:
- NVIDIA runtime support
- All GPUs visible (`NVIDIA_VISIBLE_DEVICES=all`)
- High VRAM usage mode for optimal performance
- PyTorch attention optimizations enabled

## Troubleshooting

### Container won't start
Check GPU driver and CUDA installation:
```bash
nvidia-smi
docker run --rm --gpus all nvidia/cuda:11.0-base nvidia-smi
```

### Out of memory errors
Adjust VRAM settings in the environment file:
```bash
# For lower VRAM GPUs
COMFYUI_LOWVRAM=1
COMFYUI_HIGHVRAM=0
```

### Custom nodes not loading
Ensure custom nodes are properly installed in the custom_nodes directory and restart the service.
