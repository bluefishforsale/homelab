# 🎯 Vision Statement

1. 🤖 Fully automated
2. 🔐 SSH free for 99% of tasks
3. 🛡️ Everything secured with publicly signed certs (no self-signed anywhere)
4. 📝 Git driven infrastructure as code
5. 🏗️ Git driven build for services / containers
6. 📊 Log aggregation service
7. 🎛️ Control plane for building infrastructure
8. 🏢 Isolated "clusters" and "environments" / dev-prod
9. 🏷️ Versioned releases
10. 🌐 iBGP we can use MetalLB / Cilium LoadBalancer in BGP modes
11. 🔍 Service discovery for both Docker and Kubernetes (e.g., grafana.home, grafana.svc.cluster.local)
12. ⚡ HA for all critical components (DNS, DHCP, LB)
13. 🛠️ Resiliency and Redundancy to achieve higher reliability
14. 🚨 DR plan for all services config and data, recover / restore within x mins
15. 🧠 Local LLM RAG

# 🏗️ Architecture

## 📦 GitHub stores the Ansible playbooks

1. 💻 Laptop creates control plane metal
2. 📚 Git repo, build server backs up to GitHub

## 🖥️ Control plane metal

1. 🌐 DHCP DNS setup by manual install
2. 📡 PXE TFTP GitLab manual install
3. 🚀 GitLab deploys Rundeck
4. ⚙️ PXE boot automated install of Proxmox
5. 🖥️ Rundeck creates VMs on Proxmox metals
6. 🔧 Run Ansible playbooks with Rundeck for web UI, maybe build server triggers Rundeck tasks
7. 👀 Maybe Rundeck just watches the repo for changes

## 🏠 Proxmox

1. ⚡ HA for both metals, allowing control plane VMs to migrate between
2. 🔄 _[Additional Proxmox items to be added]_

## ☸️ Kubernetes

1. ~~🌐 Kubernetes ingress for services~~
2. 🔄 Kubernetes ArgoCD private GitHub repo
3. 📊 KubeDash admin
4. 📝 Loki for all services, systems, and Kubernetes logs
5. 📊 etcd exporter
6. 📈 Grafana dashboards for requests/latency for apiserver, HAProxy, VRRP, keepalived, etcd
7. 🎮 Multi-instance GPU support
8. 🏷️ Taints for worker node VMs that have data01 and/or GPU
9. 🦙 Ollama Helm app install
10. 🔐 Secrets and Let's Encrypt certs automation - cert-manager?


## 🤖 Automated Ansible Playbooks

1. 🎭 Ansible Semaphore - https://semaphoreui.com/
2. 🔍 Investigate Temporal also - https://hub.docker.com/r/temporalio/server
3. ♻️ All idempotent
4. ✍️ Writes all configuration
5. 🏗️ Creates Infrastructure
6. 🔎 Perform investigation actions
7. 🔄 Restarts services on config change (CI/CD)
8. 🖥️ VM creation ready to be configured


## 🦊 GitLab vs Rundeck Ansible separation of concern

1. 📧 Automate email configuration
2. 🔑 Automate set default root password
3. 🐳 GitLab builds container images
4. 🔄 GitLab will install and keep Rundeck updated
5. ⚙️ GitLab will only produce config to be run by Rundeck / Ansible
6. 🏗️ Rundeck / Ansible is responsible for all IaC

## 🌊 Ocean to node006 Proxmox host

1. 💾 Decide on Proxmox boot disk configuration
2. 🔄 Convert all ocean services to Ansible in Git
3. 🖥️ Create VM with ocean SSD passthrough
4. 📦 Export / import data01 ZFS pool from ocean → node006

# 📋 Todo List

1. 🔍 Consul DNS for Docker container service discovery
2. 📝 Registrator for Docker containers
3. 🌐 Nginx auto service discovery proxy backends
4. 📊 DNS Prometheus exporter & dashboard
   - ✅ Exporter and bind config checked in but untested
   - ✅ Dashboard imported to ocean by hand
   - ❌ Need to add dashboard to kube-prometheus-stack
5. 🌐 DHCP Prometheus exporter & dashboard
6. ♻️ Rewrite playbooks so they are idempotent
7. 📦 Port ZFS pool to node006 Proxmox
8. ⚙️ Proxmox automated installation w/ PXE, TFTP, DHCP
9. 📥 Proxmox import existing ZFS pool
10. 🎧 Docker container for Audible download and convert
11. 🦊 GitLab Ansible playbook on control-plane metal
12. 📖 Runbook on control-plane metal deployed by GitLab
13. 💡 List ideas for runbook tasks
14. 🤖 Ansible automate VM creation
15. 🕳️ Pi-hole .local domain passthrough or shut down Pi-hole or Ansible configure Pi-hole on disk to allow this
16. 🚨 AlertManager and alerts for critical components
17. 📈 Update all charts and have ArgoCD automate their installation
18. 🐝 Kubernetes Cilium
19. 🌐 iBGP internally
20. 🔐 Vault for secrets - use it to bootstrap Kubernetes?
21. 📦 Local container repo
22. 🧠 LLM on Kubernetes
23. 🥧 Proxmox Raspberry Pi 5
24. ☸️ Kubernetes VM worker on Pi-5
25. 🏷️ Kubernetes ARM pod affinity
26. ☁️ NextCloud Ansible playbook
27. 🔢 Renumber IP network change subnet from /24 to /16


# 🦊 GitLab Automation

1. 🔄 GitLab pulls from github.com or is triggered via webhook
2. 🏠 Homelab repo in GitLab triggers build steps on repo update
3. 🎭 Homelab repo uses Ansible Semaphore or Rundeck in a container
4. 🔑 The runner needs access to a private SSH key allowed on the internal hosts
5. ♻️ The automation then applies all playbooks, so they all need to be idempotent