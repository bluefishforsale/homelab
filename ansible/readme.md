
### 2. Verify VM Availability
```bash
ansible -i inventory.ini k8s -b -a 'uptime'
```

### 3. Post-installation and Initial Setup
```bash
ansible-playbook -i inventory.ini -l k8s playbook_proxmox_vm_post_install.yaml
ansible-playbook -i inventory.ini -l k8s,proxmox playbook_base_unattended_upgrade.yaml
ansible-playbook -i inventory.ini -l k8s,proxmox playbook_base_packages_host_settings.yaml
ansible-playbook -i inventory.ini -l k8s,proxmox playbook_core_net_qdisc.yaml
ansible-playbook -i inventory.ini -l k8s,proxmox playbook_base_users.yaml
ansible -i inventory.ini k8s -b -a 'reboot'
```

### 4. Sequential Deployment of Kubernetes Components
```bash
# Run each phase sequentially, checking for errors after each
ls -1 playbook_kube_00.* | xargs -n1 -I% ansible-playbook -i inventory.ini %
ls -1 playbook_kube_01.* | xargs -n1 -I% ansible-playbook -i inventory.ini %
ls -1 playbook_kube_04.* | xargs -n1 -I% ansible-playbook -i inventory.ini %
ls -1 playbook_kube_05.* | xargs -n1 -I% ansible-playbook -i inventory.ini %
ls -1 playbook_kube_07.* | xargs -n1 -I% ansible-playbook -i inventory.ini %
ls -1 playbook_kube_08.* | xargs -n1 -I% ansible-playbook -i inventory.ini %
ls -1 playbook_kube_09.* | xargs -n1 -I% ansible-playbook -i inventory.ini %
ls -1 playbook_kube_10.* | xargs -n1 -I% ansible-playbook -i inventory.ini %
```

#### A. Kubernetes Components as one command
```bash
# Run all playbooks in one go if you're confident there are no errors
ls -1 playbook_kube_* | xargs -n1 -I% ansible-playbook -i inventory.ini %
```
