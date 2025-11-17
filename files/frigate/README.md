# Frigate NVR

Modern AI-powered Network Video Recorder with hardware acceleration.

## Features

- **GPU Acceleration**: NVIDIA P2000 for hardware video decode
- **AI Object Detection**: Real-time person, car, pet detection
- **Modern UI**: React-based web interface
- **Recording**: Motion-based recording with 7-day retention
- **Events**: 14-day event retention
- **Live Streaming**: WebRTC for low-latency viewing
- **Birdseye View**: Multi-camera overview

## Architecture

- **Container**: Frigate stable
- **GPU**: NVIDIA runtime with P2000 hardware acceleration
- **Storage**: `/data01/services/frigate/`
- **Database**: SQLite (frigate.db)
- **Network**: Bridge mode (frigate_network)
- **Ports**:
  - 5000: Web UI
  - 8554: RTSP feeds
  - 8555: WebRTC

## Configuration

### Hardware Acceleration

Uses NVIDIA P2000 for:
- H.264 video decode
- Reduced CPU usage
- Higher camera count support

### Object Detection

Tracks: person, car, dog, cat
Filters configured for accuracy

### Recording

- **Continuous**: Motion-based, 7 days
- **Events**: 14 days retention
- **Snapshots**: Enabled with bounding boxes

## Access

- **Local**: http://frigate.home
- **Remote**: https://frigate.terrac.com

## Service Management

```bash
# Status
sudo systemctl status frigate

# Start/Stop/Restart
sudo systemctl start frigate
sudo systemctl stop frigate
sudo systemctl restart frigate

# Logs
sudo journalctl -u frigate -f
docker logs frigate -f
```

## Storage

```
/data01/services/frigate/
├── config/
│   ├── config.yml          # Frigate configuration
│   └── frigate.db          # SQLite database
├── media/
│   ├── recordings/         # Video recordings
│   └── clips/              # Event clips
└── docker-compose.yml
```

## Adding Cameras

Edit `/data01/services/frigate/config/config.yml` and replace `cameras: {}` with:

```yaml
cameras:
  front_door:
    enabled: true
    ffmpeg:
      inputs:
        - path: rtsp://username:password@192.168.1.100:554/stream
          roles:
            - detect
            - record
    detect:
      width: 1920
      height: 1080
      fps: 5
    zones:
      front_porch:
        coordinates: 100,100,300,100,300,300,100,300
    record:
      enabled: true
    snapshots:
      enabled: true
```

Restart after changes:

```bash
sudo systemctl restart frigate
```

## Performance

With NVIDIA P2000:
- 10+ cameras (1080p @ 5fps)
- Hardware decode reduces CPU load
- Faster object detection processing

## Troubleshooting

### GPU Not Detected

Check NVIDIA runtime:
```bash
docker exec frigate nvidia-smi
```

### High CPU Usage

Verify hardware acceleration:
```bash
docker logs frigate | grep hwaccel
```

### Camera Connection Issues

Test RTSP stream:
```bash
ffmpeg -i rtsp://camera-ip:554/stream -frames:v 1 test.jpg
```

## Integration

- **Home Assistant**: Native integration available
- **MQTT**: Disabled by default
- **API**: REST API at http://frigate.home/api/

## Security

- Private network (bridge mode)
- Cloudflare Access protected for remote access
- No new privileges security option
- Local camera credentials in config

## Resources

- Documentation: https://docs.frigate.video/
- GitHub: https://github.com/blakeblackshear/frigate
- Community: https://github.com/blakeblackshear/frigate/discussions
