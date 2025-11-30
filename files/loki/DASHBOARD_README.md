# Loki Dashboard

Grafana dashboard for Loki and Promtail monitoring.

---

## Quick Reference

| Component | Host | Port |
|-----------|------|------|
| Loki | ocean (192.168.1.143) | 3100 |
| Promtail | all hosts | 9080 |
| Grafana | ocean | 8910 |

---

## Installation

1. Access Grafana at `http://192.168.1.143:8910`
2. Go to Dashboards → Import
3. Upload `homelab-loki-promtail-dashboard.json`
4. Select Prometheus datasource

---

## Panels

| Panel | Metric |
|-------|--------|
| Log Entries Sent | `promtail_sent_entries_total` |
| Log Entry Rate | `rate(promtail_sent_entries_total[1m])` |
| Dropped Entries | `promtail_dropped_entries_total` |
| Journal Lines | `promtail_journal_target_lines_total` |
| Loki Messages | `loki_internal_log_messages_total` |
| Request Latency | `promtail_request_duration_seconds_bucket` |

---

## Deploy Loki

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/loki.yaml --ask-vault-pass
```

---

## Troubleshooting

| Issue | Check |
|-------|-------|
| No data | Prometheus scraping Promtail targets |
| Missing panels | Feature not used on host |
| High latency | Loki disk I/O, retention settings |

---

## Related Documentation

- [playbooks/individual/ocean/loki.yaml](/playbooks/individual/ocean/loki.yaml)
- [playbooks/individual/base/logging.yaml](/playbooks/individual/base/logging.yaml)
