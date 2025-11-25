# GPU Test Service

Simple Docker Compose service to test NVIDIA GPU passthrough using the same configuration as Plex.

## Purpose

Verifies that:

- NVIDIA runtime is properly configured
- GPU devices are accessible in containers
- `/dev/dri` devices are available
- Render group permissions work correctly

## Deploy

```bash
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/gpu-test.yaml
```

## Test GPU Access

```bash
# Test nvidia-smi
ssh terrac@192.168.1.143 "docker exec gpu-test nvidia-smi"

# Check /dev/dri devices
ssh terrac@192.168.1.143 "docker exec gpu-test ls -la /dev/dri"

# Check NVIDIA devices
ssh terrac@192.168.1.143 "docker exec gpu-test ls -la /dev/nvidia*"

# Check groups (should include group 104 - render)
ssh terrac@192.168.1.143 "docker exec gpu-test id"

# Check environment variables
ssh terrac@192.168.1.143 "docker exec gpu-test env | grep NVIDIA"

# Verify runtime
ssh terrac@192.168.1.143 "docker inspect gpu-test | grep -i runtime"
```

## Expected Results

- `nvidia-smi` should show GPU information
- `/dev/dri/renderD128` should exist with proper permissions
- `/dev/nvidia*` devices should be present
- Container should have group 104 (render)
- Runtime should be "nvidia"

## Cleanup

```bash
ssh terrac@192.168.1.143 "sudo systemctl stop gpu-test.service && sudo systemctl disable gpu-test.service"
ssh terrac@192.168.1.143 "sudo rm /etc/systemd/system/gpu-test.service"
ssh terrac@192.168.1.143 "sudo rm -rf /data01/services/gpu-test"
```

## Configuration Mirror

This service uses the exact same GPU configuration as Plex:

- Same `runtime: nvidia`
- Same device mappings
- Same environment variables
- Same group_add for render group

If this works but Plex doesn't, the issue is Plex-specific configuration, not GPU passthrough.
