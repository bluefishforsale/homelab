# NVIDIA GPU Exporter Docker Compose

Docker Compose deployment for NVIDIA GPU Prometheus exporter using utkuozdemir/nvidia_gpu_exporter.

**Note**: This GPU exporter is deployed as part of the consolidated `playbook_node-exporter.yaml` playbook alongside other hardware exporters (cadvisor, node-exporter, process-exporter).

## Overview

This service exports NVIDIA GPU metrics for Prometheus monitoring, providing comprehensive GPU telemetry for AI/ML workloads and hardware monitoring.

## Components

- **Docker Compose**: Container orchestration and configuration
- **Health Checks**: curl-based endpoint monitoring
- **Systemd Integration**: Service management and auto-start
- **GPU Runtime**: NVIDIA Docker runtime for GPU access

## Files

- `docker-compose.yml.j2` - Main Docker Compose template
- `nvidia-gpu-exporter.service.j2` - Systemd service template
- `nvidia-gpu-exporter.env.j2` - Environment configuration template
- `README.md` - This documentation

## Configuration

### Environment Variables
- `NVIDIA_GPU_EXPORTER_PORT`: Metrics endpoint port (default: 9445)
- `NVIDIA_GPU_EXPORTER_VERSION`: Container image version
- `NVIDIA_VISIBLE_DEVICES`: GPU device visibility (all)
- `NVIDIA_DRIVER_CAPABILITIES`: Driver capabilities (compute,utility)

### Health Check
- **Method**: bash TCP socket connection (since curl/wget not available in container)
- **Port**: 9835 (localhost)
- **Command**: `timeout 5 bash -c "</dev/tcp/localhost/9835"`
- **Interval**: 30 seconds
- **Timeout**: 10 seconds
- **Retries**: 3 attempts
- **Start Period**: 40 seconds

## Deployment

```bash
# Deploy via consolidated node-exporter playbook (includes all hardware exporters)
ansible-playbook playbook_node-exporter.yaml

# The playbook will:
# - Deploy cadvisor, node-exporter, process-exporter on all hosts
# - Additionally deploy nvidia-gpu-exporter on GPU-enabled hosts only
# - Create Docker Compose setup with health checks

# Manual GPU exporter operations (if needed)
cd /data01/services/nvidia-gpu-exporter
docker-compose up -d
docker-compose ps
docker-compose logs -f

# Check all hardware exporters status
systemctl status cadvisor node-exporter process-exporter nvidia-gpu-exporter
```

## Monitoring

### Service Health
```bash
# Check systemd service
systemctl status nvidia-gpu-exporter

# Check container health
docker-compose ps
docker inspect nvidia-gpu-exporter --format='{{.State.Health.Status}}'

# View logs
docker-compose logs -f nvidia-gpu-exporter
```

### Metrics Verification
```bash
# Test metrics endpoint
curl http://localhost:9445/metrics | head -20

# Check key GPU metrics
curl -s http://localhost:9445/metrics | grep -E "(nvidia_gpu_utilization|nvidia_gpu_temperature|nvidia_gpu_memory)"

# Verify Prometheus scraping
curl -s "http://prometheus.home:9090/api/v1/query?query=nvidia_gpu_utilization_gpu_percentage"
```

## GPU Metrics

Key metrics exported by this service:

- `nvidia_gpu_utilization_gpu_percentage` - GPU utilization percentage
- `nvidia_gpu_memory_used_bytes` - GPU memory usage in bytes
- `nvidia_gpu_memory_total_bytes` - Total GPU memory in bytes
- `nvidia_gpu_temperature_celsius` - GPU temperature in Celsius
- `nvidia_gpu_power_watts` - GPU power consumption in watts
- `nvidia_gpu_fanspeed_percentage` - GPU fan speed percentage
- `nvidia_gpu_processes_count` - Number of processes using GPU

## Troubleshooting

### Common Issues

**Container fails to start:**
```bash
# Check GPU runtime availability
docker run --rm --gpus all nvidia/cuda:11.8-base-ubuntu20.04 nvidia-smi

# Verify NVIDIA Docker runtime
docker info | grep nvidia
```

**Health check failures:**
```bash
# Check if metrics endpoint is responding (using bash TCP socket)
docker exec nvidia-gpu-exporter timeout 5 bash -c "</dev/tcp/localhost/9835"

# Or test from host (since curl/wget might not be in container)
timeout 5 bash -c "</dev/tcp/localhost/9445" 2>/dev/null && echo "Port open" || echo "Port closed"

# Check container logs
docker-compose logs nvidia-gpu-exporter
```

**No GPU metrics:**
```bash
# Verify GPU devices are visible
docker exec nvidia-gpu-exporter nvidia-smi

# Check NVIDIA_VISIBLE_DEVICES setting
docker inspect nvidia-gpu-exporter | grep NVIDIA_VISIBLE_DEVICES
```

## Integration

### Prometheus Configuration
```yaml
- job_name: 'nvidia-gpu-exporter'
  static_configs:
  - targets: ['ocean.home:9445']
```

### Grafana Dashboards
Import or create dashboards using the exported GPU metrics for:
- GPU utilization trends
- Memory usage monitoring  
- Temperature tracking
- Power consumption analysis
- Process monitoring

## Security

- **No new privileges**: Container runs with no-new-privileges
- **Read-only filesystem**: Container filesystem is read-only where possible
- **Resource limits**: Memory limits prevent resource exhaustion
- **GPU device access**: Only necessary GPU devices are exposed

This deployment provides robust, monitored GPU telemetry for your homelab infrastructure.
