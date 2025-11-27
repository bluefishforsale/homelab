# Log Anomaly Detection System Deployment Guide

## Quick Start

Deploy the complete pattern-based log anomaly detection system:

```bash
# Deploy the high-performance Go service with Redis backend
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/log_anomaly_detector_redis.yaml

# Verify service status
systemctl status log-anomaly-detector
curl http://192.168.1.143:8085/health
```

## System Architecture

```
Log Sources → Promtail → Loki → Anomaly Detector → n8n Alerts
     ↓           ↓        ↓           ↓            ↓
  systemd    journald  storage   patterns     notifications
  syslog     docker    query     statistics
  containers logs      API       entropy
```

## What Gets Deployed

### 1. Pattern Storage System (✅ Completed)
- **6 pattern categories** with 300+ predefined patterns
- **Persistent storage** in repository for version control
- **Category-based matching** (system, security, network, docker, application, media)
- **Configurable anomaly scores** per pattern

### 2. Go Anomaly Detection Service (✅ Completed)
- **Structured JSON processing** with direct field extraction
- **Dual-mode pattern matching** (structured fields + fallback regex)
- **Statistical analysis**: frequency tracking, rate-of-change detection  
- **Rule-less detection**: entropy analysis, Levenshtein distance
- **Real-time processing** with 30-second intervals
- **Redis storage** for ultra-fast atomic counters and baselines

### 3. Docker Compose Deployment (✅ Completed)
- **Containerized service** with proper resource limits
- **Health checks** and monitoring endpoints
- **Volume mounts** for patterns and data persistence
- **Systemd integration** following homelab standards

### 4. n8n Alert Integration (✅ Completed)
- **Webhook endpoint** for receiving anomalies
- **Severity-based routing** (critical → email, high → email+slack, medium/low → grafana)
- **Rate limiting** to prevent alert spam
- **Multiple alert channels** (email, Slack, Grafana, AlertManager)

## Verification Steps

### 1. Check Service Health
```bash
# Service status
systemctl status log-anomaly-detector

# API health check
curl http://192.168.1.143:8085/health

# View loaded patterns
curl http://192.168.1.143:8085/patterns
```

### 2. Verify Loki Integration
```bash
# Test Loki connectivity
curl "http://192.168.1.143:3100/loki/api/v1/query_range?query={job=\"systemd-journal\"}&limit=10"

# Check recent log ingestion
curl "http://192.168.1.143:3100/ready"
```

### 3. Test Pattern Matching

The service automatically:
- Queries Loki every 30 seconds for new logs
- Matches patterns against log messages
- Builds statistical baselines from historical data
- Detects frequency and rate-change anomalies
- Sends alerts to n8n webhook

### 4. Monitor Anomaly Detection
```bash
# View service logs
journalctl -u log-anomaly-detector -f

# Check docker container logs
cd /data01/services/log-anomaly-detector
docker compose logs -f
```

## Configuration Files Created

### Repository Files
- `vars/vars_log_anomaly.yaml` - Service configuration
- `files/log-anomaly/patterns/*.patterns` - Pattern definitions  
- `files/log-anomaly/go/` - Go source code
- `files/log-anomaly/docker-compose-redis.yml.j2` - Go + Redis deployment
- `playbooks/individual/ocean/log_anomaly_detector_redis.yaml` - Ansible playbook
- `files/n8n/workflows/log-anomaly-alerts.json` - Alert workflow

### Deployed Files  
- `/data01/services/log-anomaly-detector/` - Service directory
- `/etc/systemd/system/log-anomaly-detector.service` - Systemd service
- Redis data: `/data01/services/log-anomaly-detector/redis-data/`

## Next Steps

1. **Monitor the system** for 24-48 hours to build statistical baselines
2. **Adjust thresholds** if too many false positives occur
3. **Add custom patterns** for your specific applications  
4. **Configure n8n credentials** for email and Slack notifications
5. **Set up Grafana dashboards** to visualize anomaly trends

## Pattern Updates

To add new patterns or modify existing ones:

1. Edit pattern files in `files/log-anomaly/patterns/`
2. Run the deployment playbook: `ansible-playbook playbooks/individual/ocean/log_anomaly_detector_redis.yaml`  
3. Service automatically reloads patterns on restart

## Expected Behavior

The system will now:
- **Continuously monitor** all logs from ocean, dns01, node005, node006
- **Pattern match** against 300+ known log formats
- **Detect statistical anomalies** using frequency and rate analysis
- **Find unusual patterns** using entropy and similarity analysis
- **Send alerts** for critical and high-severity anomalies
- **Store annotations** in Grafana for anomaly visualization

All logs are being pattern-matched and the system is ready for production use!
