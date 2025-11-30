# UniFi Network Operations

UniFi switch and access point configuration for the homelab.

---

## Quick Reference

| Device | Purpose |
|--------|---------|
| US-16-XG | 10G aggregation switch |
| USW switches | Access layer switches |
| UAP APs | Wireless access points |
| Controller | UniFi Network Application |

---

## Switch LACP Configuration

The US-16-XG 10G switch requires manual SSH configuration for port-channel transmit hash modes.

### Configure Layer 3-4 Load Balancing

```bash
# SSH to switch
ssh terrac@192.168.1.<SWITCH_IP>

# Access CLI
telnet localhost

# Enter privileged mode
enable

# Enter config mode
configure

# View current port-channel config
show port-channel all

# Set layer 3-4 hash (includes IP + port for better distribution)
# Options: 1=src-mac, 2=dst-mac, 3=src-dst-mac, 4=src-ip, 5=dst-ip, 6=src-dst-ip-port
port-channel load-balance 6

# Or for specific port
port-channel load-balance 6 0/1

# Exit and save
exit
write memory
```

### Verify LACP Status

```bash
# Show aggregation status
show port-channel all

# Show specific port-channel
show port-channel 1

# Show LACP partner info
show lacp 1 partner

# Show interface counters
show interface 0/1
```

---

## Access Point Configuration

### Set Controller Inform URL

```bash
# SSH to AP (default credentials: ubnt/ubnt)
ssh ubnt@<AP-IP>

# Set controller URL
set-inform http://<controller-IP>:8080/inform

# For adopted APs, may need to force re-adoption
set-inform http://<controller-IP>:8080/inform
```

### AP Debugging

```bash
# Check AP status
info

# Check wireless clients
sta

# Check connection to controller
mca-status

# View AP logs
cat /var/log/messages

# Restart AP services
syswrapper.sh restart
```

---

## Switch Debugging

### General Status

```bash
# Show system info
show version

# Show running config
show running-config

# Show interface status
show interface status

# Show MAC address table
show mac-address-table

# Show spanning tree
show spanning-tree
```

### Port Channel Troubleshooting

```bash
# Show port-channel summary
show port-channel all

# Show LACP statistics
show lacp 1 counters

# Clear LACP counters
clear lacp 1 counters

# Show interface errors
show interface 0/1 | include error
```

### VLAN Configuration

```bash
# Show VLAN database
show vlan

# Show VLAN on port
show vlan port 0/1
```

---

## Linux Bond Configuration

For hosts connecting to LACP port-channels:

### Verify Bond Status

```bash
# Check bond status
cat /proc/net/bonding/bond0

# Check interface states
ip link show bond0
ip link show eth0
ip link show eth1

# Check bond mode (should be 802.3ad)
cat /sys/class/net/bond0/bonding/mode
```

### Bond Troubleshooting

```bash
# Check LACP rate
cat /sys/class/net/bond0/bonding/lacp_rate

# Check xmit hash policy
cat /sys/class/net/bond0/bonding/xmit_hash_policy

# View bond statistics
cat /proc/net/bonding/bond0 | grep -A 5 "Slave Interface"

# Test failover (remove interface)
echo -eth0 > /sys/class/net/bond0/bonding/slaves
echo +eth0 > /sys/class/net/bond0/bonding/slaves
```

---

## Monitoring with UnPoller

UniFi metrics are collected via UnPoller and exported to Prometheus.

### Deploy UnPoller

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/monitoring/unpoller.yaml --ask-vault-pass
```

### Verify UnPoller

```bash
# Check container status
ssh terrac@192.168.1.143 "docker ps | grep unpoller"

# Check metrics endpoint
curl -s http://192.168.1.143:9130/metrics | head -20

# Check logs
ssh terrac@192.168.1.143 "docker logs unpoller --tail 50"
```

### Prometheus Target

UnPoller exposes metrics at `http://192.168.1.143:9130/metrics` for Prometheus scraping.

---

## References

- [Kernel Bonding Documentation](https://www.kernel.org/doc/Documentation/networking/bonding.rst)
- [IEEE 802.3ad Link Aggregation](https://en.wikipedia.org/wiki/Link_aggregation)
- [UniFi CLI Reference](https://help.ui.com/hc/en-us/articles/205202580-UniFi-Switch-CLI-Commands)
- [UnPoller Documentation](https://unpoller.com/)