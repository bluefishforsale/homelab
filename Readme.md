# Bluefishforsale Homelab

- Creates a proxmox VE cluster
- VM for bind and DHCPd
    -  192.168.1.2/32
- VM for Pi-Hole
    -  192.168.1.9/32
- 6 kubernetes EFI VMs
- kubernetes cluster with the following network
    - cluster cidr: 10.0.0.0/16
    - nodes cidr: 10.0.${node_number}.0/16
    - serives cidr: 10.0.250.0/20
    - api_server: 192.168.1.99/32
    - uses kube-proxy


## Start in the ansible/ directory for all this

0. todo: write instructions for creating ceph-lvm and cephfs

1. create the vms using [readme_proxmox](readme_proxmox) instrucitons

2. proceed with 3. when uptime returns w.o password on all VMs

    ```bash
    ansible -i inventory.ini k8s  -b -a 'uptime'
    ```

3. proceed with step 4. only when post-install and reboot completes

    ```bash
    ansible-playbook -i inventory.ini -l k8s playbook_proxmox_vm_post_install.yaml
    ansible-playbook -i inventory.ini -l k8s playbook_base_unattended_upgrade.yaml
    ansible-playbook -i inventory.ini -l k8s playbook_base_packages_host_settings.yaml
    ansible-playbook -i inventory.ini -l k8s playbook_core_net_qdisc.yaml
    ansible-playbook -i inventory.ini -l k8s playbook_base_users.yaml
    ansible -i inventory.ini k8s  -b -a 'reboot'
    ```

4. proceed with step 5. when ansible playbooks all apply whithout any error

    ```bash
    ls -1 playbook_kube_* | xargs -n1 -I% ansible-playbook -i inventory.ini  %
    ```

5. proceed to kube networking, ceph, then pods only when Kube clsuter is deployed correctly

    ```bash
    https://github.com/bluefishforsale/homelab-kube/blob/master/Readme-proxmox.md
    ```

### notes about LACP 802.3ad transmit hash etc

  - The US-16-XG 10G needs to have the port-channels hash transmit modes changed via ssh

### Bonding interfaces

  - https://www.kernel.org/doc/Documentation/networking/bonding.txt
  - https://www.ibm.com/docs/en/aix/7.1?topic=configuration-ieee-8023ad-link-aggregation-troubleshooting

### Unifi CLI reference

  - https://dl.ubnt.com/guides/edgemax/EdgeSwitch_CLI_Command_Reference_UG.pdf

### And this reddit post helped

  - https://www.reddit.com/r/Ubiquiti/comments/hrbe9k/unifi_switch_port_channel_configuration/

## comands needed to be run on the unifi switch to enable the layer 3-4 load-balance

### Do this for unifi switches with aggregation ports

    ```bash
    ssh terrac@192.168.1.$IP
    telnet localhost
    enable
    configure
    show port-channel all
    port-channel load-balance 6 (slot/port  or all)
    exit
    write memory
    ```

## AP inform controller

    ```bash
    ssh ubnt@<AP-IP>
    ```
You will be prompted for a username and password. The default username is usually "ubnt," and the default password is also "ubnt."

Once you're connected, run the "Set-Inform" command with the appropriate controller URL:

plaintext
Copy code
set-inform http://<controller-IP>:8080/inform