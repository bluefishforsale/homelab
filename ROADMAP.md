# 🎯 Vision Statement

1. 🔄 🤖 Fully automated (Partially - Ansible playbooks exist, GitHub Actions runners deployed)
2. ✅ 🔐 SSH free for 99% of tasks (Cloudflare tunnels + Access policies deployed)
3. ✅ 🛡️ Everything secured with publicly signed certs (Let's Encrypt via Cloudflare)
4. ✅ 📝 Git driven infrastructure as code (All playbooks in Git, idempotent)
5. 🔄 🏗️ Git driven build for services / containers (GitHub Actions with 4 ephemeral runners)
6. ❌ 📊 Log aggregation service (Loki not deployed)
7. ❌ 🎛️ Control plane for building infrastructure (Rundeck/Semaphore not deployed)
8. ❌ 🏢 Isolated "clusters" and "environments" / dev-prod
9. 🔄 🏷️ Versioned releases (Docker images with SHA tags for TinaCMS)
10. ❌ 🌐 iBGP for network routing
11. 🔄 🌐 Service discovery for Docker (Docker DNS via container names)
12. ❌ ⚡ HA for all critical components (DNS, DHCP, LB)
13. ❌ 🛠️ Resiliency and Redundancy to achieve higher reliability
14. ❌ 🚨 DR plan for all services config and data, recover / restore within x mins
15. ✅ 🧠 Local LLM RAG (llama.cpp + open-webui deployed with GPU support)

# 🏗️ Architecture

## 📦 GitHub stores the Ansible playbooks

1. ✅ 💻 Laptop creates control plane metal
2. ✅ 📚 Git repo, build server backs up to GitHub (All playbooks in homelab repo)
3. ✅ 🏃 GitHub Actions self-hosted runners deployed (4 ephemeral Docker runners with Ansible)

## 🖥️ Control plane metal

1. ✅ 🌐 DHCP DNS setup by manual install (DNS running with bind9 + DDNS)
2. ❌ 📡 PXE TFTP GitLab manual install
3. ❌ 🚀 GitLab deploys Rundeck
4. ❌ ⚙️ PXE boot automated install of Proxmox
5. ❌ 🖥️ Rundeck creates VMs on Proxmox metals
6. ❌ 🔧 Run Ansible playbooks with Rundeck for web UI, maybe build server triggers Rundeck tasks
7. ❌ 👀 Maybe Rundeck just watches the repo for changes

## 🏠 Proxmox

1. ❌ ⚡ HA for both metals, allowing control plane VMs to migrate between
2. 🔄 _[Additional Proxmox items to be added]_

aybooks

1. ❌ 🎭 Ansible Semaphore - https://semaphoreui.com/
2. ❌ 🔍 Investigate Temporal also - https://hub.docker.com/r/temporalio/server
3. ✅ ♻️ All idempotent (All 20+ playbooks are idempotent)
4. ✅ ✍️ Writes all configuration (Docker compose, systemd services, configs)
5. ✅ 🏗️ Creates Infrastructure (Services, networks, volumes, directories)
6. ✅ 🔎 Perform investigation actions (Health checks implemented)
7. ✅ 🔄 Restarts services on config change (CI/CD) (Handlers restart on changes)
8. ❌ 🖥️ VM creation ready to be configured


## 🦊 GitLab vs Rundeck Ansible separation of concern

1. � � Automate email configuration (Postfix configured for GitLab, not deployed)
2. ❌ 🔑 Automate set default root password
3. ❌ 🐳 GitLab builds container images
4. ❌ 🔄 GitLab will install and keep Rundeck updated
5. ❌ ⚙️ GitLab will only produce config to be run by Rundeck / Ansible
6. ✅ 🏗️ Rundeck / Ansible is responsible for all IaC (Ansible handles all IaC currently)

## 🌊 Ocean to node006 Proxmox host

1. ❌ 💾 Decide on Proxmox boot disk configuration
2. ✅ 🔄 Convert all ocean services to Ansible in Git (20+ services deployed)
3. ❌ 🖥️ Create VM with ocean SSD passthrough
4. ❌ 📦 Export / import data01 ZFS pool from ocean → node006

---

# ✅ Completed Items

## Infrastructure & Services

1. ✅ ♻️ Rewrite playbooks so they are idempotent (All 20+ playbooks idempotent)
2. ✅ 🎧 Docker container for Audible download and convert (Deployed with playbook)
3. ✅ ☁️ NextCloud Ansible playbook (Deployed with MariaDB + Redis)
4. ✅ 🔐 Vault for secrets (ansible-vault for all sensitive data)
5. ✅ 🧠 LLM infrastructure (llama.cpp + open-webui with P2000 GPU)
6. ✅ nginx reverse proxy with Cloudflare tunnels
7. ✅ Cloudflare Access policies automated
8. ✅ 20+ Docker Compose services with systemd integration
9. ✅ Grafana + MySQL consolidated stack
10. ✅ ComfyUI with automated model management
11. ✅ n8n workflow automation with PostgreSQL
12. ✅ Media stack (Plex, Sonarr, Radarr, Prowlarr, etc.)
13. ✅ Prometheus monitoring
14. ✅ CMS platforms (PayloadCMS, Strapi, TinaCMS)
15. ✅ 🏃 GitHub Actions self-hosted runners (4 ephemeral Docker containers with Ansible + Docker support)
16. ✅ 📝 TinaCMS Next.js demo blog (bluefishforsale/tinacms-nextjs with SHA-based image tags)

## AI/ML Infrastructure

1. ✅ 🦙 llama.cpp GPU-accelerated LLM API server (Nvidia P2000 CUDA support)
2. ✅ 🌐 Open WebUI with automatic llama.cpp integration (pre-configured API endpoints)
3. ✅ 🎨 ComfyUI with automated model management (FLUX, VAE, LoRA, ControlNet)
4. ✅ 🔄 n8n workflow automation with PostgreSQL backend and GPU access
5. ✅ 📦 Automated model downloading and permission management

## Monitoring

1. 🔄 📊 DNS Prometheus exporter & dashboard (Exporter ready, needs testing)

# 📋 Priority Todo List - Organized by Dependencies

## Phase 1: Foundation Infrastructure (No dependencies)

1. ❌ 🔢 Renumber IP network change subnet from /24 to /16
2. ❌ 🌐 DHCP Prometheus exporter & dashboard
3. ❌ 🕳️ Pi-hole .local domain passthrough or configure DNS properly
4. ❌ 📊 Complete DNS Prometheus exporter testing
5. ❌ 🚨 AlertManager and alerts for critical components (depends on Prometheus ✅)

## Phase 2: Control Plane & Automation (Requires Phase 1)

1. ❌ 🦊 GitLab Ansible playbook on control-plane metal
2. ❌ 📡 PXE TFTP setup for automated OS installs
3. ❌ 🎭 Ansible Semaphore or Rundeck deployment (requires GitLab)
4. ❌ 📖 Runbook on control-plane metal deployed by GitLab
5. ❌ 💡 List ideas for runbook tasks
6. ❌ 🤖 Ansible automate VM creation

## Phase 3: Proxmox Migration (Requires Phase 2)

1. ❌ 💾 Decide on Proxmox boot disk configuration
2. ❌ ⚙️ Proxmox automated installation w/ PXE, TFTP, DHCP
3. ❌ 📦 Port ZFS pool to node006 Proxmox
4. ❌ 📥 Proxmox import existing ZFS pool
5. ❌ 🖥️ Create VM with ocean SSD passthrough
6. ❌ ⚡ Proxmox HA for both metals

## Phase 4: Service Discovery & Advanced Networking (Requires Phase 3)

1. ❌ 🔍 Consul DNS for Docker container service discovery
2. ❌ 📝 Registrator for Docker containers
3. ❌ 🌐 Nginx auto service discovery proxy backends

## Phase 5: Expansion Hardware (Can parallelize with Phase 3-4)

1. ❌ 🥧 Proxmox Raspberry Pi 5

## Phase 6: DR & Resilience (Ongoing, starts after Phase 3)

1. ❌ 🚨 DR plan for all services config and data
2. ❌ 🛠️ Resiliency and Redundancy implementation
3. ❌ ⚡ HA for DNS, DHCP, LoadBalancer

# 🏃 GitHub Actions Automation (Current Approach)

## ✅ Deployed Configuration

1. ✅ 🐳 4 ephemeral Docker-based runners on ocean server
2. ✅ 🔄 Fresh runner container per job (ephemeral mode)
3. ✅ 🐋 Docker socket mounted for container builds in workflows
4. ✅ 🎭 Ansible pre-installed for infrastructure automation
5. ✅ 🏷️ Custom labels: self-hosted, homelab, ansible, ephemeral, docker
6. ✅ ♻️ Auto-restart after job completion for next workflow
7. ✅ 📦 Using myoung34/github-runner image (well-maintained ephemeral support)
8. ✅ 🔐 SSH key mounting for Ansible access to homelab hosts
9. ✅ 🎯 Repository-level runners (bluefishforsale/homelab)
10. ✅ ⚙️ Systemd service management for runner lifecycle

## 📝 Example Workflow Usage

```yaml
jobs:
  deploy:
    runs-on: [self-hosted, homelab, ansible]
    steps:
      - uses: actions/checkout@v4
      - name: Deploy with Ansible
        run: ansible-playbook playbook_ocean_nginx.yaml

  build:
    runs-on: [self-hosted, homelab, docker]
    steps:
      - uses: actions/checkout@v4
      - name: Build Docker image
        run: docker build -t myapp:${{ github.sha }} .
```

## 🔄 Benefits of Current Setup

- No GitLab infrastructure needed
- Integrated with GitHub repository
- Ephemeral runners = clean builds every time
- Can run Ansible playbooks directly from workflows
- Docker builds available for CI/CD pipelines
- Runs on existing ocean server (no additional hardware)
- Free for private repositories with self-hosted runners

# 🦊 GitLab Automation (Future Alternative - Part of Phase 2)

1. ❌ 🔄 GitLab pulls from github.com or is triggered via webhook
2. ❌ 🏠 Homelab repo in GitLab triggers build steps on repo update
3. ❌ 🎭 Homelab repo uses Ansible Semaphore or Rundeck in a container
4. ✅ 🔑 SSH key access pattern established with GitHub runners (can be reused)
5. ✅ ♻️ The automation then applies all playbooks, so they all need to be idempotent

**Note**: GitHub Actions runners provide similar functionality to GitLab + Rundeck approach.