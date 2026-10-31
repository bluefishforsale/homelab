# Deployment Flow

The site orchestrators and the order they apply in. A normal change never runs
these: main-apply maps it to its owning playbook and applies only that (see
[deploy-pipeline.md](deploy-pipeline.md)). The orchestrators replay only on an
`inventories/**` or `group_vars/all*` change, or by hand through the
**Deploy Services** workflow (`deploy-services.yml`). Never run them from the
laptop.

## Orchestrators

`playbooks/00_site.yaml` imports, in order:

| Playbook | Covers |
|---|---|
| `01_base_system.yaml` | packages, users and keys, sysctl/udev, logging, unattended upgrades, fail2ban, ramdisk (`individual/base/`) |
| `02_core_infrastructure.yaml` | NIC bonds and qdisc, PERC RAID, the dns01/dns02 VMs and HA stack, pi-hole (`individual/core/`) |
| `03_ocean_services.yaml` | every container service on ocean, below |

## 03_ocean_services order

Network first, because everything else is reached through it; monitoring after
the services it watches.

| Phase | Playbooks under `playbooks/individual/ocean/` |
|---|---|
| Network | `network/certbot`, `network/cloudflare_ddns`, `network/cloudflared`, `network/nginx_compose` |
| AI | `ai/llamacpp`, `ai/open_webui`, `ai/paia` |
| Media | `media/plex`, `media/sonarr`, `media/radarr`, `media/lidarr`, `media/prowlarr`, `media/bazarr`, `media/overseerr`, `media/tautulli`, `media/tdarr` |
| Monitoring | `monitoring/prometheus`, `monitoring/grafana_compose`, `monitoring/nvidia_dcgm`, `monitoring/unpoller` |
| Services | `services/terrac_com`, `services/audible_downloader`, `services/homeassistant_compose` |

Not in 03, so a site replay leaves them alone: NZBGet and the tdarr node run on
harbor (`playbooks/individual/workers/`, see
[data01-workers.md](data01-workers.md)), and services deployed by their own
repo's dispatch (blog, mem0, my-ta-jose, etrading, photonic-inventory) have a
workflow each.

## Running one

```bash
gh workflow run deploy-services.yml -f playbook=playbooks/03_ocean_services.yaml -f check_mode=true
```

Dry-run first; the same workflow without `check_mode` applies. It holds the
`deploy-ocean` concurrency group, so it waits for any ocean deploy in flight.
