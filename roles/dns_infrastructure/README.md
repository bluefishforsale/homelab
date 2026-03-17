# DNS Infrastructure HA Role

Ansible role for deploying PowerDNS + Kea DHCP + netboot.xyz stack with HA hot-standby failover.

## Features

- **PowerDNS Authoritative DNS** with AXFR zone replication
- **Kea DHCP4** with hot-standby HA failover
- **Kea DHCP-DDNS** for dynamic DNS updates
- **netboot.xyz** for PXE/TFTP boot
- **Prometheus metrics** via kea-exporter and PowerDNS API
- **Loki log aggregation** via Promtail

## HA Architecture

### Kea DHCP Hot-Standby
- Primary server (dns02): Active DHCP, handles all requests
- Standby server (dns01): Passive, takes over on primary failure
- Lease database synchronization via HA hooks
- Automatic failover with heartbeat monitoring

### PowerDNS Zone Replication
- Primary (dns02): Master zone authority
- Standby (dns01): AXFR secondary with NOTIFY
- Both servers authoritative for all zones

## Role Variables

See `defaults/main.yaml` for all available variables.

**Key variables:**
- `ha_role`: `primary` or `standby`
- `ha_peer_ip`: IP address of HA peer
- `dns_home`: Base directory for DNS stack (default: `/opt/dns-stack`)
- `enable_dhcp`: Enable Kea DHCP (default: `true`)
- `enable_pxe`: Enable netboot.xyz (default: `true`)
- `enable_monitoring`: Enable Promtail/exporters (default: `true`)

## Dependencies

- Docker CE
- docker-compose-plugin
- Debian 12 (Bookworm)

## Example Playbook

```yaml
- name: Deploy HA DNS/DHCP Stack
  hosts: dns_servers
  become: true
  
  roles:
    - role: dns_infrastructure
      vars:
        ha_role: "{{ 'primary' if inventory_hostname == 'dns02' else 'standby' }}"
        ha_peer_ip: "{{ hostvars[groups['dns_servers'] | difference([inventory_hostname]) | first]['ansible_host'] }}"
```

## Tags

- `setup`: Initial setup (Docker, directories, configs)
- `dns`: PowerDNS deployment
- `dhcp`: Kea DHCP deployment
- `pxe`: netboot.xyz deployment
- `monitoring`: Promtail/exporters deployment
- `deploy`: Start/restart services

## License

MIT

## Author

Homelab Infrastructure Team
