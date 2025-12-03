# Proxmox Repository Configuration

Configures Proxmox hosts with non-subscription repositories and removes the subscription warning.

---

## Quick Reference

| Setting | Value |
|---------|-------|
| Hosts | proxmox group (node005, node006) |
| Vault Required | No |
| Proxmox Version | 8.x (Bookworm) |
| Ceph Version | Reef (no-subscription) |

---

## What It Does

1. Downloads Proxmox GPG key
2. Configures non-subscription repositories (Debian + PVE)
3. Disables enterprise repositories (PVE + Ceph)
4. Adds Ceph Reef no-subscription repository
5. Removes subscription warning from web UI
6. Updates apt cache

---

## Usage

```bash
# Apply to all Proxmox hosts (no vault required)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/proxmox_repos.yaml

# Apply to specific host
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/proxmox_repos.yaml -l node006

# Dry run
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/proxmox_repos.yaml --check
```

## When to Use

- **After fresh Proxmox installation** - Configure repositories before installing packages
- **After Proxmox upgrades** - May need to reapply subscription warning patch
- **When repository errors occur** - Fix common apt errors related to enterprise repos

## Idempotency

✅ Safe to run multiple times:

- Only updates files if changes are needed
- Backs up `proxmoxlib.js` before first modification
- Checks if subscription warning is already patched
- Only restarts `pveproxy` service if patch is applied

## Target Hosts

Applies to hosts in the `proxmox` inventory group:

- `node005` (Dell R620)
- `node006` (Dell R720)

## Common Issues Fixed

### 401 Unauthorized for Enterprise Repos

```text
E:Failed to fetch https://enterprise.proxmox.com/debian/ceph-quincy/dists/bookworm/InRelease  401  Unauthorized
```

**Fix**: Disables enterprise repos and enables no-subscription repos

### Missing GPG Keys

```text
W:GPG error: http://download.proxmox.com/debian/pve bookworm InRelease: The following signatures couldn't be verified because the public key is not available
```

**Fix**: Downloads and installs official Proxmox GPG key

### Subscription Nag Screen

Pop-up warning on every Proxmox web UI login about missing subscription.

**Fix**: Patches JavaScript to remove the warning dialog

## Files Modified

- `/etc/apt/trusted.gpg.d/proxmox-release-bookworm.gpg` - GPG key
- `/etc/apt/sources.list` - Main repository configuration
- `/etc/apt/sources.list.d/pve-enterprise.list` - Disabled enterprise PVE
- `/etc/apt/sources.list.d/ceph.list` - Ceph Reef no-subscription
- `/usr/share/javascript/proxmox-widget-toolkit/proxmoxlib.js` - UI patch
- `/usr/share/javascript/proxmox-widget-toolkit/proxmoxlib.js.bak` - Backup

## Post-Installation

After running this playbook, you can safely:

```bash
apt-get update
apt-get dist-upgrade
```

## Related Documentation

- [Proxmox Package Repositories](https://pve.proxmox.com/wiki/Package_Repositories)
- [readme_proxmox.md](../../../readme_proxmox.md) - Original manual instructions
