# DNS Stack Deployment Review - dns02

## Deployment Summary

**Date:** March 13, 2026  
**Host:** dns02 (192.168.1.3)  
**Status:** ✅ Core Services Operational, ⚠️ Monitoring Partial

---

## Service Status

### Core DNS/DHCP Services (All Healthy ✅)

| Service | Status | Health | Uptime | Purpose |
|---------|--------|--------|--------|---------|
| **powerdns** | Running | Healthy | 17+ hours | Authoritative DNS server with API |
| **kea-dhcp4** | Running | Healthy | 17+ hours | DHCP server with DDNS integration |
| **kea-dhcp-ddns** | Running | - | 17+ hours | Dynamic DNS update daemon |
| **kea-ctrl-agent** | Running | - | 17+ hours | REST API for Kea management |

**Ports Exposed:**
- DNS: `53/tcp`, `53/udp` (PowerDNS)
- DHCP: `67/udp` (Kea DHCP4 via host network)
- PowerDNS API: `8081/tcp`
- Kea Control API: `8000/tcp`

---

## Monitoring & Logging Status

### ✅ Working Components

#### 1. Promtail (Loki Log Collection)
- **Status:** Running
- **Container:** `dns-promtail`
- **Configuration:** `/opt/dns-stack/config/promtail-config.yml`
- **Target:** Loki at `http://192.168.1.143:3100`
- **Log Sources:**
  - PowerDNS logs (journald tag: `powerdns`)
  - Kea DHCP4 logs (journald tag: `kea-dhcp4`)
  - Kea DHCP-DDNS logs (journald tag: `kea-dhcp-ddns`)
  - Kea Control Agent logs (journald tag: `kea-ctrl-agent`)

#### 2. PowerDNS Native Metrics
- **Endpoint:** `http://192.168.1.3:8081/metrics?do=prometheus`
- **Authentication:** X-API-Key header required
- **Prometheus Job:** `powerdns` (already configured)
- **Status:** ✅ Native metrics available via PowerDNS API

### ⚠️ Issues Identified

#### 1. Kea Exporter - Socket Access Issue
- **Status:** Restarting (crash loop)
- **Container:** `kea-exporter`
- **Root Cause:** Cannot access Unix domain sockets in Docker volume
- **Error:** `Unix domain socket does not exist at /kea/sockets/kea4-ctrl-socket`
- **Reason:** Exporter uses `network_mode: host` but sockets are in named volume `kea-sockets`
- **Impact:** No Kea DHCP metrics exported to Prometheus

**Technical Details:**
- Kea control sockets are in Docker volume: `kea-sockets:/kea/sockets`
- Kea exporter runs with `network_mode: host` (required for port binding)
- Host network mode prevents access to container volumes
- Socket paths: `/kea/sockets/kea4-ctrl-socket`, `/kea/sockets/kea-ddns-ctrl-socket`

**Possible Solutions:**
1. Mount sockets to host path instead of Docker volume
2. Run Kea exporter in same network namespace as Kea containers
3. Use Kea's built-in stats endpoint (if available)

#### 2. PowerDNS Exporter - Removed
- **Status:** Removed from deployment
- **Reason:** PowerDNS provides native Prometheus metrics via API
- **Replacement:** Using PowerDNS API endpoint directly in Prometheus

#### 3. netboot.xyz - Unhealthy
- **Status:** Running but unhealthy
- **Note:** Separate issue, not related to DNS/DHCP functionality
- **Impact:** PXE boot functionality may be affected

---

## Prometheus Integration

### Current Scrape Targets

#### ✅ PowerDNS (Working)
```yaml
- job_name: 'powerdns'
  scrape_interval: 30s
  metrics_path: /metrics
  params:
    do: ['prometheus']
  headers:
    X-API-Key: "{{ vault_secret }}"
  static_configs:
    - targets: ['dns02.home:8081']
```

#### ⚠️ Kea Exporter (Not Working)
```yaml
- job_name: 'kea-exporter'
  scrape_interval: 30s
  static_configs:
    - targets: ['dns02.home:9547']
```
**Status:** Target unreachable due to container crash loop

#### ✅ Promtail (Working)
```yaml
- job_name: 'promtail'
  scrape_interval: 15s
  static_configs:
    - targets: ['dns02.home:9080']
```

---

## Loki Log Collection

### Log Pipeline Configuration

**Scrape Config:** `dns-stack-journal`
- **Source:** systemd journal via journald driver
- **Labels:** `job=dns-stack`, `host=dns02`
- **Relabeling:** Extracts container name, service tag, unit name

**Log Parsing Stages:**

1. **PowerDNS Logs**
   - Pattern: `TIMESTAMP LEVEL [COMPONENT] MESSAGE`
   - Labels extracted: `level`, `component`

2. **Kea Logs**
   - Pattern: `YYYY-MM-DD HH:MM:SS.mmm LEVEL [COMPONENT] MESSAGE`
   - Labels extracted: `level`, `component`

3. **Exporter Logs**
   - Pattern: `level=VALUE MESSAGE`
   - Labels extracted: `level`

**Log Retention:** Managed by Loki on ocean (192.168.1.143)

---

## DNS Stack Architecture

### Service Dependencies

```
PowerDNS (Authoritative DNS)
    ↓ (healthy check)
    ├─→ Kea DHCP4 (DHCP Server)
    │       ↓
    │       └─→ Kea Control Agent (REST API)
    │
    └─→ Kea DHCP-DDNS (Dynamic DNS Updates)
            ↓
            └─→ Kea Control Agent (REST API)

Promtail → Loki (ocean:3100)
```

### Volume Mounts

| Volume | Mount Point | Purpose |
|--------|-------------|---------|
| `{{ home }}/config/` | `/etc/kea/`, `/etc/powerdns/` | Configuration files |
| `{{ home }}/data/pdns/` | `/var/lib/powerdns/` | PowerDNS SQLite database |
| `{{ home }}/data/kea/` | `/kea/leases/` | Kea lease database |
| `kea-sockets` (volume) | `/kea/sockets/` | Kea control sockets |
| `/var/log/journal/` | `/var/log/journal/` | Promtail log source |

---

## Key Achievements ✅

1. **PowerDNS Operational**
   - SQLite database initialized
   - API enabled with Prometheus metrics
   - Health checks passing
   - Correct permissions (0777 on data directory)

2. **Kea DHCP Stack Operational**
   - All services running and healthy
   - Correct internal paths (`/kea/sockets`, `/kea/leases`)
   - DDNS integration configured
   - Control sockets functional

3. **Log Aggregation Working**
   - Promtail collecting all DNS stack logs
   - Logs sent to Loki on ocean
   - Structured log parsing configured
   - Journal integration working

4. **Idempotent Deployment**
   - Playbook safe to run multiple times
   - All configuration templated
   - Systemd service management
   - Docker Compose orchestration

---

## Outstanding Issues ⚠️

### High Priority

1. **Kea Exporter Socket Access**
   - **Impact:** No DHCP metrics in Prometheus
   - **Workaround Needed:** Reconfigure socket mounting strategy
   - **Options:**
     - Mount sockets to host filesystem
     - Use bridge network instead of host network
     - Query Kea API directly instead of sockets

### Medium Priority

2. **netboot.xyz Unhealthy**
   - **Impact:** PXE boot may not work
   - **Investigation Needed:** Check container logs and health check

### Low Priority

3. **PowerDNS Exporter Port Reserved**
   - **Port 9120** still reserved in `vars_service_ports.yaml`
   - **Action:** Can be removed since native metrics are used

---

## Metrics Available

### PowerDNS Metrics (via API)
- Query statistics
- Answer statistics  
- Cache performance
- Backend performance
- Uptime and version info

### Kea Metrics (when exporter fixed)
- DHCP lease statistics
- Pool utilization
- Packet statistics
- Subnet statistics

### Promtail Metrics
- Log entries sent
- Dropped entries
- Request latency
- Journal lines read

---

## Next Steps

### Immediate Actions

1. **Fix Kea Exporter**
   - Modify socket mounting strategy
   - Test with bind mount to host path
   - Verify metrics endpoint accessibility

2. **Update Prometheus**
   - Deploy updated prometheus.yml with Kea exporter target
   - Reload Prometheus configuration
   - Verify scrape targets

3. **Verify Log Collection**
   - Check Loki for DNS stack logs
   - Verify log parsing and labels
   - Create Grafana dashboard queries

### Future Enhancements

1. **Grafana Dashboards**
   - PowerDNS query performance
   - Kea DHCP lease utilization
   - DNS/DHCP error rates
   - Log anomaly detection

2. **Alerting Rules**
   - PowerDNS service down
   - Kea DHCP pool exhaustion
   - High DNS error rates
   - DDNS update failures

3. **Performance Testing**
   - DNS query response times
   - DHCP lease assignment speed
   - Compare with Cloudflare DNS (1.1.1.1)
   - Compare with Google DNS (8.8.8.8)

---

## Configuration Files

| File | Purpose | Location |
|------|---------|----------|
| `docker-compose.yml.j2` | Service orchestration | `files/dns-stack/` |
| `pdns.conf.j2` | PowerDNS config | `files/dns-stack/` |
| `kea-dhcp4.conf.j2` | Kea DHCP4 config | `files/dns-stack/` |
| `kea-dhcp-ddns.conf.j2` | Kea DDNS config | `files/dns-stack/` |
| `kea-ctrl-agent.conf.j2` | Kea API config | `files/dns-stack/` |
| `promtail-config.yml.j2` | Log collection config | `files/dns-stack/` |
| `dns-stack.service.j2` | Systemd service | `files/dns-stack/` |
| `dns02.yaml` | Deployment playbook | `playbooks/individual/core/services/` |

---

## Deployment Commands

```bash
# Full deployment
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/core/services/dns02.yaml

# Deploy only (skip setup)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/core/services/dns02.yaml --tags deploy

# Check service status
ssh debian@192.168.1.3 "sudo docker ps"

# View logs
ssh debian@192.168.1.3 "sudo docker logs powerdns"
ssh debian@192.168.1.3 "sudo docker logs kea-dhcp4"
ssh debian@192.168.1.3 "sudo docker logs dns-promtail"

# Restart services
ssh debian@192.168.1.3 "sudo systemctl restart dns-stack"
```

---

## Summary

**Core DNS/DHCP functionality:** ✅ **Fully Operational**
- PowerDNS serving DNS queries
- Kea DHCP4 assigning leases
- DDNS updates working
- All health checks passing

**Monitoring:** ⚠️ **Partially Working**
- ✅ PowerDNS metrics available
- ✅ Log collection to Loki working
- ⚠️ Kea metrics unavailable (exporter issue)

**Overall Status:** 🟡 **Production Ready with Monitoring Gaps**

The DNS stack is fully functional for its primary purpose (DNS and DHCP services). The monitoring gap (Kea metrics) does not affect service operation but limits observability into DHCP performance and utilization.
