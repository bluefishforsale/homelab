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

## Monitoring

Each registry instance exposes `/v2/` as a health endpoint. A Prometheus HTTP probe or simple cron curl confirms each cache is alive. No dedicated exporter needed initially.

## Out of Scope

- TLS termination on the cache (internal network, HTTP is acceptable)
- Image signing or vulnerability scanning
- Multi-site replication
- Automatic failover for non-Hub registries (manual `registry_cache_enabled` toggle is sufficient)
