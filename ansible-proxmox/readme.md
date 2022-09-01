# how to run these playbooks

### TL;DR

Run all kube playbooks in order and setup the cluster from scratch
```
for FILE in playbook*kube*.yaml ; do ansible-playbook -i inventory.yaml $FILE ; done
```

# examples:
### Become Root ask pass
ansible-playbook -i inventory.yaml playbook.yaml -K

### Check playbook do not run
ansible-playbook -i inventory.yaml playbook.yaml --check

### Single host from inventory
ansible-playbook -i inventory.yaml -l onehost playbook.yaml


## The playbooks

### Base Packages and Settings
Uses apt to install a bunch of useful CLI tools.
Sets up NTP date and timezone to UTC.
Has a task for adding path items to /etc/environment

### Unattended Upgrades
Does a full package update and upgrade.

### Users
sets up ssh keys from github,
ZSH, oh-mh-zsh, poewrlevel9k, tmux, ultimate vim config.

### DNS & DHCP
Use these to setup bind9 and isc-dhcp-server
Config files in `files/isc-dhcp-server` and `files/bind9`
One playbook per service. These need to be run with -K

### Kubernetes PKI
Installs the CF SSL tool using brew (yes OSX only)
One playbook for generating all the Certs.
Two playbooks for copying them to nodes. Controller and Worker.

### Kubernetes Configs
One playbook for generating all the Configs.
Two playbooks for copying them to nodes. Controller and Worker.
