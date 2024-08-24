# Ansible setup for VMs
Not Ready: ansible playbook to create the VMs 
```bash
ansible-playbook -i inventory.ini playbook_proxmox_create_kube_vm.yaml
```

### 1. DHCP & DNS
Host groups where these plays are installed are in the playbooks themselves.

DHCP & DNS verify VM Availability
```bash
ansible -i inventory.ini dns -b -a 'uptime'
```
Run the playbooks
```bash
ansible-playbook -i inventory.ini -l playbook_core_svc_00_dhcp_ddns.yaml
ansible-playbook -i inventory.ini -l playbook_core_svc_00_dns.yaml
```

### 2. PiHole
```bash
ansible-playbook -i inventory.ini -l playbook_core_svc_00_pi-hole.yaml
```

### 3. K8s verify VM Availability
```bash
ansible -i inventory.ini k8s -b -a 'uptime'
```

### 4. Post-installation and Initial Setup
```bash
ansible-playbook -i inventory.ini -l proxmox playbook_core_net_qdisc.yaml
ansible-playbook -i inventory.ini -l k8s playbook_proxmox_vm_post_install.yaml
ansible-playbook -i inventory.ini -l k8s,proxmox playbook_base_packages_host_settings.yaml
ansible-playbook -i inventory.ini -l k8s,proxmox playbook_base_users.yaml
ansible -i inventory.ini k8s -b -a 'reboot'
```

### 5. Sequential Deployment of Kubernetes Components
Run each phase sequentially, checking for errors after each
```bash
ansible-playbook -i inventory.ini playbook_kube_00.1_gen_pki.yaml
ansible-playbook -i inventory.ini playbook_kube_00.2_gen_kubeconfigs.yaml
ansible-playbook -i inventory.ini playbook_kube_01.1_pki_dist_controller.yaml
ansible-playbook -i inventory.ini playbook_kube_01.2_pki_dist_worker.yaml
ansible-playbook -i inventory.ini playbook_kube_01.3_pki_dist_enckey_controller.yaml
ansible-playbook -i inventory.ini playbook_kube_01.4_kubeconf_dist_controller.yaml
ansible-playbook -i inventory.ini playbook_kube_01.5_kubeconf_dist_worker.yaml
ansible-playbook -i inventory.ini playbook_kube_04.0_etcd.yaml
ansible-playbook -i inventory.ini playbook_kube_05.1_k8s_binaries_control.yaml
ansible-playbook -i inventory.ini playbook_kube_05.2_k8s_binaries_worker.yaml
ansible-playbook -i inventory.ini playbook_kube_07.0_haproxy_keepalive.yaml
ansible-playbook -i inventory.ini playbook_kube_08.1_node_admin_kubeconf.yaml
ansible-playbook -i inventory.ini playbook_kube_09.1_local_join_cluster.yaml
ansible-playbook -i inventory.ini playbook_kube_09.2_RBAC.yaml
ansible-playbook -i inventory.ini playbook_kube_09.3_local_label_nodes.yaml
ansible-playbook -i inventory.ini playbook_kube_10.0_cilium.yaml
```

#### A. Kubernetes Components as one command
This runs all kube playbooks sequentially, not stopping for errors
```bash
ls -1 playbook_kube_* | xargs -n1 -I% ansible-playbook -i inventory.ini %
```

### 6. Networking, Ceph, and Pod Deployment
[Readme-proxmox.md](https://github.com/bluefishforsale/homelab-kube/blob/master/Readme.md)
