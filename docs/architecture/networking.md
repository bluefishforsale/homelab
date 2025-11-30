# Network Architecture

Homelab network configuration and topology.

---

## Quick Reference

| Host | IP | Purpose |
|------|----|---------|
| Gateway | 192.168.1.1 | Router |
| dns01 | 192.168.1.2 | BIND DNS |
| gitlab | 192.168.1.5 | CI/CD |
| pihole | 192.168.1.9 | DNS filtering |
| node005 | 192.168.1.105 | Proxmox host |
| node006 | 192.168.1.106 | Proxmox host |
| ocean | 192.168.1.143 | Docker services |
| gh-runner-01 | 192.168.1.250 | GitHub runners |

---

## Network Topology

```text
Internet
    │
    ▼
Router (192.168.1.1)
    │
    ▼
UniFi US-16-XG (10GbE)
    │
    ├── node005 (bond0) ──► dns01, pihole, gitlab, gh-runner-01
    │
    └── node006 (bond0) ──► ocean
```

---

## Subnet: 192.168.1.0/24

Single flat network for all hosts and services.

| Range | Purpose |
|-------|---------|
| .1 | Gateway |
| .2-.50 | Infrastructure VMs |
| .100-.110 | Proxmox hosts |
| .143 | Ocean services |
| .200-.250 | DHCP pool |

---

## Proxmox Host Network

### Bond Configuration (LACP)

```bash
auto bond0
iface bond0 inet manual
    bond-slaves eth0 eth1
    bond-miimon 100
    bond-mode 802.3ad
    bond-xmit-hash-policy layer3+4
```

### Bridge Configuration

```bash
auto vmbr0
iface vmbr0 inet static
    address 192.168.1.106/24
    gateway 192.168.1.1
    bridge-ports bond0
    bridge-stp off
    bridge-fd 0
```

See [unifi.md](/docs/operations/unifi.md) for switch LACP configuration.

---

## DNS

### Internal DNS (BIND)

dns01 (192.168.1.2) serves `.home` domain for internal services.

```bash
# Test resolution
nslookup ocean.home 192.168.1.2
```

### External DNS (Cloudflare)

External access via `*.terrac.com` through Cloudflare tunnels.

Deploy DDNS updater:

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/network/cloudflare_ddns.yaml --ask-vault-pass
```

---

## External Access

### Cloudflare Tunnels

Services exposed via cloudflared tunnel (no port forwarding required).

```bash
# Deploy tunnels
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/network/cloudflared.yaml --ask-vault-pass

# Check tunnel status
ssh terrac@192.168.1.143 "docker logs cloudflared --tail 20"
```

### Traffic Flow

```text
Internet → Cloudflare → cloudflared → nginx → service
```

---

## Troubleshooting

### Diagnostics

```bash
# Check interfaces
ip link show
ip addr show

# Check bond status
cat /proc/net/bonding/bond0

# Test connectivity
ping -c 4 192.168.1.1
traceroute google.com

# DNS resolution
nslookup ocean.home 192.168.1.2
```

### Performance Testing

```bash
# iperf3 between hosts
iperf3 -s                      # Server
iperf3 -c 192.168.1.143        # Client
```

---

## Related Documentation

- [UniFi Operations](/docs/operations/unifi.md) - Switch configuration
- [Architecture Overview](overview.md) - System architecture
