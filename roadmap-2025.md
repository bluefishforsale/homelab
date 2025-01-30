# Vision Statement
1. Fully automated
1. SSH free for 99% of tasks
1. Everything secured with publicly signed certs (no self-signed anywhere)
1. Git driven infrastructure as code
1. Git driven build for services / containers
1. Logg aggregation service
1. Control plane for building infrastructure
1. Isolated "clusters" and "environments" / dev-prod
1. Versioned releases
1. iBGP we can use metalLB / Cilium Loadbalancer in GBP modes
1. Service discovery for both docker and kubernetes eg. grafana.home, grafana.svc.cluster.local
1. HA for all critical components (dns, dhcp, LB)
1. Resiliancy and Redundancy to achieve higher reliability
1. DR plan for all services config and data, recover / restore within x mins
1. local LLM RAG

# Architecture
## Github stores the Ansible playbooks
1. Laptop creates control plane metal
1. Git repo, build server backs up to github

## Control plane metal 
1. dhcp dns setup by manual install
1. pxe tftp gitlab manual install
1. gitlab deploys rundeck
1. pxe boot automated install of proxmox
1. rundeck creates VMs on proxmox metals
1. Run ansible playbooks with run deck for web UI, maybe build server triggers rundeck tasks
1. Maybe run deck just watches the repo for changes 

## Proxmox metals
1. Automatically installed, configured from network

## Automated Ansible Playbooks 
1. ansible semaphore https://semaphoreui.com/
1. investigate temportal also https://hub.docker.com/r/temporalio/server 
1. All idempotent 
1. Writes all configuration
1. Creates Infrastructure
1. Perform investigation actions
1. Restarts services on config change (CI/CD)
1. VM  creation ready to be configured

## Gitlab vs Rundeck Ansible separation of convern
1. GitLab builds container images
1. GitLab will install and keep rundeck updated
1. GitLab will only produce config to be run by Rundeck / Ansible
1. Rundeck / Ansible is responsible for all IAC

# Todo list 
1. Consul DNS
1. Registrator for docker containers
1. nginx auto service discovery proxy backends
1. DNS prometheus exporter & dashboard 
    - exporter and bind config checked in but untested
    - dashboard imported to ocean by hand
    - need to add dashboard to kube-prometheus-stack
1. DHCP prometheus exporter & dashboard
1. convert all ocean services to ansible in git
1. rewrite playbooks so they are idempotent 
1. proxmox move ocean to VM node006
1. port ZFS pool to node006 proxmox
1. proxmox automated installation w/ PXE, TFTP, DHCP
1. proxmox ocean -> node006
1. proxmox import existing ZFS pool
1. docker container for audible downaload and convert
1. Gitlab ansible playbook on control-plane metal
1. Runbook on contol-plane metal deplyoed by gitlab
1. List ideas for runbook tasks
1. Ansible automate VM creation
1. pihole .local domain passthrough or shut down pi-hole or ansible configure pi-hole on disk to allow this
1. kubernetes ingress for services
1. kubernetes etcd exporter
1. kubernetes dashboards for requests/latency for apiserver, haproxy, vrrp, keepalived, etcd
1. kubernetes admin kubedash
1. Loki for all services, systems, and kubernetes logs
1. kubernetes gpu support
1. kubernetes argoCD
1. alertmanager and alerts for critical components
1. update all charts and have ArgoCD automate their installation
1. kubernetes secrets and letsencrypt certs automation - certmanager?
1. kubernetes cilium
1. iBGP internally
1. vault for secrets - use it to bootstrap kubernetes?
1. local container repo
1. LLM on kubernetes
1. proxmox raspberry pi 5
1. kubernetes VM worker on pi-5
1. kubernetes ARM pod affinity
1. NextCloud ansible playbook
1. renumber IP network change subnet from /24 to /16


# gitlab automation
1. gitlab pulls from github.com or is triggered via webhook
1. homelab repo in gitlab triggers build steps on repo update
1. homelab repo uses ansible semaphore or rundeck in a container
1. the runner needs access to a private ssh key allowed on the internal hosts
1. the automation then applies all playbooks, so they all need to be idempotent