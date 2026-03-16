# DNS Stack (PowerDNS + Kea DHCP + netboot.xyz)

Replaces ISC DHCP + BIND9 on dns01 with a modern, HA-capable stack.

## Components

| Service | Image | Ports | Purpose |
|---------|-------|-------|---------|
| PowerDNS Auth | `powerdns/pdns-auth-49` | 53/tcp+udp, 8081/tcp | Authoritative DNS + API + Prometheus |
| Kea DHCP4 | `docker.cloudsmith.io/isc/docker/kea-dhcp4:2.6` | 67/udp (host net) | DHCP server with PXE support |
| Kea DHCP-DDNS | `docker.cloudsmith.io/isc/docker/kea-dhcp-ddns:2.6` | 53001/udp (internal) | RFC 2136 DNS updates to PowerDNS |
| Kea Ctrl Agent | `docker.cloudsmith.io/isc/docker/kea-ctrl-agent:2.6` | 8000/tcp (host net) | REST API for Kea management |
| netboot.xyz | `ghcr.io/netbootxyz/netbootxyz` | 69/udp, 8070/tcp, 3001/tcp | PXE/TFTP boot menus + Web UI |

## Deployment

```bash
# Full deploy
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/core/services/dns02.yaml

# Import/refresh zone data only
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/core/services/dns02.yaml --tags zones

# Validate DNS responses
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/core/services/dns02.yaml --tags validate

# Ad-hoc parity check from local machine
./scripts/dns-parity-check.sh --old 192.168.1.2 --new 192.168.1.3
```

## Directory Layout (on dns02)

```
/opt/dns-stack/
├── docker-compose.yml
├── import-zones.sh
├── config/
│   ├── pdns.conf
│   ├── kea-dhcp4.conf
│   ├── kea-dhcp-ddns.conf
│   └── kea-ctrl-agent.conf
└── data/
    ├── pdns/          # SQLite database
    ├── kea/           # DHCP lease files
    └── netboot/
        ├── config/    # netboot.xyz configuration
        └── assets/    # Downloaded boot images
```

## PXE Boot Flow

1. Client PXE ROM broadcasts DHCP discover
2. Kea DHCP offers IP + next-server + boot-file based on client architecture:
   - BIOS → `undionly.kpxe` via TFTP
   - UEFI → `ipxe.efi` via TFTP
   - Already iPXE → HTTP menu URL
3. iPXE chainloads to netboot.xyz HTTP menu
4. User selects from GRUB-like menu (live images, installers, utilities)
5. Selected image downloads via HTTP and boots

## DDNS Flow

1. Client gets DHCP lease from Kea
2. Kea forwards DDNS request to kea-dhcp-ddns daemon
3. kea-dhcp-ddns sends RFC 2136 DNS UPDATE to PowerDNS (authenticated via TSIG key)
4. PowerDNS updates forward (`home.`) and reverse (`1.168.192.in-addr.arpa`) zones

## Vault Secrets Required

```yaml
infrastructure:
  dns_stack:
    pdns_api_key: "<openssl rand -hex 32>"
    tsig_key: "<python3 -c 'import base64,os; print(base64.b64encode(os.urandom(32)).decode())'>"
    kea_api_password: "<openssl rand -hex 16>"
```

## Monitoring

- **PowerDNS metrics:** `http://192.168.1.3:8081/metrics` (Prometheus)
- **Kea API:** `http://192.168.1.3:8000/` (REST, basic auth)
- **netboot.xyz Web UI:** `http://192.168.1.3:3001/`
- **Logs:** All containers use journald driver → `journalctl -u dns-stack`

## Phase 2: HA

After validation, rebuild dns01 with the same stack and configure:
- Kea HA hot-standby (dns02=primary, dns01=standby)
- PowerDNS AXFR zone replication
- keepalived VIP floating between nodes
