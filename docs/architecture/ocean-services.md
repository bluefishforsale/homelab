# Ocean Services Architecture

Services running on ocean VM (192.168.1.143).

---

## Service Diagram

```mermaid
graph TB
    subgraph ocean["Ocean VM (192.168.1.143)"]
        subgraph network["Network"]
            Nginx[nginx<br/>:80, :443]
            DDNS[cloudflare_ddns]
            Cloudflared[cloudflared]
        end
        
        subgraph ai["AI/ML (GPU)"]
            LlamaCpp[llama.cpp<br/>:8080]
            OpenWebUI[Open WebUI<br/>:3000]
            ComfyUI[ComfyUI<br/>:8188]
        end
        
        subgraph media["Media"]
            Plex[Plex<br/>:32400]
            Sonarr[Sonarr<br/>:8989]
            Radarr[Radarr<br/>:7878]
            Prowlarr[Prowlarr<br/>:9696]
            Bazarr[Bazarr<br/>:6767]
            NZBGet[NZBGet<br/>:6789]
            Overseerr[Overseerr<br/>:5055]
            Tautulli[Tautulli<br/>:8181]
            Tdarr[Tdarr<br/>:8265]
        end
        
        subgraph monitoring["Monitoring"]
            Prometheus[Prometheus<br/>:9090]
            Grafana[Grafana<br/>:8910]
            DCGM[NVIDIA DCGM]
            UnPoller[UnPoller<br/>:9130]
        end
        
        subgraph services["Services"]
            NextCloud[NextCloud]
            TinaCMS[TinaCMS]
            Frigate[Frigate]
            HomeAssistant[Home Assistant]
            Audible[Audible Downloader]
        end
        
        subgraph hardware["Hardware"]
            GPU[RTX 3090<br/>24GB VRAM]
            ZFS[ZFS data01<br/>8x 12TB]
        end
    end
    
    Internet[Internet] --> Cloudflared
    Cloudflared --> Nginx
    Nginx --> Plex
    Nginx --> OpenWebUI
    
    Prowlarr --> Sonarr
    Prowlarr --> Radarr
    Sonarr --> NZBGet
    Radarr --> NZBGet
    
    Plex --> Tautulli
    Overseerr --> Sonarr
    Overseerr --> Radarr
    
    LlamaCpp --> OpenWebUI
    GPU --> LlamaCpp
    GPU --> ComfyUI
    GPU --> Plex
    
    Prometheus --> Grafana
    DCGM --> Prometheus
    UnPoller --> Prometheus
    
    ZFS --> Plex
    ZFS --> LlamaCpp
```

---

## Service List

| Category | Services |
|----------|----------|
| Network | nginx, cloudflared, cloudflare_ddns |
| AI/ML | llama.cpp, Open WebUI, ComfyUI |
| Media | Plex, Sonarr, Radarr, Prowlarr, Bazarr, NZBGet, Overseerr, Tautulli, Tdarr |
| Monitoring | Prometheus, Grafana, NVIDIA DCGM, UnPoller |
| Services | NextCloud, TinaCMS, Audible Downloader, Frigate, Home Assistant |

---

## Deploy

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/03_ocean_services.yaml --ask-vault-pass
```

See [deployment-flow.md](deployment-flow.md) for service order.
