# Homelab
## start in the ansible/ directory for all this
0. todo: write instructions for creating ceph-lvm and cephfs

1. create the vms using `readme_proxmox.md` instrucitons

2. proceed with 3. when uptime returns w.o password on all VMs
  - ansible -i inventory.ini k8s  -b -a 'uptime'

3. proceed with step 4. only when post-install and reboot completes
  - ansible-playbook -i inventory.ini -l k8s playbook_proxmox_vm_post_install.yaml playbook_base_packages_host_settings.yaml playbook_base_unattended_upgrade.yaml ; ansible -i inventory.ini k8s  -b -a reboot

4. proceed with step 5. when ansible playbooks all apply whithout any error
  - ls -1 playbook_kube_* | xargs -n1 -I% ansible-playbook -i inventory.ini  %

5. proceed to kube networking, ceph, then pods only when Kube clsuter is deployed correctly
  - https://github.com/bluefishforsale/homelab-kube/blob/master/Readme-proxmox.md


## notes about LACP 802.3ad transmit hash etc.
The US-16-XG 10G needs to have the port-channels hash transmit modes changed via ssh

# Netplan reference
https://netplan.io/reference/

# A word on bonding modes
https://www.kernel.org/doc/Documentation/networking/bonding.txt
https://www.ibm.com/docs/en/aix/7.1?topic=configuration-ieee-8023ad-link-aggregation-troubleshooting

# Unifi CLI reference
https://dl.ubnt.com/guides/edgemax/EdgeSwitch_CLI_Command_Reference_UG.pdf

# And this reddit post helped.
https://www.reddit.com/r/Ubiquiti/comments/hrbe9k/unifi_switch_port_channel_configuration/

## comands needed to be run on the unifi switch to enable the layer 3-4 load-balance
```
    ssh terrac@192.168.1.223
    telnet localhost
    enable
    configure
    show port-channel 3/1
    show port-channel 3/2
    show port-channel 3/3
    show port-channel 3/4
    port-channel load-balance 6 all
    show port-channel 3/1
    show port-channel 3/2
    show port-channel 3/3
    show port-channel 3/4
    exit
    write memory
```