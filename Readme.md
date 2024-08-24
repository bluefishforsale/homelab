# Homelab Automation

This repository contains Ansible playbooks to automate the setup and management of a homelab environment, which includes a Proxmox VE cluster, various VMs, and a Kubernetes cluster. The playbooks are organized into different phases to ensure a step-by-step configuration of the entire infrastructure.

## Overview

### Infrastructure Components
1. **Proxmox VE Cluster**: 
   - Manages the virtualization of multiple VMs, including those used for DNS, DHCP, and Kubernetes.

2. **DNS and DHCP VM**:
   - **IP**: 192.168.1.2/32
   - Hosts BIND and DHCPd services.

3. **Pi-Hole VM**:
   - **IP**: 192.168.1.9/32
   - Provides network-wide ad blocking and DNS filtering.

4. **Kubernetes VMs**:
   - Six EFI VMs are provisioned to form a Kubernetes cluster.
   - **Network Configuration**:
     - **Cluster CIDR**: 10.0.0.0/16
     - **Nodes CIDR**: 10.0.${node_number}.0/16
     - **Services CIDR**: 10.0.250.0/20
     - **API Server**: 192.168.1.99/32
   - Utilizes `kube-proxy` for networking.

## Getting Started with Ansible Playbooks

### 0. Pre-requisites

This is a critical step before provisioning the VMs.
- Proxmox host(s) setup and ready to run VMs
- Ceph storage (Ceph-LVM and CephFS) is properly configured
- TODO: integrate this into the DHCP setup w/ PXE and Preseed

### 1. Setup Proxmox host(s)

- [Proxmox Readme](https://github.com/bluefishforsale/homelab/blob/master/readme_proxmox.md)

### 2. Create the VMs

- Creating the necessary VMs as described in [Proxmox Readme#making-vms](https://github.com/bluefishforsale/homelab/blob/master/readme_proxmox.md#making-vms)

### 3. Set up the hosts for roles usign Ansible playbooks

- [Ansible setup Readme](https://github.com/bluefishforsale/homelab/blob/master/ansible/readme.md)