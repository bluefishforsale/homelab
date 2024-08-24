# Bluefishforsale Homelab Automation

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
- [Proxmox host ready to run VMs](https://github.com/bluefishforsale/homelab/blob/master/readme_proxmox.md)
- Ensure that Ceph storage (Ceph-LVM and CephFS) is properly configured
- TODO: integrate this into the DHCP setup w/ PXE and Preseed

### 1. Create the VMs
Start by creating the necessary VMs as described in the [Proxmox Readme](https://github.com/bluefishforsale/homelab/blob/master/readme_proxmox.md) file. This is done using the following Ansible playbook:

### 2. Continue to ansible setup for hosts

- [Ansible setup Readme](https://github.com/bluefishforsale/homelab/blob/master/ansible/readme.md)

