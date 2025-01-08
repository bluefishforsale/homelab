# Vision Statement
1. Fully automated
1. SSH free for 99% of tasks
1. Everything secured with publicly signed certs (no self-signed anywhere)
1. Git driven infrastructure as code
1. Git driven build for services / containers
1. Control plane for building infrastructure
1. Isolated "clusters" and "environments" / dev-prod
1. Versioned releases
1. iBGP we can use metalLB / Cilium Loadbalancer in GBP modes
1. Service discovery for both docker and kubernetes eg. grafana.home, grafana.svc.cluster.local
1. HA for all critical components (dns, dhcp, LB)
1. Resiliancy and Redundancy to achieve higher reliability
1. DR plan for all services config and data, recover / restore within x mins

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


1. kubernetes ingress for services
1. pihole .local domain passthrough
1. kubernetes gpu support
1. kubernetes argoCD
1. update all charts and have ArgoCD automate their installation
1. kubernetes secrets and letsencrypt certs automation - certmanager?
1. kubernetes cilium
1. iBGP internally
1. vault for secrets - use it to bootstrap kubernetes?
1. alertmanager and alerts for critical components
1. local container repo
1. rewrite playbooks so they are idempotent (produces the same result when applied multiple times as it does when applied once)
1. LLM on kubernetes
1. proxmox automated installation w/ PXE, TFTP, DHCP
1. git ansible ocean services
1. proxmox ocean -> node006
1. ansible automate VM creation
1. proxmox move ocean to VM node006
1. proxmox import existing ZFS pool
1. install proxmox on raspberry pi 5
1. pi5 as control-plane node
1. move dns, dhcp to pi5
1. vm for pxe install on pi5
1. playbook for installing gitlab
1. GitLab on pi5 for automating the run of ansible playbooks
1. renumber IP network change subnet from /24 to /16
1. playbook to install nextcloud