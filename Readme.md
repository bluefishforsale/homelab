# Homelab


## notes about LACP 802.3ad transmit hash etc.
The US-16-XG 10G needs to have the port-channels hash transmit modes changed via ssh

Netplan reference
https://netplan.io/reference/

A work on bonding modes
https://www.kernel.org/doc/Documentation/networking/bonding.txt
https://www.ibm.com/docs/en/aix/7.1?topic=configuration-ieee-8023ad-link-aggregation-troubleshooting

Unifi CLI reference
https://dl.ubnt.com/guides/edgemax/EdgeSwitch_CLI_Command_Reference_UG.pdf

And this reddit post helped.
https://www.reddit.com/r/Ubiquiti/comments/hrbe9k/unifi_switch_port_channel_configuration/
### here's the actual commands run to log in see settings and change them.
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
y
```