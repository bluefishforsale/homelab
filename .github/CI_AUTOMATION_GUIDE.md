# CI Automation Implementation Guide

## Overview

Three new workflows have been implemented for fully automated PR testing and production deployment:

1. **`pr-test.yml`** - Test playbooks on ephemeral VM during PR
2. **`pr-cleanup.yml`** - Cleanup test VM when PR closes
3. **`main-apply.yml`** - Auto-apply changed playbooks after merge to main

## Architecture Summary

```
Developer Workflow:
  Create PR → Test VM Created → Playbooks Tested → Results Posted → VM Destroyed
              ↓ (if merged)
  Merge to Main → Detect Changes → Apply to Production → Summary Generated
```

### Test VM Specifications
- **Proxmox Host:** node005.home
- **Template:** VMID 9999 (same as GitHub runners)
- **Resources:** 4 cores, 2GB RAM
- **Network:** Static IP 192.168.1.25 (configured via cloud-init)
- **Hostname:** gh-test-vm.home
- **DNS Record:** gh-test-vm.home → 192.168.1.25
- **SSH Keys:** Downloaded from github.com/bluefishforsale.keys
- **Naming:** `ci-test-pr-{PR_NUMBER}` (e.g., `ci-test-pr-123`)
- **VMID:** 8000 + (PR_NUMBER % 1000)
- **Note:** Only one test VM runs at a time (reuses same IP/hostname)

## Prerequisites

### 1. Proxmox Template (VMID 9999)
You already have this for GitHub runners. Verify it exists:
```bash
ssh node005.home "qm list | grep 9999"
```

### 2. GitHub Secrets

**IMPORTANT:** Both secrets must be in **Repository secrets**, NOT Environment secrets.

**Required secrets for CI workflows:**
- ❗ `ANSIBLE_VAULT_PASSWORD` - Password for decrypting Ansible vault files
- ❗ `PROXMOX_SSH_KEY` - SSH private key for root@node005.home (base64-encoded)

**Check your configuration:** Go to **Settings** → **Secrets and variables** → **Actions**

You should see **both secrets** listed under "Repository secrets":
- ✅ `ANSIBLE_VAULT_PASSWORD` (Repository secrets)
- ✅ `PROXMOX_SSH_KEY` (Repository secrets)

**If you see secrets in "Environment secrets" instead:** 
The workflows cannot access Environment secrets without specifying `environment:` in the workflow file. You must copy them to Repository secrets:

1. Open the Environment secret (e.g., `ANSIBLE_VAULT_PASSWORD`)
2. Copy the value
3. Go to **Repository secrets** section
4. Click "New repository secret"
5. Name: `ANSIBLE_VAULT_PASSWORD`
6. Value: Paste the copied value
7. Click "Add secret"
8. Repeat for any other Environment secrets

After migration, the Environment secrets can be safely deleted if not used elsewhere.

#### Setting up ANSIBLE_VAULT_PASSWORD

This secret is used to decrypt vault-encrypted files (like `vault/secrets.yaml`) during playbook execution.

**Step 1: Get your vault password**
```bash
# Your vault password is what you use when running:
# ansible-vault edit vault/secrets.yaml

# If you have it stored in a file (e.g., ~/.vault_pass):
cat ~/.vault_pass

# Otherwise, you should know this password from memory
```

**Step 2: Add to GitHub Secrets**
1. Go to **Settings** → **Secrets and variables** → **Actions**
2. Make sure you're in the **"Repository secrets"** section (NOT "Environment secrets")
3. Click **New repository secret**
4. Name: `ANSIBLE_VAULT_PASSWORD`
5. Value: Paste your vault password (plain text, no encoding needed)
6. Click **Add secret**

**Note:** This is a simple text password, NOT base64-encoded. Just the plain password string.

**Common mistake:** If you see "Environment secrets" in the UI, you're in the wrong place. Repository secrets are available to all workflows without specifying an environment.

#### Setting up PROXMOX_SSH_KEY

**IMPORTANT:** The SSH key must be base64-encoded before storing as a GitHub secret.

**Step 1: Get the SSH private key from node005.home**
```bash
# SSH to node005.home
ssh root@node005.home

# View the private key (typically id_rsa or id_ed25519)
cat ~/.ssh/id_rsa
# OR
cat ~/.ssh/id_ed25519
```

**Step 2: Base64-encode the key**
```bash
# On macOS (copies to clipboard):
cat ~/.ssh/id_rsa | base64 | pbcopy

# On Linux (display output to copy manually):
cat ~/.ssh/id_rsa | base64

# OR if you have the key in a file locally:
cat /path/to/proxmox_key | base64
```

**Step 3: Add to GitHub Secrets**
1. Go to **Settings** → **Secrets and variables** → **Actions**
2. Make sure you're in the **"Repository secrets"** section (NOT "Environment secrets")
3. Click **New repository secret**
4. Name: `PROXMOX_SSH_KEY`
5. Value: Paste the base64-encoded string (one long line of random characters)
6. Click **Add secret**

**Expected format:** The encoded key will be a single long line like:
```
LS0tLS1CRUdJTiBPUEVOU1NIIFBSSVZBVEUgS0VZLS0tLS0KYjNCbGJuTnphQzFyWlhrdGRqRUFBQUFBQkc1dmJtVUFBQUFFYm05...
```

**Why base64?** GitHub Actions has issues with multi-line secrets in shell contexts. Base64 encoding ensures the key is stored as a single line, and the workflow decodes it when writing to the SSH key file.

### 3. Self-Hosted Runners
Already deployed:
- ✅ Runners with labels: `self-hosted`, `homelab`, `ansible`

### 4. SSH Access to node005.home
Verify from your GitHub runner:
```bash
# Test from runner container
ssh node005.home "qm list"
```

## Testing Phase 1: PR Test Workflow

### Step 1: Make a Simple Change
```bash
# Create a test branch
git checkout -b test-ci-automation

# Make a trivial change to a safe playbook
echo "# Testing CI automation" >> playbooks/individual/base/packages.yaml

# Commit and push
git add playbooks/individual/base/packages.yaml
git commit -m "Test: CI automation for PR testing"
git push origin test-ci-automation
```

### Step 2: Open PR on GitHub
1. Go to GitHub repository
2. Click "Compare & pull request"
3. Create PR targeting `main` branch

### Step 3: Watch Workflow Execute
1. Go to **Actions** tab
2. Find "PR - Test Playbooks on VM" workflow
3. Watch the jobs execute:
   - ✅ Detect Changed Playbooks
   - ✅ Provision Test VM
   - ✅ Test Playbooks
   - ✅ Cleanup Test VM

### Step 4: Verify Results
**On GitHub:**
- PR should have a comment with test results
- Check status shows green ✅

**On Proxmox:**
```bash
# While workflow is running, check VM exists
ssh node005.home "qm list | grep ci-test"

# After workflow completes, VM should be gone
ssh node005.home "qm list | grep ci-test"  # Should be empty
```

### Expected Output

**PR Comment:**
```markdown
## ✅ CI Test Results - PASSED

**Test VM:** `ci-test-pr-123` (`192.168.1.X`)
**Playbooks Tested:** 1
**Passed:** 1
**Failed:** 0

### Results
✅ **PASSED** `playbooks/individual/base/packages.yaml` (45s)
```

## Testing Phase 2: Production Apply Workflow

### Step 1: Merge the PR
1. Review PR test results
2. Click "Merge pull request"
3. Confirm merge to `main`

### Step 2: Watch Auto-Apply
1. Go to **Actions** tab
2. Find "Main - Apply Playbooks to Production" workflow
3. Watch jobs execute:
   - ✅ Detect Changed Playbooks
   - ✅ Apply Playbooks to Production

### Step 3: Review Deployment Summary
1. Click into the workflow run
2. View the summary (top of page)
3. Should show deployment table

### Expected Output

**GitHub Actions Summary:**
```markdown
# 🚀 Production Deployment

## ✅ DEPLOYMENT SUCCESSFUL

**Commit:** `abc123f`
**Author:** Your Name
**Message:** Test: CI automation for PR testing
**Playbooks Applied:** 1
**Successful:** 1
**Failed:** 0

## Results

| Playbook | Status | Duration | Hosts |
|----------|--------|----------|-------|
| `playbooks/individual/base/packages.yaml` | ✅ Success | 32s | ocean |
```

## How It Works

### Changed Playbook Detection

**Direct Playbook Changes:**
```bash
# Any playbook in these locations triggers testing
playbooks/individual/**/*.yaml
playbooks/0[0-9]_*.yaml
```

**Shared Resource Changes:**
If these change, orchestrator playbooks are also tested:
```bash
roles/**
group_vars/**
files/**
vars/**
```

This ensures changes to shared templates/files trigger affected playbooks.

### Host Targeting

Playbooks self-declare their targets via `hosts:` directive:
```yaml
# playbooks/individual/ocean/network/nginx_compose.yaml
- name: Configure Nginx
  hosts: ocean  # ← This determines which host(s) to target
  ...
```

**Test VM Inventory:**
The test VM pretends to be ALL groups:
```ini
[ocean]
192.168.1.X ansible_user=root

[docker]
192.168.1.X ansible_user=root

# ... all groups point to test VM
```

**Production Inventory:**
Uses real inventory where `ocean` = your actual ocean server.

### Concurrent PR Support

Multiple PRs can test simultaneously:
- PR #123 → VMID 8123 → `ci-test-pr-123`
- PR #456 → VMID 8456 → `ci-test-pr-456`

Each gets a unique VM, no conflicts.

## Troubleshooting

### Workflow Fails: "VM already exists"
**Cause:** Previous workflow crashed before cleanup.
**Fix:** Manual cleanup:
```bash
PR_NUM=123  # Your PR number
VMID=$((8000 + (PR_NUM % 1000)))
ssh node005.home "qm stop $VMID && qm destroy $VMID"
```

### Workflow Fails: "Timeout waiting for VM IP"
**Cause:** VM didn't get IP from DHCP or cloud-init failed.
**Fix:** Check Proxmox console:
```bash
ssh node005.home "qm showcmd $VMID"
```

Verify template 9999 has cloud-init configured.

### Workflow Fails: "SSH connection refused"
**Cause:** SSH keys not configured or cloud-init incomplete.
**Fix:** Check SSH keys were downloaded:
```bash
curl https://github.com/bluefishforsale.keys
```

### Playbook Fails on Test VM
**Cause:** Test VM is fresh Ubuntu, missing prerequisites.
**Expected:** Some playbooks may fail if they depend on existing state.
**Resolution:** This is acceptable! Basic validation is the goal.

If playbooks are idempotent (they are), failures indicate real issues.

### Production Apply Runs Unwanted Playbooks
**Cause:** Detection logic added orchestrator playbooks due to shared file changes.
**Fix:** This is by design. Shared files affect multiple playbooks.
**Alternative:** Make more specific changes or manually revert unwanted commits.

## Advanced Usage

### Skip PR Testing for Documentation
Add `[skip ci]` to commit message:
```bash
git commit -m "docs: Update README [skip ci]"
```

### Force Re-test Without Changes
Close and reopen the PR to trigger fresh test.

### Test Multiple Playbooks
Change multiple playbooks in one PR:
```bash
vim playbooks/individual/base/packages.yaml
vim playbooks/individual/ocean/network/nginx_compose.yaml
git add playbooks/
git commit -m "Update packages and nginx"
```

All changed playbooks will be tested on the same VM.

### View Full Logs
Production apply uploads logs as artifacts:
1. Go to workflow run
2. Scroll to bottom
3. Download "playbook-logs-{run_number}"

## Cleanup & Maintenance

### Orphaned VM Cleanup (Optional)
Add a scheduled workflow to cleanup stale VMs:
```bash
# List VMs older than 24h matching ci-test-pr-*
ssh node005.home "qm list | grep ci-test-pr"
```

For now, `pr-cleanup.yml` handles this on PR close.

### Manual VM Cleanup
```bash
# List all CI test VMs
ssh node005.home "qm list | grep ci-test"

# Destroy a specific VM
ssh node005.home "qm stop 8123 && qm destroy 8123"
```

## Security Notes

- Test VMs use same SSH keys as production (from GitHub)
- Test VMs have internet access (can download packages)
- Test VMs are isolated (destroyed after each PR)
- Vault password used for testing (same as production)

## Next Steps

1. ✅ **Test with simple playbook** (packages.yaml)
2. ⏭️ **Test with service playbook** (nginx, grafana, etc.)
3. ⏭️ **Test with orchestrator playbook** (03_ocean_services.yaml)
4. ⏭️ **Monitor production applies** for first few merges
5. ⏭️ **Add notifications** (Slack/Discord) if desired

## Success Metrics

After implementation, you should see:
- ✅ PRs automatically tested on ephemeral VMs
- ✅ Test results posted to PR comments
- ✅ VMs automatically destroyed after testing
- ✅ Merged changes automatically applied to production
- ✅ Zero manual deployment intervention needed

## Rollback Plan

If automation causes issues:

**Disable PR Testing:**
```bash
# Rename workflow to disable
mv .github/workflows/pr-test.yml .github/workflows/pr-test.yml.disabled
git commit -m "Disable PR testing temporarily"
```

**Disable Auto-Apply:**
```bash
# Rename workflow to disable
mv .github/workflows/main-apply.yml .github/workflows/main-apply.yml.disabled
git commit -m "Disable auto-apply temporarily"
```

**Re-enable:**
```bash
# Restore original names
git mv .github/workflows/pr-test.yml.disabled .github/workflows/pr-test.yml
git commit -m "Re-enable PR testing"
```

---

**Questions or issues?** Check workflow logs in GitHub Actions for detailed output.
