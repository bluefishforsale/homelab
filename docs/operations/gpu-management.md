# GPU Management Operations

NVIDIA GPU management, monitoring, and optimization for AI/ML workloads.

---

## Quick Reference

| Component | Value |
|-----------|-------|
| GPU | NVIDIA RTX 3090 (24GB VRAM) |
| Host | ocean (192.168.1.143) on node006 |
| Passthrough | PCI 42:00.0 via VFIO |
| Exporter Port | 9835 |
| CUDA Version | 12.x |

---

## GPU Status Check

```bash
# SSH to ocean and check GPU
ssh terrac@192.168.1.143

# Basic status
nvidia-smi

# Detailed info
nvidia-smi -q -d MEMORY,UTILIZATION,TEMPERATURE,POWER

# Watch real-time
watch -n 1 nvidia-smi
```

---

## Driver Management

### Check Driver Status

```bash
# Check if GPU is visible
lspci | grep -i nvidia

# Check loaded modules
lsmod | grep nvidia

# Check driver version
nvidia-smi | grep "Driver Version"
```

### Driver Persistence

```bash
# Enable persistence mode
sudo nvidia-smi -pm 1

# Check persistence status
nvidia-smi -q | grep "Persistence Mode"
```

---

## Docker GPU Integration

NVIDIA Container Toolkit is deployed via Ansible:

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/docker_ce.yaml -l ocean --ask-vault-pass
```

### Test GPU Access

```bash
# Test NVIDIA runtime
docker run --rm --gpus all nvidia/cuda:12.2.0-base-ubuntu22.04 nvidia-smi

# Test with gpu-test container
ssh terrac@192.168.1.143 "docker exec gpu-test nvidia-smi"
```

### Docker Compose GPU Configuration

```yaml
# Example with GPU reservations (recommended)
services:
  ai-service:
    image: your-ai-image:latest
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
      - NVIDIA_DRIVER_CAPABILITIES=compute,utility
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu, compute, utility]
```

---

## Performance Monitoring

### Real-Time Monitoring

```bash
# Watch GPU status
watch -n 1 nvidia-smi

# Process monitoring
nvidia-smi pmon

# Detailed metrics
nvidia-smi -q -d MEMORY,UTILIZATION,TEMPERATURE,POWER
```

### Query Specific Metrics

```bash
# Temperature
nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader,nounits

# Memory usage
nvidia-smi --query-gpu=memory.used,memory.total --format=csv,noheader

# Power draw
nvidia-smi --query-gpu=power.draw --format=csv,noheader

# Utilization
nvidia-smi --query-gpu=utilization.gpu,utilization.memory --format=csv,noheader
```

---

## GPU Services

### llama.cpp (LLM Server)

```bash
# Deploy
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/llamacpp.yaml --ask-vault-pass

# Check GPU layers
ssh terrac@192.168.1.143 "docker logs llamacpp 2>&1 | grep -i 'offload\|layer'"

# Access: http://192.168.1.143:8080
```

### ComfyUI (Image Generation)

```bash
# Deploy
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/ai/comfyui.yaml --ask-vault-pass

# Access: http://192.168.1.143:8188
```

### Plex (Hardware Transcoding)

```bash
# Deploy
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/plex.yaml --ask-vault-pass

# Check NVENC usage during transcoding
ssh terrac@192.168.1.143 "nvidia-smi pmon -s u -d 1"
```

---

## Troubleshooting

### GPU Not Detected

```bash
# Check PCI
lspci | grep -i nvidia

# Check driver modules
lsmod | grep nvidia

# Check dmesg for errors
dmesg | grep -i nvidia | tail -20

# Reload modules
sudo modprobe -r nvidia_drm nvidia_modeset nvidia
sudo modprobe nvidia nvidia_modeset nvidia_drm
```

### Docker GPU Issues

```bash
# Check runtime
docker info | grep -i runtime

# Test GPU access
docker run --rm --gpus all nvidia/cuda:12.2.0-base-ubuntu22.04 nvidia-smi

# Check daemon config
cat /etc/docker/daemon.json
```

### Performance Issues

```bash
# Check throttling
nvidia-smi --query-gpu=clocks_throttle_reasons.active --format=csv

# Check PCIe link
nvidia-smi --query-gpu=pci.link.gen.current,pci.link.width.current --format=csv
```

---

## Prometheus GPU Exporter

GPU metrics are collected via `utkuozdemir/nvidia_gpu_exporter`, deployed with the node-exporter playbook.

### Deploy

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/node_exporter.yaml -l ocean
```

### Verify

```bash
# Check service
ssh terrac@192.168.1.143 "systemctl status nvidia-gpu-exporter"

# Check metrics
curl -s http://192.168.1.143:9835/metrics | grep nvidia_gpu | head -10
```

### Key Metrics

| Metric | Description |
|--------|-------------|
| `nvidia_gpu_utilization_gpu_percentage` | GPU utilization |
| `nvidia_gpu_memory_used_bytes` | VRAM used |
| `nvidia_gpu_memory_total_bytes` | Total VRAM |
| `nvidia_gpu_temperature_celsius` | GPU temperature |
| `nvidia_gpu_power_watts` | Power draw |

---

## GPU Passthrough (Proxmox)

The RTX 3090 is passed through to ocean VM on node006.

### Verify on Proxmox Host

```bash
# SSH to node006
ssh root@192.168.1.106

# Check VFIO binding
lspci -nnk -s 42:00 | grep -A 2 "Kernel driver"
# Should show: Kernel driver in use: vfio-pci
```

### Verify in VM

```bash
# SSH to ocean
ssh terrac@192.168.1.143

# Check GPU is visible
lspci | grep -i nvidia
nvidia-smi
```

See [ocean-migration-plan.md](ocean-migration-plan.md) for full passthrough setup.

---

## Emergency Procedures

### GPU Unresponsive

```bash
# Stop GPU containers
ssh terrac@192.168.1.143 "sudo systemctl stop llamacpp comfyui plex"

# Reset GPU
ssh terrac@192.168.1.143 "sudo nvidia-smi --gpu-reset"

# Restart services
ssh terrac@192.168.1.143 "sudo systemctl start llamacpp comfyui plex"
```

### Thermal Emergency (>85°C)

```bash
# Check temperature
ssh terrac@192.168.1.143 "nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader"

# Stop high-load services
ssh terrac@192.168.1.143 "sudo systemctl stop llamacpp comfyui"

# Monitor until temperature drops
ssh terrac@192.168.1.143 "watch -n 1 nvidia-smi --query-gpu=temperature.gpu --format=csv"
```

---

## Related Documentation

- [ocean-migration-plan.md](ocean-migration-plan.md) - GPU passthrough setup
- [dell-hardware.md](dell-hardware.md) - Server hardware details
- [files/gpu-test/README.md](/files/gpu-test/README.md) - GPU test container
