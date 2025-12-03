# GPU Test Service

Test container for NVIDIA GPU passthrough verification.

---

## Quick Reference

| Setting | Value |
|---------|-------|
| Image | nvidia/cuda:12.2.0-base-ubuntu22.04 |
| Runtime | nvidia |
| Resources | 1 CPU, 1GB memory |

---

## Deployment

```bash
# Deploy (no vault required)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/gpu-test.yaml
```

---

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml.j2` | Container with GPU config |
| `gpu-test.service.j2` | Systemd service |

---

## GPU Configuration

### Devices Mapped

- `/dev/nvidia0`
- `/dev/nvidia-uvm`
- `/dev/nvidia-uvm-tools`
- `/dev/nvidiactl`
- `/dev/nvidia-modeset`
- `/dev/dri` (for render group)

### Environment

```yaml
NVIDIA_VISIBLE_DEVICES: all
NVIDIA_DRIVER_CAPABILITIES: all
```

### Groups

- `104` (render) - for DRI device access

---

## Test GPU Access

Run on GPU host:

```bash
# Test nvidia-smi
docker exec gpu-test nvidia-smi

# Check /dev/dri devices
docker exec gpu-test ls -la /dev/dri

# Check NVIDIA devices
docker exec gpu-test ls -la /dev/nvidia*

# Check groups (should include 104 - render)
docker exec gpu-test id

# Check environment
docker exec gpu-test env | grep NVIDIA

# Verify runtime
docker inspect gpu-test | grep -i runtime
```

---

## Expected Results

- `nvidia-smi` shows GPU info (RTX 3090)
- `/dev/dri/renderD128` exists
- `/dev/nvidia*` devices present
- Container has group 104 (render)
- Runtime is "nvidia"

---

## Cleanup

```bash
# On GPU host
systemctl stop gpu-test
systemctl disable gpu-test
rm /etc/systemd/system/gpu-test.service
rm -rf /data01/services/gpu-test
systemctl daemon-reload
```

---

## Purpose

Mirrors GPU config used by Plex and other GPU services:

- Same `runtime: nvidia`
- Same device mappings
- Same environment variables
- Same group_add for render

If this works but another service doesn't, the issue is service-specific, not GPU passthrough.
