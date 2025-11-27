# Homelab Loki & Promtail Dashboard

A custom Grafana dashboard designed for bare metal homelab deployments of Loki and Promtail.

## Dashboard Overview

This dashboard monitors your log pipeline with the following panels:

### Log Pipeline Overview
- **Log Entries Sent (Total)** - Total entries sent by each Promtail instance
- **Log Entry Rate** - Real-time ingestion rate per second
- **Dropped Entries Rate** - Failed entries that couldn't be sent to Loki
- **Journal Lines Read** - systemd journal lines processed by Promtail

### Loki Internal Metrics
- **Loki Internal Log Messages Rate** - Loki's own logs by severity level
- **Request Duration Percentiles** - 50th/99th percentile latency for Promtail → Loki requests

### Syslog Collection (Ocean Only)
- **Syslog Collection** - Entries received via syslog listener on port 1514

## Installation

1. **Access Grafana**: Go to `http://grafana.home`
2. **Import Dashboard**:
   - Click **"+" → Import**
   - Click **"Upload JSON file"**
   - Select `homelab-loki-promtail-dashboard.json`
   - Click **"Load"**
3. **Configure Datasource**: Ensure Prometheus datasource is set to your Prometheus instance

## Requirements

- **Prometheus** collecting metrics from Promtail instances
- **Loki** running and exposing metrics
- **Promtail** agents on all hosts configured with metrics enabled

## Metrics Used

Based on your actual homelab metrics:

```promql
# Core Promtail metrics
promtail_sent_entries_total
promtail_dropped_entries_total  
promtail_journal_target_lines_total
promtail_request_duration_seconds_bucket

# Loki internal metrics
loki_internal_log_messages_total

# Syslog specific (ocean only)
promtail_syslog_target_entries_total
promtail_syslog_target_parsing_errors_total
```

## Dashboard Features

- **Auto-refresh**: 30 second refresh interval
- **Time range**: Default 1 hour view
- **Instance filtering**: Shows metrics per Promtail instance
- **Color coding**: 
  - 🟢 Green: Normal operations
  - 🟡 Yellow: Warnings  
  - 🔴 Red: Errors/high latency
- **Thresholds**: Pre-configured alert thresholds for key metrics

## Troubleshooting

**No data showing:**
1. Verify Prometheus is scraping Promtail targets: `promtail` job
2. Check datasource configuration points to correct Prometheus
3. Ensure Promtail metrics are enabled (they should be by default)

**Missing panels:**
- Some panels may show "No data" if features aren't being used (e.g., syslog on non-ocean hosts)

## Customization

Feel free to modify:
- Time ranges and refresh intervals
- Add additional panels for specific metrics from your `/metrics` output
- Adjust thresholds based on your environment
- Add alerting rules for critical metrics
