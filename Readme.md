# Homelab


## notes about LACP 802.3ad transmit hash etc.
The US-16-XG 10G needs to have the port-channels hash transmit modes changed via ssh
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