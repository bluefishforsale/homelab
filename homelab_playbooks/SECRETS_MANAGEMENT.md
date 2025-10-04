# Homelab Secrets Operations Guide

This document provides operational procedures for managing the consolidated secrets vault in your homelab environment.

## **Daily Operations Overview**

Your homelab uses a single encrypted `vault_secrets.yaml` file containing all service credentials, API keys, and sensitive configuration. This guide covers:
- Adding new secrets and services
- Rotating and removing credentials  
- Disaster recovery procedures
- Troubleshooting vault issues
- Backup and restore operations

## **Current Vault Structure**

The vault is organized into these categories:
- **`cloudflare`** - API credentials and zone IDs
- **`gitlab`** - Admin passwords and registration tokens  
- **`databases`** - MySQL/PostgreSQL passwords
- **`media_services`** - Plex, Sonarr, Tautulli API keys
- **`ai_services`** - N8N, Open WebUI configuration
- **`external_apis`** - DDNS, TMDB, cloud service keys
- **`infrastructure`** - SSH keys, VPN configurations
- **`monitoring`** - Grafana, Prometheus credentials

## **Adding New Secrets**

### 1. Edit the Encrypted Vault
```bash
cd homelab_playbooks
ansible-vault edit vault_secrets.yaml
```

### 2. Add New Service Section
Follow the existing structure:
```yaml
# Add new service under appropriate category
media_services:
  # Existing services...
  jellyfin:  # New service
    api_key: "your-jellyfin-api-key"
    admin_password: "secure-admin-password"
```

### 3. Update Playbook Integration
```yaml
# In your new playbook
vars_files:
  - vault_secrets.yaml
  
# Access the new secrets
- name: Configure Jellyfin
  template:
    src: jellyfin.conf.j2
    dest: /etc/jellyfin/jellyfin.conf
  vars:
    admin_pass: "{{ media_services.jellyfin.admin_password }}"
```

### 4. Test and Commit

```bash
# Test syntax
ansible-playbook new_playbook.yaml --syntax-check

# Test decryption
ansible-vault view vault_secrets.yaml | grep jellyfin

# Commit changes
git add vault_secrets.yaml new_playbook.yaml
git commit -m "feat: add Jellyfin service with encrypted credentials"
```

## **Secret Rotation and Removal**

### Rotating Expired Credentials

1. **Update the service first** (generate new API key, etc.)
2. **Edit vault with new credentials:**

```bash
ansible-vault edit vault_secrets.yaml
# Update the relevant secret value
```

3. **Deploy updated configuration:**

```bash
# Test with new credentials
ansible-playbook affected_playbook.yaml --check

# Deploy if test passes
ansible-playbook affected_playbook.yaml
```

4. **Verify service functionality** before removing old credentials

### Removing Deprecated Services

1. **Remove from playbooks first:**

```bash
# Comment out or remove service tasks
# Test that playbook still works
ansible-playbook playbook.yaml --syntax-check
```

2. **Remove from vault:**

```bash
ansible-vault edit vault_secrets.yaml
# Delete entire service section
```

3. **Clean up service data** (optional):

```bash
# Stop services, remove data directories
ansible-playbook cleanup_service.yaml
```

## **Disaster Recovery Procedures**

### Scenario 1: Lost Vault Password

**Prevention:** Always maintain password in multiple secure locations

**Recovery Options:**
1. **Check password managers** (1Password, Bitwarden, etc.)
2. **Check secure notes** in other systems
3. **Ask team members** who may have access
4. **Last resort:** Recreate vault from service UIs

### Scenario 2: Corrupted Vault File

**Immediate Steps:**

```bash
# Check git history
git log --oneline vault_secrets.yaml

# Restore from previous commit
git checkout HEAD~1 -- vault_secrets.yaml

# Test vault integrity
ansible-vault view vault_secrets.yaml
```

### Scenario 3: Compromised Credentials

**Emergency Response:**

1. **Immediately rotate all secrets:**

```bash
# Generate new API keys in all services
# Update vault with emergency credentials
ansible-vault edit vault_secrets.yaml
```

2. **Deploy emergency updates:**

```bash
# Deploy critical services first
ansible-playbook playbook_ocean_cloudflared.yaml
ansible-playbook playbook_gitlab_packages.yaml
```

3. **Audit access logs** in all services
4. **Change vault password:**

```bash
ansible-vault rekey vault_secrets.yaml
```

## **Backup and Recovery Operations**

### Regular Backup Procedures

```bash
# Daily backup (automated via cron)
cp vault_secrets.yaml ~/backups/vault_secrets_$(date +%Y%m%d).yaml

# Weekly encrypted backup to external storage
ansible-vault decrypt vault_secrets.yaml --output=/tmp/vault_plain.yaml
tar -czf ~/secure_backup/homelab_secrets_$(date +%Y%m%d).tar.gz /tmp/vault_plain.yaml
rm /tmp/vault_plain.yaml
```

### Recovery Testing

**Monthly recovery test:**

```bash
# Test vault decryption
ansible-vault view vault_secrets.yaml > /dev/null
echo "Vault decryption: $?"

# Test playbook execution
ansible-playbook playbook_ocean_n8n.yaml --check
echo "Playbook syntax: $?"
```

## **Troubleshooting Common Issues**

### Environment Variable Issues

**Problem:** `ERROR! Attempting to decrypt but no vault secrets found`

```bash
# Solution: Set vault password environment variable
echo "your-vault-password" > ~/.ansible_vault_pass
chmod 600 ~/.ansible_vault_pass
export ANSIBLE_VAULT_PASSWORD_FILE=~/.ansible_vault_pass
```

### Inventory Configuration

**Problem:** `Could not match supplied host pattern, ignoring: ocean`

```bash
# Solution: Create or update inventory.ini
cat > inventory.ini << 'EOF'
[ocean]
ocean ansible_host=192.168.1.143 ansible_user=terrac

[gitlab]
gitlab ansible_host=192.168.1.x ansible_user=your-user
EOF
```

### Variable Access Patterns

**Use dot notation for vault variables:**

```yaml
# Correct access patterns
cloudflare_token: "{{ cloudflare.api_token }}"
gitlab_password: "{{ gitlab.root_password }}"
db_password: "{{ databases.mysql.root_password }}"
api_key: "{{ media_services.plex.api_key }}"
```

## **Daily Operational Commands**

### Quick Status Checks

```bash
# Verify vault integrity
ansible-vault view vault_secrets.yaml | head -5

# Test key playbooks
ansible-playbook -i inventory.ini playbook_ocean_n8n.yaml --check
ansible-playbook -i inventory.ini playbook_ocean_plex.yaml --check
```

### Emergency Access

```bash
# Quick service restart with current secrets
ansible-playbook -i inventory.ini playbook_ocean_plex.yaml --tags restart

# Emergency credential rotation
ansible-vault edit vault_secrets.yaml
# Update compromised credential
ansible-playbook -i inventory.ini affected_playbook.yaml
```

### Service Health Monitoring

```bash
# Check all ocean services
ansible-playbook -i inventory.ini playbook_ocean_*.yaml --check --list-tasks

# Verify Cloudflare tunnel status
ansible-playbook -i inventory.ini playbook_ocean_cloudflared.yaml --tags health-check
```

## **Compliance and Security Auditing**

### Monthly Security Review

```bash
# Check vault file permissions
ls -la vault_secrets.yaml
# Should show: -rw------- (600 permissions)

# Audit vault password storage
ls -la ~/.ansible_vault_pass
# Should show: -rw------- (600 permissions)

# Review git history for plain-text secrets
git log --all --full-history -- vault_secrets.yaml
```

### Access Audit Trail

```bash
# Check who has accessed vault recently
sudo ausearch -f vault_secrets.yaml -ts recent

# Review successful deployments
grep "vault_secrets.yaml" ~/.bash_history | tail -10
```

## **Advanced Operations**

### Vault File Management

```bash
# View specific secrets without full decryption
ansible-vault view vault_secrets.yaml | yq '.cloudflare.api_token'

# Compare vault versions
git show HEAD~1:vault_secrets.yaml > /tmp/old_vault
ansible-vault decrypt /tmp/old_vault
ansible-vault view vault_secrets.yaml > /tmp/new_vault
diff /tmp/old_vault /tmp/new_vault

# Batch update multiple secrets
ansible-vault edit vault_secrets.yaml
# Use editor find/replace for bulk changes
```

### Multi-Environment Support

```bash
# Production vs Development vaults
cp vault_secrets.yaml vault_secrets_prod.yaml
cp vault_secrets.yaml vault_secrets_dev.yaml

# Use environment-specific vaults
ansible-playbook -e vault_file=vault_secrets_prod.yaml playbook.yaml
```

## **Automation and CI/CD Integration**

### Automated Deployment Pipeline

```yaml
# .gitlab-ci.yml or .github/workflows/deploy.yml
stages:
  - validate
  - deploy

validate_secrets:
  script:
    - echo "$ANSIBLE_VAULT_PASSWORD" > /tmp/vault_pass
    - ansible-vault view vault_secrets.yaml --vault-password-file /tmp/vault_pass > /dev/null
    - ansible-playbook playbook_ocean_n8n.yaml --syntax-check --vault-password-file /tmp/vault_pass

deploy_services:
  script:
    - echo "$ANSIBLE_VAULT_PASSWORD" > /tmp/vault_pass
    - ansible-playbook -i inventory.ini playbook_ocean_n8n.yaml --vault-password-file /tmp/vault_pass
  only:
    - main
```

### Scheduled Maintenance

```bash
# Weekly credential health check (cron)
0 2 * * 0 /home/user/homelab/check_credentials.sh

# Monthly backup rotation
0 3 1 * * /home/user/homelab/backup_vault.sh
```

## **Emergency Procedures Quick Reference**

### Service Down - Credential Issues

1. **Check vault access:** `ansible-vault view vault_secrets.yaml`
2. **Verify service config:** `ansible-playbook service_playbook.yaml --check`
3. **Check service logs:** `docker logs service_container`
4. **Rotate credentials if needed:** `ansible-vault edit vault_secrets.yaml`
5. **Redeploy:** `ansible-playbook service_playbook.yaml`

### Vault Corruption Recovery

1. **Stop all deployments immediately**
2. **Check git history:** `git log --oneline vault_secrets.yaml`
3. **Restore from backup:** `git checkout HEAD~1 -- vault_secrets.yaml`
4. **Test vault:** `ansible-vault view vault_secrets.yaml`
5. **Validate services:** Run syntax checks on critical playbooks
6. **Resume deployments only after validation**

### Complete Infrastructure Recovery

1. **Restore from external backup**
2. **Decrypt and verify all secrets**
3. **Deploy in order:** Cloudflare → GitLab → N8N → Media services
4. **Validate each service before proceeding**
5. **Update monitoring and logging**

## **Best Practices Summary**

### Daily Operations
- ✅ **Always test with `--check` first**
- ✅ **Use environment variables for vault password**
- ✅ **Commit vault changes immediately after testing**
- ✅ **Review logs after deployments**

### Security Operations
- ✅ **Rotate credentials quarterly**
- ✅ **Audit vault access monthly**
- ✅ **Maintain encrypted backups**
- ✅ **Test disaster recovery procedures**

### Emergency Response
- ✅ **Stop deployments first during incidents**
- ✅ **Verify vault integrity before proceeding**
- ✅ **Document all emergency changes**
- ✅ **Review and improve procedures post-incident**

---

**Last Updated:** $(date +%Y-%m-%d)  
**Vault Version:** Check `git log --oneline vault_secrets.yaml | head -1`  
**Emergency Contact:** Your homelab administrator

> **Note:** This operations guide follows the idempotency principles from your homelab memories - all procedures are designed to be safe to run multiple times without causing unwanted changes or side effects.
