# Registry cache (registry-cache-01)

A pull-through image cache on its own VM, so an image the fleet has pulled once
can still be pulled while its upstream registry is down. Four `registry:2`
containers each proxy one upstream (Docker Hub, ghcr.io, registry.gitlab.com,
nvcr.io). Only the Docker Hub one is in any pull path today: dockerd uses it as
a `registry-mirrors` entry, and nothing points at the other three.

The host is `registry-cache-01` (192.168.1.30, node005), in both `[vms]` and
`[registry_cache]` in `inventories/production/hosts.ini`. DNS is
`registry-cache.home` and `registry-cache-01.home`, both A records to
192.168.1.30 in `roles/dns_infrastructure/templates/powerdns/import-zones.sh.j2`.

## Playbooks and workflows

| File | Host | What it owns |
|---|---|---|
| `infrastructure/create_registry_cache_vm.yaml` | node005 | VM 7000 `registry-cache-01`, cloned from template 9999: 2 cores, 2048 MB, scsi0 grown `+192G`, onboot, static IP (dig of `registry-cache-01.home`, else 192.168.1.30), single nameserver 192.168.1.2. CPU, memory and network are only set when the VM is not running. |
| `infrastructure/registry_cache.yaml` | `registry_cache` | Installs Docker inline (not via `docker_ce.yaml`), renders `daemon.json`, the four registry configs, compose file, promtail config and `registry-cache.service`, plus the cache-size metric script and timer. State under `/data01/services/registry-cache/{config,data/<upstream>}`. |
| `.github/workflows/deploy-registry-cache.yml` | runner | `workflow_dispatch` or `repository_dispatch: deploy-registry-cache`; runs `registry_cache.yaml`. Own concurrency group `deploy-registry-cache`, not `deploy-ocean`. |
| `infrastructure/docker_ce.yaml` | `all:!github_runners`, `serial: 1` | Renders `files/docker/daemon.json.j2` on every other docker host, which is where the mirror line comes from. |

`/data01` on this VM is a plain local directory. The host is not in
`[data01_workers]` and mounts nothing from ocean.

## Upstreams and ports

| Container | Upstream | Registry port | Metrics port | Auth |
|---|---|---|---|---|
| `docker-hub-cache` | `https://registry-1.docker.io` | 5001 | 5011 | vault `development.docker_registry.{username,password}` |
| `ghcr-cache` | `https://ghcr.io` | 5002 | 5012 | none |
| `gitlab-cache` | `https://registry.gitlab.com` | 5003 | 5013 | none |
| `nvcr-cache` | `https://nvcr.io` | 5004 | 5014 | none |

Host ports map to container `:5000` (registry) and `:5001` (debug/metrics) in
`files/registry-cache/docker-compose.yml.j2`. Each container is capped at 0.5
CPU and 512M. None of these ports are in `vars/vars_service_ports.yaml`; they
are literals in the compose file, `files/docker/daemon.json.j2` and
`files/ocean-prometheus/prometheus.yml.j2`.

Registry configs (`files/registry-cache/config/*.yml.j2`) are identical except
upstream and Hub credentials: upload purging at 168h every 24h, `delete`
enabled, storage-driver health check every 30s.

## The mirror

`files/docker/daemon.json.j2` adds
`"registry-mirrors": ["http://registry-cache.home:5001"]` unless the host sets
`registry_cache_mirror` false. Only `registry-cache-01` sets it false
(`hosts.ini`), so the cache never mirrors through itself.

NOTE: a change to `files/docker/daemon.json.j2` renders on every host
`docker_ce.yaml` targets and restarts dockerd one host at a time
(`serial: 1`). The play's own comment says that restart "bounces every
container on the host", so a mirror edit is a rolling restart of the fleet.

Hosts that do not get the mirror:

- `gh-runner-01`, excluded from `docker_ce.yaml` because restarting dockerd
  there kills the running job.
- The DNS servers: `roles/dns_infrastructure/tasks/common.yaml` writes its own
  `daemon.json` with only `"log-driver": "journald"`, which overwrites the
  mirror line whenever that role runs after `docker_ce.yaml`.

## Service unit

`registry-cache.service` is `Type=oneshot`, `RemainAfterExit=yes`, with
`ExecStartPre=docker compose pull --quiet` and `up -d --remove-orphans`. It is a
compose project, so the `docker-refresh@` timer that `node_exporter.yaml`
installs on all hosts keeps `registry:2` and `grafana/promtail:latest` current.

## Monitoring

- Prometheus job `registry-cache` scrapes `registry-cache.home:5011-5014`
  (`files/ocean-prometheus/prometheus.yml.j2`) and relabels each port to an
  `upstream` label.
- No dedicated alert rules. A container that stops answering metrics trips the
  generic `ServiceDown` (`up{job!~"blackbox-.*|netboot-hosts"} == 0` for 5m) in
  `files/ocean-prometheus/alert_rules.yml.j2`.
- `registry_cache_size_bytes{upstream=...}`: the registry exposes no on-disk
  size, so `/usr/local/sbin/registry-cache-size-metric` runs `du -sb` per
  upstream every 10 minutes into node-exporter's textfile dir
  (`/data01/services/node-exporter/text_files/registry_cache_size.prom`).
- Logs: a promtail sidecar (`registry-cache-promtail`) reads the host journal
  and pushes to Loki at `192.168.1.143:3100`, labelled `job=registry-cache`,
  `host=registry-cache-01`.
- Dashboard: `files/grafana-compose/dashboards/Registry_Cache.json` (uid
  `registry-cache-001`), deployed by `grafana_compose.yaml`. Panels: cache size,
  pulls served (24h), containers up, HTTP rate by status, latency p50/p95/p99,
  blob upload rate, logs.

## Divergences from the spec

- DNS is `registry-cache.home`, not `registry-cache.internal`.
- Docker is installed inline by `registry_cache.yaml`, not by `docker_ce.yaml`.
- The mirror is opt-out per host (`registry_cache_mirror`), not a
  `registry_cache_enabled` group var.
- The dashboard lives in `files/grafana-compose/dashboards/`, not
  `files/registry-cache/`, and has no hit-rate panel.
- Cache-size textfile metric and its timer were added; the spec had none.

## Not built

- Non-Hub routing. No `registry_cache_enabled`, `registry_cache_host` or
  `cached_registries` var exists, and compose templates still name `ghcr.io/`,
  `registry.gitlab.com/` and `nvcr.io/` directly (for example
  `files/gethomepage/`, `files/dns-stack/`, `files/ocean-nvidia/`). The ghcr,
  gitlab and nvcr caches run and are scraped but serve no traffic from the
  fleet.
- Garbage collection. No timer or script runs `registry garbage-collect`.
  Disk is bounded only by the ~200G VM disk; watch `registry_cache_size_bytes`.
- Cache warming. No playbook pre-pulls fleet images through the cache.
- Registry-specific alerts (the spec also deferred these).
- TLS. The mirror is plain HTTP (out of scope in the spec).

## Backout

Docker Hub pulls fall back to the upstream when the mirror is unreachable
(dockerd behavior, not verified in this repo), so stopping
`registry-cache.service` should not block Hub pulls. To remove the mirror for
real, set `registry_cache_mirror=false` for the hosts (or drop the block from
`daemon.json.j2`) and let `docker_ce.yaml` roll it out. Nothing else in the
fleet depends on the other three caches.
