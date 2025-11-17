# 🎯 Vision Statement

1. 🔄 🤖 Fully automated (Partially - Ansible playbooks exist, need GitLab/Rundeck orchestration)
2. ✅ 🔐 SSH free for 99% of tasks (Cloudflare tunnels + Access policies deployed)
3. ✅ 🛡️ Everything secured with publicly signed certs (Let's Encrypt via Cloudflare)
4. ✅ 📝 Git driven infrastructure as code (All playbooks in Git, idempotent)
5. 🔄 🏗️ Git driven build for services / containers (GitLab not deployed yet)
6. ❌ 📊 Log aggregation service (Loki not deployed)
7. ❌ 🎛️ Control plane for building infrastructure (Rundeck/Semaphore not deployed)
8. ❌ 🏢 Isolated "clusters" and "environments" / dev-prod
9. 🔄 🏷️ Versioned releases (Docker images with SHA tags for TinaCMS)
10. ❌ 🌐 iBGP we can use MetalLB / Cilium LoadBalancer in BGP modes
11. � � Service discovery for both Docker and Kubernetes (Docker DNS via container names, K8s not deployed)
12. ❌ ⚡ HA for all critical components (DNS, DHCP, LB)
13. ❌ 🛠️ Resiliency and Redundancy to achieve higher reliability
14. ❌ 🚨 DR plan for all services config and data, recover / restore within x mins
15. ✅ 🧠 Local LLM RAG (llama.cpp + open-webui deployed with GPU support)

# 🏗️ Architecture

## 📦 GitHub stores the Ansible playbooks

1. ✅ 💻 Laptop creates control plane metal
2. ✅ 📚 Git repo, build server backs up to GitHub (All playbooks in homelab repo)

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

## Monitoring

1. 🔄 📊 DNS Prometheus exporter & dashboard (Exporter ready, needs testing + K8s dashboard)

# 📋 Priority Todo List - Organized by Dependencies

## Phase 1: Foundation Infrastructure (No dependencies)

1. ❌ 🔢 Renumber IP network change subnet from /24 to /16
2. ❌ 🌐 DHCP Prometheus exporter & dashboard
3. ❌ 🕳️ Pi-hole .local domain passthrough or configure DNS properly
4. ❌ 📊 Complete DNS Prometheus exporter testing + add K8s dashboard
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

## Phase 4: Kubernetes Cluster (Requires Phase 3)

1. ❌ ☸️ Deploy base Kubernetes cluster
2. ❌ 🐝 Kubernetes Cilium networking
3. ❌ 🌐 iBGP internally with MetalLB
4. ❌ 📊 KubeDash admin interface
5. ❌ 📝 Loki for log aggregation (all services, systems, K8s)
6. ❌ 🔐 cert-manager for Let's Encrypt automation
7. ❌ 📊 etcd exporter + Grafana dashboards
8. ❌ 📈 Grafana dashboards for apiserver, HAProxy, VRRP, keepalived
9. ❌ 🔄 Kubernetes ArgoCD private GitHub repo
10. ❌ 📈 Update all Helm charts with ArgoCD automation

## Phase 5: Advanced Kubernetes Features (Requires Phase 4)

1. ❌ 🎮 Multi-instance GPU support in Kubernetes
2. ❌ 🏷️ Taints for worker nodes with data01 and/or GPU
3. ❌ 🦙 Ollama Helm app install
4. ❌ 🧠 LLM on Kubernetes (migrate from Docker)
5. ❌ 📦 Local container registry

## Phase 6: Service Discovery & Advanced Networking (Can parallelize with Phase 4-5)

1. ❌ 🔍 Consul DNS for Docker container service discovery
2. ❌ 📝 Registrator for Docker containers
3. ❌ 🌐 Nginx auto service discovery proxy backends

## Phase 7: Expansion Hardware (Can parallelize with Phase 4-6)

1. ❌ 🥧 Proxmox Raspberry Pi 5
2. ❌ ☸️ Kubernetes VM worker on Pi-5
3. ❌ 🏷️ Kubernetes ARM pod affinity

## Phase 8: DR & Resilience (Ongoing, starts after Phase 3)

1. ❌ 🚨 DR plan for all services config and data
2. ❌ 🛠️ Resiliency and Redundancy implementation
3. ❌ ⚡ HA for DNS, DHCP, LoadBalancer

# 🦊 GitLab Automation (Part of Phase 2)

1. ❌ 🔄 GitLab pulls from github.com or is triggered via webhook
2. ❌ 🏠 Homelab repo in GitLab triggers build steps on repo update
3. ❌ 🎭 Homelab repo uses Ansible Semaphore or Rundeck in a container
4. ❌ 🔑 The runner needs access to a private SSH key allowed on the internal hosts
5. ✅ ♻️ The automation then applies all playbooks, so they all need to be idempotent