# Deprecated Playbooks

This directory contains playbooks that have been deprecated but are preserved for emergency rollback scenarios.

## ⚠️ Important

**DO NOT use these playbooks in normal operations.** They are kept only for emergency situations where the replacement infrastructure fails and immediate rollback is required.

---

## ISC DHCP Server

**File:** `dhcp_ddns_isc.yaml`  
**Deprecated:** March 2026  
**Replaced By:** Kea DHCP on dns02 (`playbooks/individual/core/services/dns02.yaml`)

### Why Deprecated

ISC DHCP has been replaced by Kea DHCP as part of the DNS/DHCP infrastructure modernization:

- **Old Stack (dns01):** ISC BIND9 + ISC DHCP
- **New Stack (dns02):** PowerDNS + Kea DHCP + Kea DHCP-DDNS

### Replacement Features

- Modern REST API for DHCP management
- Better DDNS integration with PowerDNS
- Prometheus metrics and monitoring
- Docker-based deployment with systemd management
- Comprehensive logging to Loki

### Emergency Rollback

If Kea DHCP fails and immediate rollback is required:

```bash
# 1. Stop new DHCP service on dns02
ssh dns02 'systemctl stop dns-stack.service'

# 2. Deploy ISC DHCP on dns01
ansible-playbook -i inventories/production/hosts.ini playbooks/deprecated/dhcp_ddns_isc.yaml

# 3. Verify service
ssh dns01 'systemctl status isc-dhcp-server.service'
ssh dns01 'journalctl -u isc-dhcp-server.service -f'

# 4. Test DHCP
# Release/renew lease on a test client
```

### Configuration Files

ISC DHCP configuration files are preserved in `files/isc-dhcp-server/`:

- `/etc/dhcp/dhcpd.conf` - Main DHCP configuration
- `/etc/dhcp/kube-nodes.conf` - Kubernetes node reservations
- `/etc/default/isc-dhcp-server` - Service defaults
- `/etc/systemd/system/isc-dhcp-server.service` - Systemd unit

### Phase 2 Removal

Once dns02 has been validated in production for 90+ days with no issues:

1. Remove this playbook entirely
2. Remove `files/isc-dhcp-server/` directory
3. Remove ISC DHCP package from dns01
4. Update this README to reflect removal

---

## dns_bind9.yaml - BIND9 DNS Server

**File:** `dns_bind9.yaml`  
**Deprecated:** March 2026  
**Replaced By:** HA DNS/DHCP Stack (`playbooks/individual/core/services/dns_ha_stack.yaml`)

### Why Deprecated

BIND9 on dns01 has been replaced by PowerDNS as part of the HA DNS/DHCP infrastructure:

- **Old Stack:** BIND9 on dns01 (standalone)
- **New Stack:** PowerDNS on both dns01 and dns02 with AXFR replication

### Emergency Rollback

If the HA DNS stack fails on dns01:

```bash
# Stop HA stack on dns01
ssh dns01 'systemctl stop dns-stack.service'

# Deploy BIND9
ansible-playbook -i inventories/production/hosts.ini playbooks/deprecated/dns_bind9.yaml

# Verify BIND9
ssh dns01 'systemctl status named'
dig @192.168.1.2 ocean.home
```

---

## dns02_standalone.yaml - Standalone PowerDNS Stack

**File:** `dns02_standalone.yaml`  
**Deprecated:** March 2026  
**Replaced By:** HA DNS/DHCP Stack (`playbooks/individual/core/services/dns_ha_stack.yaml`)

### Why Deprecated

Standalone dns02 deployment has been replaced by HA deployment with Kea hot-standby:

- **Old Stack:** PowerDNS + Kea DHCP on dns02 only (standalone)
- **New Stack:** PowerDNS + Kea DHCP on both dns01 and dns02 with HA failover

### Emergency Rollback

If the HA DNS stack fails:

```bash
# Stop HA stack on both servers
ssh dns01 'systemctl stop dns-stack.service'
ssh dns02 'systemctl stop dns-stack.service'

# Deploy standalone dns02
ansible-playbook -i inventories/production/hosts.ini playbooks/deprecated/dns02_standalone.yaml

# Verify dns02
ssh dns02 'systemctl status dns-stack.service'
dig @192.168.1.3 ocean.home
```

---

## Adding Deprecated Playbooks

When deprecating a playbook:

1. **Move to this directory** with descriptive name
2. **Add deprecation header** to the playbook explaining:
   - What replaced it
   - When it was deprecated
   - Emergency usage instructions
3. **Update this README** with full context
4. **Comment out** in orchestrator playbooks (e.g., `02_core_infrastructure.yaml`)
5. **Preserve configuration files** in `files/` directory
6. **Set removal date** (typically 90 days after replacement validation)
