# Homelab Architecture Documentation

This directory contains comprehensive architecture diagrams for the homelab infrastructure, showing the physical layout, services, network topology, and deployment workflow.

## Architecture Diagrams

### 1. Physical Architecture
![Rack Front](./homelab_rack_front.jpg)
![Rack Rear](./homelab_rack_front.jpg)

**Overview**: Shows the two physical servers and their relationships:
- **Ocean Server** (192.168.1.143): Ubuntu Docker host with ZFS and NVIDIA P2000 GPU
- **Node006** (192.168.1.106): Proxmox host running all VMs (DNS, K8s cluster, GitLab, Pi-hole)

### 2. Ocean Services Architecture  
![Ocean Services](./ocean-services.png)

**Overview**: Detailed view of all 21 services running on the Ocean server:
- **Infrastructure**: MySQL, Nginx, Cloudflare services
- **Media Services**: Plex ecosystem with full Arr suite automation
- **AI/ML Services**: GPU-accelerated LLM and image generation
- **Monitoring**: Prometheus and Grafana stack

### 3. Network Topology
![Network Topology](./network-topology.png)

**Overview**: Shows network flows and connectivity:
- External access via Cloudflare tunnels and Access policies
- Internal network routing and DNS resolution
- Service interconnections and dependencies

### 4. Deployment Flow
![Deployment Flow](./deployment-flow.png)

**Overview**: 5-phase deployment strategy with dependencies:
1. **Infrastructure Foundation**: Core services setup
2. **Network Services**: DNS and tunnels
3. **Media Stack**: Complete automation pipeline
4. **AI/ML Services**: GPU-powered workloads
5. **Optional Services**: Enhancement and monitoring

## File Structure

```
docs/architecture/
├── README.md                    # This file
├── physical-architecture.png    # Physical server layout
├── ocean-services.png          # All ocean services
├── network-topology.png        # Network connectivity
├── deployment-flow.png         # Deployment phases
├── *.md files                  # Markdown source with mermaid code
└── *.mmd files                 # Raw mermaid diagram files
```

## Key Infrastructure Facts

### Physical Servers
- **Ocean**: Pre-existing Ubuntu server being converted to Ansible management
- **Node006**: Proxmox hypervisor hosting all VMs

### Services Count
- **21 Ocean Services**: All containerized with Docker Compose
- **GPU Utilization**: NVIDIA P2000 for AI/ML workloads
- **Storage**: ZFS pool with automatic snapshots and management

### Network Architecture
- **Internal**: 192.168.1.0/24 network
- **External**: Cloudflare tunnels for secure access
- **DNS**: Redundant DNS with Pi-hole ad blocking

### Deployment Strategy
- **Idempotent**: All playbooks safe to run multiple times
- **Phased**: Clear dependencies and logical ordering
- **Automated**: Full infrastructure as code approach

## Using These Diagrams

These diagrams can be included in:
- Project documentation and README files
- Technical presentations and reviews
- Troubleshooting and planning sessions
- New team member onboarding

All diagrams are in PNG format with transparent backgrounds for easy inclusion in documentation.
