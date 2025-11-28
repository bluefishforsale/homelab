# Log Anomaly Detection Service

A high-performance Go-based log anomaly detection system that monitors your homelab infrastructure for unusual patterns and behaviors.

## Architecture

The service implements a **Tier 1: Pattern-based** anomaly detection approach with:

- **Structured JSON processing** with direct field extraction (no regex on JSON)
- **Dual-mode pattern matching** (structured fields + fallback regex)
- **Statistical analysis** with frequency and rate-of-change detection
- **Rule-less detection** using entropy analysis and Levenshtein distance
- **Real-time processing** with low latency (30-second intervals)
- **Redis backend** for ultra-fast counter operations and statistics

## Features

### Pattern-Based Detection

- **6 pattern categories**: system, security, network, docker, application, media
- **300+ predefined patterns** covering common log formats and error conditions
- **Configurable anomaly scores** per pattern type
- **Dynamic category selection** based on log labels and metadata

### Statistical Analysis

- **Frequency anomaly detection**: Uses z-score analysis to detect unusual pattern frequencies
- **Rate-of-change detection**: Identifies sudden spikes or drops in log pattern rates
- **Baseline calculation**: Automatically builds statistical baselines from historical data
- **Redis storage**: Ultra-fast atomic counters and statistical baseline storage

### Rule-less Detection

- **Entropy analysis**: Detects logs with unusually high information content
- **Levenshtein distance**: Identifies repeated similar messages that might indicate loops
- **Clustering analysis**: Groups similar log patterns to detect outliers

### Real-time Processing

- **Loki integration**: Queries Loki for recent log entries every 30 seconds
- **Batch processing**: Processes up to 1000 log entries per batch
- **Memory-efficient**: Optimized Go routines and efficient data structures
- **Concurrent processing**: Non-blocking analysis with native Go HTTP server

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LOKI_URL` | `http://192.168.1.143:3100` | Loki API endpoint |
| `CHECK_INTERVAL` | `30` | Seconds between anomaly checks |
| `BATCH_SIZE` | `1000` | Max logs processed per batch |
| `FREQUENCY_SIGMA` | `3.0` | Z-score threshold for frequency anomalies |
| `RATE_CHANGE_THRESHOLD` | `5.0` | Rate change multiplier threshold |
| `ENTROPY_THRESHOLD` | `4.5` | Entropy score threshold |
| `LEVENSHTEIN_THRESHOLD` | `0.7` | String similarity threshold (0-1) |

### Pattern Files

Pattern files are stored in `/patterns/` and follow this format:

```text
PATTERN_NAME REGEX_PATTERN [anomaly_score] [description]
```

Example:

```text
KERNEL_ERROR (?i)kernel:.*error|panic|oops 5.0 Critical kernel errors
HTTP_SUCCESS (?i)http.*["\s]200["\s] 0.1 HTTP success response
```

### Anomaly Scores

- **0.0-1.0**: Normal operation, informational
- **1.0-2.0**: Minor issues, warnings  
- **2.0-3.5**: Moderate problems, errors
- **3.5-5.0**: Serious issues, failures
- **5.0+**: Critical system problems

### Severity Levels

- **low**: Score 0-2.0
- **medium**: Score 2.0-3.5  
- **high**: Score 3.5-5.0
- **critical**: Score 5.0+

## API Endpoints

### Health Check
```bash
curl http://192.168.1.143:8085/health
```

### Service Status
```bash
curl http://192.168.1.143:8085/status
```

### View Loaded Patterns
```bash
curl http://192.168.1.143:8085/patterns
```

## Deployment

### Using Ansible (Go + Redis Version)
```bash
# Deploy the high-performance Go service with Redis backend
ansible-playbook playbooks/individual/ocean/log_anomaly_detector_redis.yaml

# Check service status
systemctl status log-anomaly-detector
```

### Manual Docker Compose
```bash
cd /data01/services/log-anomaly-detector
docker compose up -d
```

## Monitoring and Maintenance

### Log Files
- Service logs: `journalctl -u log-anomaly-detector -f`
- Container logs: `docker compose logs -f log-anomaly-detector`

### Database Maintenance
The SQLite database automatically:
- Retains 7 days of statistical baselines
- Purges old pattern count data
- Rebuilds baselines hourly

### Pattern Updates
Pattern files are version-controlled in the repository:
1. Edit patterns in `files/log-anomaly/patterns/`
2. Deploy with Ansible playbook
3. Service automatically reloads patterns

## Troubleshooting

### Service Won't Start
```bash
# Check systemd status
systemctl status log-anomaly-detector

# Check docker compose logs
cd /data01/services/log-anomaly-detector
docker compose logs

# Verify Loki connectivity
curl http://192.168.1.143:3100/ready
```

### No Anomalies Detected
1. Check Loki has recent logs: `curl "http://192.168.1.143:3100/loki/api/v1/query_range?query={job=~\"systemd-journal|syslog\"}"`
2. Verify webhook URL is reachable
3. Lower statistical thresholds in environment variables
4. Check pattern matching with `/patterns` endpoint

### High Memory Usage
1. Reduce `BATCH_SIZE` environment variable
2. Increase `CHECK_INTERVAL` to process less frequently
3. Clear SQLite database: `rm /data01/services/log-anomaly-detector/data/anomaly_stats.db`

## Performance

### Resource Usage
- **CPU**: 0.5-1.0 cores during analysis
- **Memory**: 256-512 MB baseline, up to 1 GB during processing
- **Storage**: ~100 MB for patterns and baselines
- **Network**: Minimal (Loki queries only)

### Scaling Considerations
- Service is designed for single-instance deployment
- Can handle 10,000+ log entries per minute
- SQLite database scales to millions of pattern records
- Consider Redis for multi-instance deployments

## Security

- Runs as non-root user (media:1001)
- No privileged access required
- Read-only access to pattern files
- Database stored on persistent volume
