# Proxmox Repository Configuration Playbook

## Purpose

Configures Proxmox hosts with non-subscription repositories and removes the subscription warning from the web UI. This playbook should be run after fresh Proxmox installations.

## What It Does

1. **Downloads Proxmox GPG Key** - Ensures package verification
2. **Configures Non-Subscription Repositories**:
   - Debian Bookworm main and contrib
   - Debian security updates
   - Proxmox PVE no-subscription repository
3. **Disables Enterprise Repositories**:
   - Comments out enterprise PVE repository
   - Comments out enterprise Ceph repository
4. **Adds Ceph Reef Repository** - No-subscription Ceph Reef for storage
5. **Removes Subscription Warning** - Patches Proxmox UI to remove nag screen
6. **Updates Package Cache** - Ensures repositories are ready to use

## Usage

```bash
# Apply to all Proxmox hosts
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/infrastructure/proxmox_repos.yaml

# Apply to specific Proxmox host
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/infrastructure/proxmox_repos.yaml -l node006

# Dry run to see what would change
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/infrastructure/proxmox_repos.yaml --check
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
