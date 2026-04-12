# Local Docker Registry Cache — Design Spec

## Problem

All 25+ Docker services in the homelab pull images directly from public registries (Docker Hub, ghcr.io, registry.gitlab.com, nvcr.io). If any upstream registry goes down, services cannot be restarted or redeployed. We need a local pull-through cache so that previously-pulled images remain available during upstream outages.

## Solution

Deploy the CNCF Distribution registry (`registry:2`) in pull-through cache mode on a dedicated VM on node005. One container per upstream registry. Docker Hub gets transparent caching via `registry-mirrors` in the Docker daemon config. Non-Hub registries are routed through the cache via Ansible variable prefixes in compose templates.

## Architecture

### VM

- **Host:** node005
- **OS:** Debian 12 (standard homelab base)
- **Resources:** 2 vCPU, 2GB RAM, 200GB disk
- **Storage path:** `/data01/services/registry-cache/`
- **DNS:** `registry-cache.internal` A record in PowerDNS pointing to VM IP

### Cache Instances

| Name | Upstream | Port | Auth |
|------|----------|------|------|
| docker-hub-cache | `https://registry-1.docker.io` | 5001 | Docker Hub credentials from vault |
| ghcr-cache | `https://ghcr.io` | 5002 | None |
| gitlab-cache | `https://registry.gitlab.com` | 5003 | None |
| nvcr-cache | `https://nvcr.io` | 5004 | None |

All four run in a single docker-compose stack managed by one systemd unit.

### Storage Layout

```
/data01/services/registry-cache/
  config/
    docker-hub.yml        # registry config per upstream
    ghcr.yml
    gitlab.yml
    nvcr.yml
  data/
    docker-hub/           # cached layers/manifests
    ghcr/
    gitlab/
    nvcr/
```

### Registry Container Config

Each container gets a config file:

```yaml
version: 0.1
proxy:
  remoteurl: <upstream-url>
  # username/password only for docker hub
storage:
  filesystem:
    rootdirectory: /var/lib/registry
  maintenance:
    uploadpurging:
      enabled: true
      age: 168h
      interval: 24h
  delete:
    enabled: true
http:
  addr: :5000
  headers:
    X-Content-Type-Options: [nosniff]
health:
  storagedriver:
    enabled: true
    interval: 30s
    threshold: 3
```

## Docker Daemon Changes

### daemon.json.j2

Add `registry-mirrors` to the existing template:

```json
{
  "registry-mirrors": ["http://registry-cache.internal:5001"]
}
```

This makes Docker Hub pulls transparent on all hosts — Docker tries the mirror first, falls back to Docker Hub directly if the mirror is unreachable.

## Ansible Template Changes

### Group Vars

New variables in group_vars:

```yaml
registry_cache_enabled: true
registry_cache_host: "registry-cache.internal"
cached_registries:
  ghcr.io: "{{ registry_cache_host ~ ':5002' if registry_cache_enabled else 'ghcr.io' }}"
  registry.gitlab.com: "{{ registry_cache_host ~ ':5003' if registry_cache_enabled else 'registry.gitlab.com' }}"
  nvcr.io: "{{ registry_cache_host ~ ':5004' if registry_cache_enabled else 'nvcr.io' }}"
```

The `registry_cache_enabled` flag allows reverting to direct pulls by flipping one variable.

### Compose Template Updates

Services using Docker Hub images require no changes — `registry-mirrors` handles them transparently.

Services using non-Hub registries need their image variable updated:

| Service | Current Image Prefix | New Image Prefix |
|---------|---------------------|------------------|
| homepage | `ghcr.io/gethomepage/homepage` | `{{ cached_registries['ghcr.io'] }}/gethomepage/homepage` |
| fail2ban-exporter | `registry.gitlab.com/hctrdev/...` | `{{ cached_registries['registry.gitlab.com'] }}/hctrdev/...` |
| nvidia/dcgm | `nvcr.io/nvidia/...` | `{{ cached_registries['nvcr.io'] }}/nvidia/...` |
| dns-stack (kea-exporter) | `ghcr.io/mweinelt/kea-exporter` | `{{ cached_registries['ghcr.io'] }}/mweinelt/kea-exporter` |
| dns-stack (netbootxyz) | `ghcr.io/netbootxyz/netbootxyz` | `{{ cached_registries['ghcr.io'] }}/netbootxyz/netbootxyz` |

Any additional non-Hub image references discovered during implementation will follow the same pattern.

## File Structure

```
playbooks/individual/infrastructure/registry_cache.yaml
files/registry-cache/
  docker-compose.yml.j2
  registry-cache.service.j2
  promtail-config.yml.j2
  grafana-registry-cache-dashboard.json
  config/
    docker-hub.yml.j2
    ghcr.yml.j2
    gitlab.yml.j2
    nvcr.yml.j2
```

## Deployment Order

1. Provision VM on node005 — Debian 12, install Docker (reuse existing `deb12_docker.yaml` or `docker_ce.yaml`)
2. Deploy registry-cache playbook — create directories, template configs, start compose stack
3. Add `registry-cache.internal` DNS A record in PowerDNS
4. Update `daemon.json.j2` with `registry-mirrors`, redeploy Docker config to ocean and other hosts
5. Update non-Hub image references in affected compose templates
6. Run cache-warming playbook to pre-populate the cache
7. Verify: pull an image on ocean, confirm it routes through the cache

## Maintenance

### Garbage Collection

Weekly systemd timer runs GC on each container:

```bash
docker exec <container> registry garbage-collect /etc/docker/registry/config.yml
```

Reclaims space from unreferenced layers.

### Cache Warming

A playbook that iterates over all image references from compose templates and pulls them through the cache. Run after initial deployment and optionally on a weekly schedule to keep the cache fresh for outage resilience.

## Failure Modes

| Scenario | Behavior |
|----------|----------|
| Upstream down, image cached | Pull succeeds from local cache |
| Upstream down, image NOT cached | Pull fails (unavoidable — mitigated by cache warming) |
| Cache VM down | Docker Hub: automatic fallback via `registry-mirrors`. Non-Hub: pull fails; flip `registry_cache_enabled` to false and redeploy to restore direct pulls |
| Disk full | GC reclaims unreferenced layers; increase disk or run GC more aggressively |
| Cache staleness | Pull-through checks upstream manifest on every pull; cached layers served only if tag unchanged or upstream unreachable |

## Observability

### Metrics (Prometheus)

The Distribution registry natively exposes Prometheus metrics at `/metrics` when enabled in the config. Add to each registry config:

```yaml
http:
  debug:
    addr: :5001  # debug/metrics port (internal to container)
    prometheus:
      enabled: true
      path: /metrics
```

Since all four containers share a compose stack, expose a metrics port per container (5011-5014) and add a single Prometheus scrape job:

```yaml
- job_name: 'registry-cache'
  static_configs:
    - targets:
        - registry-cache.internal:5011
        - registry-cache.internal:5012
        - registry-cache.internal:5013
        - registry-cache.internal:5014
      labels:
        __metrics_path__: /metrics
  relabel_configs:
    - source_labels: [__address__]
      regex: '.*:5011'
      target_label: upstream
      replacement: docker-hub
    - source_labels: [__address__]
      regex: '.*:5012'
      target_label: upstream
      replacement: ghcr
    - source_labels: [__address__]
      regex: '.*:5013'
      target_label: upstream
      replacement: gitlab
    - source_labels: [__address__]
      regex: '.*:5014'
      target_label: upstream
      replacement: nvcr
```

Key metrics exposed by Distribution registry:
- `registry_proxy_hits_total` / `registry_proxy_misses_total` — cache hit/miss rates
- `registry_storage_blob_upload_total` — blobs cached from upstream
- `registry_http_requests_total` — request counts by method/status
- `registry_http_request_duration_seconds` — request latency
- `registry_storage_cache_total` — storage cache operations

This job gets added to `files/ocean-prometheus/prometheus.yml.j2` following the existing pattern.

### Logging (Loki)

The registry-cache systemd service uses journald logging (standard for all homelab services). Promtail on the registry-cache VM scrapes journald and ships logs to Loki. The Promtail config follows the same pattern as other hosts — scrape `systemd-journal` with labels for host and unit.

Registry containers log to journald via the Docker journald log driver (already the default in `daemon.json`). Logs include pull requests, cache hits/misses, upstream connectivity errors, and GC activity.

Add a Promtail sidecar container to the registry-cache compose stack (same pattern as `files/dns-stack/docker-compose.yml.j2` which already includes Promtail) or install Promtail as a systemd service on the VM.

### Grafana Dashboard

A dedicated Grafana dashboard (`files/registry-cache/grafana-registry-cache-dashboard.json`) with:

**Cache Performance:**
- Cache hit rate (%) per upstream — `rate(registry_proxy_hits_total) / (rate(registry_proxy_hits_total) + rate(registry_proxy_misses_total))`
- Cache hits vs misses over time (stacked area chart per upstream)
- Total pulls served from cache vs upstream

**Health & Availability:**
- Upstream reachability status per registry (up/down indicator)
- HTTP request rate and error rate (4xx/5xx) per upstream
- Request latency p50/p95/p99

**Storage:**
- Disk usage per upstream cache (blobs uploaded over time)
- GC runs and space reclaimed (from logs)

**Logs Panel:**
- Recent pull activity from Loki (filterable by upstream, image name)
- Error log stream (upstream connectivity failures, GC errors)

The dashboard JSON is provisioned via the existing Grafana dashboard deployment pattern.

## Out of Scope

- TLS termination on the cache (internal network, HTTP is acceptable)
- Image signing or vulnerability scanning
- Multi-site replication
- Automatic failover for non-Hub registries (manual `registry_cache_enabled` toggle is sufficient)
- Alerting rules (can be added later once baseline metrics are established)
