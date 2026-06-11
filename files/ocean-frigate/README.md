# Frigate NVR

AI-powered Network Video Recorder with hardware-accelerated video decoding.

---

## Quick Reference

| Setting | Value |
|---------|-------|
| Image | ghcr.io/blakeblackshear/frigate:stable |
| Web UI | 5000 |
| RTSP | 8554 |
| WebRTC | 8555 (TCP/UDP) |
| Network | frigate_network (bridge) |

---

## Deployment

```bash
# Deploy Frigate (requires vault for any camera credentials)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/services/frigate.yaml --ask-vault-pass
```

---

## Files

| File | Purpose |
|------|---------|
| `docker-compose.yml.j2` | Container configuration |
| `frigate.service.j2` | Systemd service |
| `frigate.yml.j2` | Frigate config (detectors, recording, objects) |

---

## Directory Structure

```text
/data01/services/frigate/
├── docker-compose.yml
├── config/
│   ├── config.yml          # Frigate configuration
│   └── frigate.db          # SQLite database
├── media/                  # Recordings and clips
└── database/               # Database directory
```

---

## Hardware Acceleration

### Video Decode (FFmpeg)

NVIDIA preset for H.264 hardware decode:

```yaml
ffmpeg:
  hwaccel_args: preset-nvidia-h264
```

### Object Detection

CPU-based detection (3 threads):

```yaml
detectors:
  cpu1:
    type: cpu
    num_threads: 3
```

### Container Resources

- **tmpfs cache**: 4GB at `/tmp/cache`
- **Shared memory**: 256MB
- **GPU devices**: `/dev/dri` (Intel/AMD), NVIDIA runtime

---

## Configuration Defaults

### Object Tracking

- person, car, dog, cat
- Filters for min/max area and confidence threshold

### Recording

- **Retention**: 7 days (motion-based)
- **Events**: 14 days

### Snapshots

- Enabled with timestamps and bounding boxes
- 14 days retention

### Live View

- 720p height, quality 8
- Birdseye multi-camera view enabled

---

## Access

- **Local**: http://frigate.home
- **Direct**: http://192.168.1.143:5000
- **Remote**: https://frigate.terrac.com

---

## Service Management

```bash
# Status
systemctl status frigate

# Restart
systemctl restart frigate

# Logs
journalctl -u frigate -f
docker logs frigate -f
```

---

## Adding Cameras

Edit `/data01/services/frigate/config/config.yml` and replace `cameras: {}`:

```yaml
cameras:
  front_door:
    enabled: true
    ffmpeg:
      inputs:
        - path: rtsp://user:pass@192.168.1.100:554/stream
          roles:
            - detect
            - record
    detect:
      width: 1920
      height: 1080
      fps: 5
    record:
      enabled: true
    snapshots:
      enabled: true
```

Restart after changes:

```bash
systemctl restart frigate
```

---

## Troubleshooting

### GPU Not Detected

```bash
# Check NVIDIA runtime
docker exec frigate nvidia-smi

# Check device access
ls -la /dev/dri
```

### High CPU Usage

```bash
# Verify hardware acceleration
docker logs frigate | grep -i hwaccel

# Check detector type (should be cpu or gpu)
docker logs frigate | grep -i detector
```

### Camera Connection Issues

```bash
# Test RTSP stream
ffmpeg -i rtsp://camera-ip:554/stream -frames:v 1 test.jpg

# Check Frigate logs for camera errors
docker logs frigate | grep -i error
```

### Health Check

```bash
# API stats
curl http://localhost:5000/api/stats

# Container health
docker inspect frigate --format='{{.State.Health.Status}}'
```

---

## Integration

- **Home Assistant**: Native integration
- **MQTT**: Disabled by default
- **API**: http://localhost:5000/api/

---

## Security

- **no-new-privileges**: Enabled
- **Network**: Dedicated bridge (frigate_network)
- **Privileged mode**: Required for hardware access
- **Remote access**: Cloudflare Access protected

---

## Resources

- Docs: <https://docs.frigate.video/>
- GitHub: <https://github.com/blakeblackshear/frigate>
