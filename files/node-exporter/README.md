# Hardware Exporters

Prometheus exporters for comprehensive host and hardware monitoring. All deployed via a single consolidated playbook.

---

## Quick Reference

| Exporter | Port | Image | Hosts |
|----------|------|-------|-------|
| node-exporter | 9100 | prom/node-exporter | All |
| cadvisor | 8912 | gcr.io/cadvisor/cadvisor | All |
| process-exporter | 9256 | ncabatoff/process-exporter | All |
| nvidia-gpu-exporter | 9445 | utkuozdemir/nvidia_gpu_exporter:1.4.0 | GPU only |
| smart-exporter | 9633 | prometheuscommunity/smartctl-exporter | All |

---

## Deployment

```bash
# Deploy all hardware exporters (no vault required)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/node_exporter.yaml

# The playbook automatically:
# - Deploys node-exporter, cadvisor, process-exporter, smart-exporter on all hosts
# - Detects NVIDIA GPU and deploys nvidia-gpu-exporter only on GPU hosts
# - Creates Docker Compose configs with health checks
# - Configures systemd services
```

---

## Files

| File | Purpose |
|------|---------|
| `node-exporter-compose.yml.j2` | Node exporter Docker Compose |
| `node-exporter.service.j2` | Node exporter systemd service |
| `node-exporter.env.j2` | Node exporter environment |
| `cadvisor-compose.yml.j2` | cAdvisor Docker Compose |
| `cadvisor.service.j2` | cAdvisor systemd service |
| `cadvisor.env.j2` | cAdvisor environment |
| `process-exporter.service.j2` | Process exporter systemd service (docker run, not compose) |
| `nvidia-gpu-exporter-compose.yml.j2` | GPU exporter Docker Compose |
| `nvidia-gpu-exporter.service.j2` | GPU exporter systemd service |
| `nvidia-gpu-exporter.env.j2` | GPU exporter environment |
| `smart-exporter-compose.yml.j2` | SMART exporter Docker Compose |
| `smart-exporter.service.j2` | SMART exporter systemd service |
| `smart-exporter.env.j2` | SMART exporter environment |
| `zfs-exporter-compose.yml.j2` | ZFS exporter Docker Compose (manual) |

---

## Service Directories

All services deployed to `/data01/services/`:

```text
/data01/services/
├── node-exporter/
│   ├── docker-compose.yml
│   ├── .env
│   └── text_files/           # Textfile collector directory
├── cadvisor/
│   ├── docker-compose.yml
│   └── .env
├── nvidia-gpu-exporter/      # GPU hosts only
│   ├── docker-compose.yml
│   └── .env
└── smart-exporter/
    ├── docker-compose.yml
    └── .env

# Note: process-exporter runs via direct docker run in systemd, no compose directory
```

---

## Exporter Details

### Node Exporter (port 9100)

Host metrics: CPU, memory, disk, network, ZFS, systemd, processes.

```bash
# Check status
systemctl status node-exporter
curl -s http://localhost:9100/metrics | head -20

# Key metrics
curl -s http://localhost:9100/metrics | grep -E "node_cpu_seconds|node_memory_MemAvailable"
```

**Features:**

- Host networking mode for accurate metrics
- Textfile collector at `/data01/services/node-exporter/text_files/`
- ZFS, thermal, ethtool, systemd collectors enabled

### cAdvisor (port 8912)

Container metrics: CPU, memory, network per container.

```bash
# Check status
systemctl status cadvisor
curl -s http://localhost:8912/metrics | head -20

# Web UI
open http://localhost:8912/containers/
```

**Features:**

- Docker-only mode (no system containers)
- 30s housekeeping interval
- Reduced metric set for performance

### Process Exporter (port 9256)

Per-process metrics: CPU, memory, I/O by process name.

```bash
# Check status (systemd docker run, not compose)
systemctl status process-exporter
curl -s http://localhost:9256/metrics | head -20

# View running container
docker ps | grep process-exporter
```

**Features:**

- Direct docker run in systemd (not Docker Compose)
- Monitors 100+ named processes
- 7-day RuntimeMaxSec with auto-restart
- Privileged mode for /proc access

### NVIDIA GPU Exporter (port 9445)

GPU metrics: utilization, memory, temperature, power.

```bash
# Check status (GPU hosts only)
systemctl status nvidia-gpu-exporter
curl -s http://localhost:9445/metrics | grep nvidia_gpu

# Key metrics
curl -s http://localhost:9445/metrics | grep -E "nvidia_gpu_utilization|nvidia_gpu_temperature"
```

**Features:**

- NVIDIA runtime required
- Bash TCP health check (no curl in container)
- Auto-detected: only deploys if `lspci | grep -i nvidia` succeeds

### SMART Exporter (port 9633)

Disk health metrics: SMART attributes, disk temperature, errors.

```bash
# Check status
systemctl status smart-exporter
curl -s http://localhost:9633/metrics | head -20

# Key metrics
curl -s http://localhost:9633/metrics | grep smartctl_device
```

**Features:**

- Privileged mode for raw disk access
- Mounts /dev, /proc, /sys

---

## Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'node-exporter'
    static_configs:
      - targets: ['ocean.home:9100']

  - job_name: 'cadvisor'
    static_configs:
      - targets: ['ocean.home:8912']

  - job_name: 'process-exporter'
    static_configs:
      - targets: ['ocean.home:9256']

  - job_name: 'nvidia-gpu-exporter'
    static_configs:
      - targets: ['ocean.home:9445']

  - job_name: 'smart-exporter'
    static_configs:
      - targets: ['ocean.home:9633']
```

---

## Troubleshooting

### Check all services

```bash
systemctl status node-exporter cadvisor process-exporter smart-exporter nvidia-gpu-exporter
```

### Container health

```bash
docker ps --format "table {{.Names}}\t{{.Status}}"
```

### GPU exporter not starting

```bash
# Verify GPU runtime
docker run --rm --gpus all nvidia/cuda:11.8-base-ubuntu20.04 nvidia-smi

# Check NVIDIA runtime configured
docker info | grep nvidia
```

### No metrics from exporter

```bash
# Test endpoint directly
curl -s http://localhost:PORT/metrics | head -20

# Check container logs
docker logs CONTAINER_NAME
```

---

## Security

All containers configured with:

- **no-new-privileges** (except smart-exporter which needs full access)
- **Resource limits**: Memory caps prevent exhaustion
- **Read-only filesystems** where possible
- **Health checks**: Auto-restart on failure
