# 🎮 GPU Management Operations Guide

## Overview

This guide covers NVIDIA GPU management, monitoring, and optimization for your homelab's AI/ML workloads and virtualization.

## 📋 GPU Hardware Overview

### NVIDIA P2000 Specifications
- **Architecture**: Pascal
- **CUDA Cores**: 1024
- **Memory**: 5GB GDDR5
- **Memory Bandwidth**: 140 GB/s  
- **Power Consumption**: 75W
- **CUDA Compute Capability**: 6.1

### Supported Workloads
- **AI/ML Inference**: ComfyUI, llama.cpp, Open WebUI
- **Video Transcoding**: Tdarr, Plex hardware acceleration
- **Containerized GPU**: Docker runtime nvidia
- **Virtualization**: GPU passthrough to VMs

## 🚀 GPU Driver Management

### NVIDIA Driver Installation
```bash
# Check current GPU status
lspci | grep -i nvidia
nvidia-smi  # If drivers installed

# Remove old drivers (if needed)
sudo apt remove --purge nvidia-*
sudo apt autoremove

# Install recommended drivers
sudo apt update
sudo apt install nvidia-driver-470  # For P2000 compatibility
sudo apt install nvidia-utils-470

# Reboot to load drivers
sudo reboot

# Verify installation
nvidia-smi
nvidia-settings --version
```

### CUDA Runtime Installation
```bash
# Install CUDA toolkit (version compatible with P2000)
wget https://developer.download.nvidia.com/compute/cuda/11.8.0/local_installers/cuda_11.8.0_520.61.05_linux.run
sudo sh cuda_11.8.0_520.61.05_linux.run --toolkit --silent

# Add CUDA to PATH
echo 'export PATH=/usr/local/cuda/bin:$PATH' >> ~/.bashrc
echo 'export LD_LIBRARY_PATH=/usr/local/cuda/lib64:$LD_LIBRARY_PATH' >> ~/.bashrc
source ~/.bashrc

# Verify CUDA installation
nvcc --version
nvidia-smi
```

### Driver Persistence
```bash
# Enable persistence mode (recommended for servers)
sudo nvidia-smi -pm 1

# Set performance mode
sudo nvidia-smi -lgc 1328,1531  # Set GPU clock speeds for P2000

# Create systemd service for persistence
cat > /etc/systemd/system/nvidia-persistence.service << EOF
[Unit]
Description=NVIDIA Persistence Daemon
Wants=syslog.target

[Service]
Type=forking
ExecStart=/usr/bin/nvidia-persistenced --verbose
ExecStopPost=/bin/rm -rf /var/run/nvidia-persistenced

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now nvidia-persistence
```

## 🐳 Docker GPU Integration

### Docker Runtime Configuration
```bash
# Install nvidia-container-runtime
distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
curl -s -L https://nvidia.github.io/nvidia-docker/gpgkey | sudo apt-key add -
curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.list | sudo tee /etc/apt/sources.list.d/nvidia-docker.list

sudo apt update
sudo apt install nvidia-container-runtime nvidia-docker2

# Restart Docker
sudo systemctl restart docker
```

### Test GPU Access in Container
```bash
# Test NVIDIA runtime
docker run --rm --runtime=nvidia --gpus all nvidia/cuda:11.8-base-ubuntu20.04 nvidia-smi

# Verify CUDA availability
docker run --rm --runtime=nvidia --gpus all nvidia/cuda:11.8-devel-ubuntu20.04 nvcc --version
```

### Docker Compose GPU Configuration
```yaml
# Example docker-compose.yml with GPU access
version: '3.8'
services:
  ai-service:
    image: your-ai-image:latest
    runtime: nvidia
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
      - NVIDIA_DRIVER_CAPABILITIES=compute,utility
    volumes:
      - ./models:/app/models
    ports:
      - "8080:8080"
```

## 🔧 GPU Performance Optimization

### Performance Monitoring
```bash
# Real-time GPU monitoring
nvidia-smi -l 1  # Update every second
watch -n 1 nvidia-smi

# Detailed GPU information
nvidia-smi -q -d MEMORY,UTILIZATION,ECC,TEMPERATURE,POWER,CLOCK,COMPUTE

# GPU process monitoring
nvidia-smi pmon  # Process monitoring mode
```

### Power and Thermal Management
```bash
# Check power limits
nvidia-smi -q -d POWER

# Set power limit (if supported)
sudo nvidia-smi -pl 65  # Set to 65W (lower than 75W max for P2000)

# Monitor temperatures
nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader,nounits
```

### Memory Management
```bash
# Check GPU memory usage
nvidia-smi --query-gpu=memory.total,memory.used,memory.free --format=csv,noheader,nounits

# Clear GPU memory (kill processes using GPU)
sudo fuser -v /dev/nvidia*
# Kill specific processes if needed
```

## 🎯 Service-Specific GPU Configuration

### ComfyUI Optimization
Following the YanWenKun image configuration from your memories:
```yaml
# docker-compose.yml for ComfyUI
services:
  comfyui:
    image: yanwk/comfyui-boot:cu126-slim
    runtime: nvidia
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
      - CLI_ARGS=--fast --use-pytorch-cross-attention --fp16-vae
    volumes:
      - /data01/services/comfyui:/root
    ports:
      - "8188:8188"
```

### llama.cpp Server Configuration  
Following the server-cuda configuration from your memories:
```yaml
# docker-compose.yml for llama.cpp
services:
  llama-cpp:
    image: ghcr.io/ggerganov/llama.cpp:server-cuda
    runtime: nvidia
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
    command: >
      --host 0.0.0.0
      --port 8080
      --model ""
      --verbose
      --n-gpu-layers 32
    volumes:
      - /data01/services/llama-cpp/models:/models
    ports:
      - "8080:8080"
```

### Plex Hardware Acceleration
```bash
# Enable hardware transcoding in Plex
# Settings > Server > Transcoder
# Enable: "Use hardware acceleration when available"
# Hardware transcoding device: NVIDIA NVENC

# Monitor transcoding sessions
tail -f '/var/lib/plexmediaserver/Library/Application Support/Plex Media Server/Logs/Plex Media Scanner.log'
```

## 🔍 GPU Troubleshooting

### Common Issues

#### GPU Not Detected
```bash
# Check if GPU is visible to system
lspci | grep -i vga
lspci | grep -i nvidia

# Check if drivers are loaded
lsmod | grep nvidia

# Reload nvidia modules
sudo modprobe -r nvidia_drm nvidia_modeset nvidia
sudo modprobe nvidia nvidia_modeset nvidia_drm

# Check for conflicts
dmesg | grep -i nvidia | tail -20
```

#### CUDA Errors
```bash
# Check CUDA driver version compatibility
nvidia-smi | grep "CUDA Version"
nvcc --version

# Test CUDA functionality
cat > test_cuda.cu << EOF
#include <stdio.h>
__global__ void hello() {
    printf("Hello from GPU!\n");
}
int main() {
    hello<<<1,1>>>();
    cudaDeviceSynchronize();
    return 0;
}
EOF

nvcc test_cuda.cu -o test_cuda
./test_cuda
```

#### Docker GPU Issues
```bash
# Check nvidia-container-runtime
docker info | grep nvidia

# Test basic GPU access
docker run --rm --gpus all nvidia/cuda:11.8-base-ubuntu20.04 nvidia-smi

# Check runtime configuration
cat /etc/docker/daemon.json
```

#### Performance Issues
```bash
# Check for thermal throttling
nvidia-smi --query-gpu=clocks_throttle_reasons.active --format=csv,noheader,nounits

# Monitor power consumption
nvidia-smi --query-gpu=power.draw --format=csv,noheader,nounits

# Check PCIe bandwidth
nvidia-smi --query-gpu=pci.link.gen.current,pci.link.width.current --format=csv,noheader,nounits
```

## 📊 GPU Monitoring & Alerting

### Prometheus GPU Exporter
```bash
# Install nvidia_gpu_exporter
wget https://github.com/mindprince/nvidia_gpu_prometheus_exporter/releases/download/v1.2.0/nvidia_gpu_prometheus_exporter-1.2.0.linux-amd64.tar.gz
tar xzf nvidia_gpu_prometheus_exporter-1.2.0.linux-amd64.tar.gz
sudo cp nvidia_gpu_prometheus_exporter /usr/local/bin/

# Create systemd service
cat > /etc/systemd/system/nvidia-gpu-exporter.service << EOF
[Unit]
Description=NVIDIA GPU Prometheus Exporter
After=network.target

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/nvidia_gpu_prometheus_exporter --web.listen-address=:9445
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now nvidia-gpu-exporter
```

### Key Metrics to Monitor
```yaml
# GPU utilization percentage
nvidia_gpu_utilization_gpu_percentage

# GPU memory usage
nvidia_gpu_memory_used_bytes
nvidia_gpu_memory_total_bytes

# GPU temperature
nvidia_gpu_temperature_celsius

# Power consumption
nvidia_gpu_power_watts

# Fan speed
nvidia_gpu_fanspeed_percentage
```

### Grafana Dashboard
Key panels for GPU monitoring:
- GPU utilization over time
- Memory usage (used/total)
- Temperature trends
- Power consumption
- Process list with GPU usage
- Error rate monitoring

## 🖥️ Virtualization & Passthrough

### GPU Passthrough Setup
```bash
# Enable IOMMU in GRUB (if not already done)
echo 'GRUB_CMDLINE_LINUX_DEFAULT="quiet intel_iommu=on iommu=pt"' >> /etc/default/grub
update-grub

# Load VFIO modules
echo 'vfio
vfio_iommu_type1  
vfio_pci
vfio_virqfd' >> /etc/modules

# Bind GPU to VFIO
lspci | grep NVIDIA  # Note the PCIe ID (e.g., 01:00.0)
lspci -n -s 01:00.0  # Get vendor:device ID (e.g., 10de:1c30)

echo 'options vfio-pci ids=10de:1c30' >> /etc/modprobe.d/vfio.conf
update-initramfs -u
reboot
```

### Verify Passthrough Setup
```bash
# Check IOMMU groups
find /sys/kernel/iommu_groups/ -type l | grep 01:00.0

# Verify VFIO binding
lspci -nnk | grep -A 3 NVIDIA
```

## 🔄 Maintenance Procedures

### Regular Maintenance Tasks

#### Daily Checks
```bash
# GPU health status
nvidia-smi --query-gpu=name,driver_version,temperature.gpu,power.draw,utilization.gpu --format=csv,noheader

# Check for errors
dmesg | grep -i "nvidia\|gpu\|cuda" | tail -10
```

#### Weekly Maintenance
```bash
# Clean GPU memory
sudo nvidia-smi --gpu-reset  # Only if no processes running

# Check driver updates
apt list --upgradable | grep nvidia

# Review GPU utilization trends
# Check Grafana dashboards for patterns
```

#### Monthly Tasks
```bash
# Comprehensive GPU diagnostics
nvidia-smi -q > /tmp/gpu-status-$(date +%Y%m%d).log

# Review and update GPU configurations
# Check for new CUDA versions
# Review container image updates (ComfyUI, llama.cpp, etc.)
```

## 🚨 Emergency Procedures

### GPU Failure Recovery
```bash
# If GPU becomes unresponsive
sudo systemctl stop docker  # Stop GPU containers
sudo nvidia-smi --gpu-reset
sudo systemctl start docker

# If driver issues
sudo systemctl stop nvidia-persistenced
sudo modprobe -r nvidia_drm nvidia_modeset nvidia
sudo modprobe nvidia nvidia_modeset nvidia_drm
sudo systemctl start nvidia-persistenced
```

### Thermal Emergency
```bash
# If GPU overheating (>80°C for P2000)
# 1. Check current temperature
nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader,nounits

# 2. Reduce power limit temporarily
sudo nvidia-smi -pl 50  # Reduce to 50W

# 3. Stop high-load processes
docker stop $(docker ps -q --filter ancestor=yanwk/comfyui-boot:cu126-slim)

# 4. Check physical ventilation
# 5. Resume normal operations once temperature drops
```

## 📋 GPU Optimization Checklist

### Performance Optimization
- [ ] Enable GPU persistence mode
- [ ] Set appropriate power limits
- [ ] Configure optimal clock speeds
- [ ] Enable hardware acceleration in applications
- [ ] Monitor thermal performance

### Container Optimization
- [ ] Use runtime: nvidia for Docker
- [ ] Set NVIDIA_VISIBLE_DEVICES appropriately
- [ ] Optimize CUDA image versions
- [ ] Configure GPU memory limits if needed
- [ ] Monitor container GPU usage

### Application-Specific Tuning
- [ ] ComfyUI: Enable fp16-vae and cross-attention optimization
- [ ] llama.cpp: Configure n-gpu-layers based on VRAM
- [ ] Plex: Enable NVENC hardware transcoding
- [ ] N8N: GPU access for AI workflows

This GPU management guide ensures optimal performance and reliability of your NVIDIA P2000 while supporting the AI/ML services in your homelab environment.
