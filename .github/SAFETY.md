# GitHub Actions Safety & Protection Guide

## 🛡️ Critical Infrastructure Protection

This repository has **MAXIMUM SAFETY** protections for critical infrastructure.

### 🔴 CRITICAL SERVICES (Protected)

These services require special workflow with mandatory approval:

- **DNS** (192.168.1.2) - Infrastructure foundation
- **DHCP** (192.168.1.2) - Network address management  
- **Plex** (192.168.1.143:32400) - Primary media service

**To deploy critical services:**
→ Use workflow: `Deploy Critical Service (Protected)`
→ **Mandatory approval required**
→ **Mandatory health checks** before & after
→ **Automatic config backups** before deployment

### ⚠️ PROTECTED HOSTS

These hosts are **NEVER targeted** by automated deployments:

- **ocean** (192.168.1.143) - Primary service host
- **node005** (192.168.1.X) - Secondary infrastructure
- **dns01** (192.168.1.2) - DNS/DHCP server

**Deployments to these hosts require:**
1. ✅ Syntax validation
2. ✅ Mandatory dry-run
3. ✅ Manual approval (for critical services)
4. ✅ Pre-deployment health check
5. ✅ Configuration backup
6. ✅ Post-deployment verification

## 🔒 Safety Layers

### Layer 1: Workflow Separation

**Three separate workflows with different protections:**

1. **`ci-validate.yml`** - Automatic validation only
   - Runs on every push/PR
   - No deployment capability
   - Read-only operations

2. **`deploy-ocean-service.yml`** - Standard services
   - EXCLUDES critical services (DNS, DHCP, Plex)
   - Requires: validation → dry-run → deploy
   - For: nginx, media stack (non-Plex), AI services, monitoring

3. **`deploy-critical-service.yml`** - Protected services
   - ONLY for DNS, DHCP, Plex
   - Requires: validation → dry-run → **manual approval** → deploy
   - Includes pre/post health checks
   - Creates configuration backups

### Layer 2: Mandatory Gates

**Every deployment must pass:**

```
┌─────────────────┐
│ Syntax Validate │ ← YAML + Ansible syntax
└────────┬────────┘
         │ PASS ✓
         ↓
┌─────────────────┐
│ Ansible Lint    │ ← Style & best practices
└────────┬────────┘
         │ PASS ✓
         ↓
┌─────────────────┐
│ Dry-Run (--check) │ ← Full execution simulation
└────────┬────────┘
         │ PASS ✓
         ↓
┌─────────────────┐
│ [Manual Approval]│ ← Only for critical services
└────────┬────────┘
         │ APPROVED
         ↓
┌─────────────────┐
│ Deploy          │ ← Actual changes
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│ Health Check    │ ← Verify service operational
└─────────────────┘
```

**If ANY gate fails → Deployment STOPS**

### Layer 3: Fail-Fast Enforcement

All deployment scripts use:

```bash
set -e              # Exit on ANY error
set -o pipefail     # Catch errors in pipes
EXIT_CODE=$?
if [ $EXIT_CODE -ne 0 ]; then
  echo "❌ FAILED"
  exit $EXIT_CODE
fi
```

**No silent failures possible**

### Layer 4: Health Checks

**Critical services only:**

**Pre-deployment:**
- Service must be healthy BEFORE deployment starts
- If unhealthy → Deployment BLOCKED

**Post-deployment:**
- Service must respond within timeout
- Retries with exponential backoff
- If health check fails → **ALERT + Rollback instructions**

**Example: DNS health check**
```bash
# Must resolve both internal and external
dig @192.168.1.2 ocean.home +short || FAIL
dig @192.168.1.2 google.com +short || FAIL
```

### Layer 5: Configuration Backups

**Critical services only:**

Before ANY change:
```bash
/tmp/bind9-backup-20241117-140523.tar.gz
/tmp/dhcp-backup-20241117-140523.tar.gz
/tmp/plex-config-snapshot-20241117-140523.txt
```

Rollback instructions provided if deployment fails.

## 🚫 What CANNOT Happen

Due to safety layers, these scenarios are **IMPOSSIBLE**:

❌ Deploy critical service without dry-run
❌ Deploy critical service without approval
❌ Deploy if validation fails
❌ Deploy if health check fails
❌ Silent failures (all errors halt deployment)
❌ Accidentally deploy to wrong service
❌ Deploy DNS/DHCP/Plex via standard workflow

## ✅ Safe Operations

These operations are safe for automation:

### Automatic (No Human Required)
- ✅ Syntax validation (CI)
- ✅ Linting (CI)
- ✅ Dry-runs (pre-deployment)

### Manual Trigger Required
- ⚠️ Standard service deployment (nginx, sonarr, etc.)
- 🔴 Critical service deployment (DNS, DHCP, Plex)

### Manual Approval Required
- 🔴 Critical service deployment only

## 📋 Deployment Checklist

### For Standard Services

1. ✅ Go to Actions → `Deploy Ocean Service`
2. ✅ Select service (non-critical)
3. ✅ Click "Run workflow"
4. ✅ Wait for validation → dry-run → deploy
5. ✅ Monitor logs for success

**Abort if:**
- Validation fails
- Dry-run shows unexpected changes
- Health concerns arise

### For Critical Services

1. 🔴 Go to Actions → `Deploy Critical Service (Protected)`
2. 🔴 Select service (DNS/DHCP/Plex)
3. 🔴 Ensure approval checkbox enabled
4. 🔴 Click "Run workflow"
5. 🔴 Review validation logs
6. 🔴 Review dry-run logs **CAREFULLY**
7. 🔴 **DECISION POINT**: Approve or reject
8. 🔴 If approved → Monitor health checks
9. 🔴 Verify service operational

**Abort if:**
- Any validation fails
- Dry-run shows unexpected changes
- Uncertain about changes
- Services currently unstable
- During peak usage hours

## 🚨 Emergency Procedures

### If Deployment Fails

1. **Check GitHub Actions logs** for error details
2. **Verify service status** manually:
   ```bash
   # DNS
   dig @192.168.1.2 ocean.home
   
   # DHCP
   ssh dns01 "systemctl status isc-dhcp-server"
   
   # Plex
   curl http://192.168.1.143:32400/web/index.html
   ```

3. **Locate backup** (critical services):
   ```bash
   ssh dns01 "ls -lh /tmp/*-backup-*.tar.gz"
   ```

4. **Rollback if needed**:
   ```bash
   # Example for DNS
   ssh dns01 "tar -xzf /tmp/bind9-backup-*.tar.gz -C /"
   ssh dns01 "systemctl restart bind9"
   ```

### If Service Becomes Unhealthy

1. **DO NOT deploy again** until investigated
2. Check service logs
3. Review recent changes
4. Restore from backup if needed
5. Fix root cause before retry

## 🔐 Security Considerations

### Secrets Management
- ✅ Vault password in GitHub Secrets
- ✅ Vault files encrypted in repo
- ✅ SSH keys mounted in runners (not in repo)
- ✅ Temporary vault password files cleaned up

### Access Control
- Repository access controls who can trigger workflows
- Critical services require approval
- Runners operate with restricted permissions
- No direct SSH access from workflows

### Audit Trail
- All workflow runs logged in GitHub Actions
- Commit history tracks all changes
- Deployment timestamps recorded
- Approval decisions logged

## 📊 Risk Assessment

### Low Risk Operations
- Validation & linting
- Dry-runs
- Read-only health checks

### Medium Risk Operations
- Standard service deployments (nginx, sonarr, etc.)
- Non-critical service restarts
- Configuration updates (non-DNS/DHCP)

### High Risk Operations
- DNS deployment
- DHCP deployment
- Plex deployment
- Any change to ocean/node005 servers

**High-risk operations have maximum protection enabled**

## 🎯 Best Practices

1. **Always review dry-run logs** before approving
2. **Deploy during maintenance windows** for critical services
3. **Test in check mode first** for new playbooks
4. **Monitor health checks** during deployment
5. **Keep backups** of critical configurations
6. **Document changes** in commit messages
7. **One service at a time** for critical deployments
8. **Verify manually** after automated deployment

## 📞 Support

**Before deploying:**
- Review this safety guide
- Check service health
- Verify recent changes
- Ensure no ongoing issues

**During deployment:**
- Monitor GitHub Actions logs
- Watch for errors or warnings
- Be ready to investigate failures

**After deployment:**
- Verify service health
- Check for unexpected changes
- Monitor logs for issues

---

**Remember**: The multiple safety layers exist to protect your critical infrastructure. **Never bypass or disable safety features.** If a gate blocks deployment, there's a good reason - investigate rather than force through.

**When in doubt → DON'T DEPLOY**
