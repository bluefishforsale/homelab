# fail2ban SSH Protection

SSH brute force protection using fail2ban.

---

## Quick Reference

| Setting | Value |
|---------|-------|
| ignoreip | 127.0.0.1/8, ::1, 192.168.1.0/24 |
| maxretry | 3 attempts |
| findtime | 120 seconds (2 minutes) |
| bantime | 86400 seconds (24 hours) |
| logpath | /var/log/auth.log |
| banaction | iptables-multiport |

**Rule**: 3 failed SSH attempts within 2 minutes = banned for 24 hours.

**Local LAN Exception**: 192.168.1.0/24 is never banned.

---

## Deployment

```bash
# Deploy to all hosts (no vault required)
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/base/fail2ban.yaml

# Deploy to specific host
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/base/fail2ban.yaml -l ocean
```

## Verification

```bash
# Check fail2ban status
sudo fail2ban-client status

# Check sshd jail status
sudo fail2ban-client status sshd

# View banned IPs
sudo fail2ban-client get sshd banned

# Check iptables rules
sudo iptables -L f2b-sshd -n -v

# View fail2ban logs
sudo tail -f /var/log/fail2ban.log

# View auth attempts
sudo journalctl -u sshd -f
```

## Manual Operations

```bash
# Manually ban an IP
sudo fail2ban-client set sshd banip 1.2.3.4

# Manually unban an IP
sudo fail2ban-client set sshd unbanip 1.2.3.4

# Reload configuration
sudo fail2ban-client reload
```

## Expected Behavior

With the aggressive configuration:

- Single IP making 3+ connection attempts in 2 minutes → BANNED for 24 hours
- Logs will show: `[sshd] Ban 1.2.3.4`
- iptables will have DROP rules in f2b-sshd chain
- `fail2ban-client get sshd banned` will show active bans

## Notes

- Configuration is SRE-level resilient and idempotent
- Safe to run multiple times without side effects
- Works with systemd journal and /var/log/auth.log
- Compatible with all Debian-based systems in homelab
