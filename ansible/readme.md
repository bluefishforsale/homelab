# Ansible VM setup playbooks

## The playbooks

### Users

- sets up ssh keys from github,
- ZSH, oh-mh-zsh, poewrlevel9k, tmux, ultimate vim config.

```bash
ansible-playbook -i inventory.ini -l dns,pihole playbook_base_users.yaml
```

### Base Packages and Settings

Uses apt to install a bunch of useful CLI tools.
Sets up NTP date and timezone to UTC.
Has a task for adding path items to /etc/environment

```bash
ansible-playbook -i inventory.ini -l dns,pihole playbook_base_packages_host_settings.yaml
```

### Unattended Upgrades

Does a full package update and upgrade.

### DNS & DHCP

- Use these to setup bind9 and isc-dhcp-server
- Config files in `files/isc-dhcp-server` and `files/bind9`
- One playbook per service, run within the ansible dir

```bash
ansible-playbook -i inventory.ini -l dns playbook_core_svc_00_dns.yaml playbook_core_svc_00_dhcp_ddns.yaml
ansible-playbook -i inventory.ini -l pihole playbook_core_svc_00_pi-hole.yaml
```

### Kubernetes PKI

- Installs the CF SSL tool using brew (yes OSX only)
- One playbook for generating all the Certs.
- Two playbooks for copying them to nodes. Controller and Worker.

### Kubernetes Configs

- One playbook for generating all the Configs.
- Two playbooks for copying them to nodes. Controller and Worker.

### Kubernetes nodes

- Run all kube playbooks in order and setup the cluster from scratch

```bash
ls -1 playbook*kube*.yaml | xargs -n1 -I% ansible-playbook -i inventory.yaml %
```

## Examples

- Become Root ask pass

```bash
ansible-playbook -i inventory.yaml playbook.yaml -K
```

### Check playbook do not run

ansible-playbook -i inventory.yaml playbook.yaml --check

### Single host from inventory

```bash
ansible-playbook -i inventory.yaml -l onehost playbook.yaml
```
