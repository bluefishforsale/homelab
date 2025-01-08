# Additional Configuration Notes
### LACP 802.3ad Transmit Hash

The US-16-XG 10G switch requires manual configuration of port-channels' transmit hash modes via SSH.

### Bonding Interfaces

- [Kernel Documentation on Ethernet Bonding](https://www.kernel.org/doc/Documentation/networking/bonding.rst)

IBM Documentation on IEEE 802.3ad Link Aggregation

### Unifi CLI Reference

Unifi Switch CLI Command Reference Guide

### Unifi Switch Aggregation Port Configuration

For layer 3-4 load balancing, follow these commands on Unifi switches with aggregation ports:

```bash
ssh terrac@192.168.1.$IP
telnet localhost
enable
configure
show port-channel all
port-channel load-balance 6 (slot/port or all)
exit
write memory
```

### AP Inform Controller

To set up the AP inform URL, SSH into the AP and run:

```bash
ssh ubnt@<AP-IP>
set-inform http://<controller-IP>:8080/inform
```