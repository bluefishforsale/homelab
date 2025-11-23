# GitHub Actions Setup Guide

This guide walks through setting up GitHub Actions CI/CD for your homelab repository.

## Prerequisites

✅ Self-hosted GitHub runners deployed (4 ephemeral Docker runners)
✅ Runners have Ansible installed
✅ Runners have SSH access to homelab hosts
✅ Repository pushed to GitHub

## Step 1: Verify Runners

Check that your self-hosted runners are active:

```bash
# SSH to ocean server
ssh ocean

# Check runner status
docker ps | grep github-runner

# Check runner logs
docker logs github-runner-1
```

You should see 4 runners registered and waiting for jobs.

## Step 2: Add GitHub Secret

The workflows need your Ansible vault password to decrypt secrets.

### Add ANSIBLE_VAULT_PASSWORD Secret:

1. Go to your repository on GitHub
2. Click **Settings** → **Secrets and variables** → **Actions**
3. Click **New repository secret**
4. Name: `ANSIBLE_VAULT_PASSWORD`
5. Value: Your ansible vault password (the one you use with `--ask-vault-pass`)
6. Click **Add secret**

⚠️ **Important**: Keep this password secure. It decrypts all your homelab secrets.

## Step 3: Push Workflows to GitHub

Commit and push the new workflows:

```bash
cd /Users/terrac/Projects/bluefishorsale/homelab

git add .github/
git commit -m "Add GitHub Actions CI/CD workflows"
git push origin main
```

## Step 4: Verify CI Workflow

After pushing, the CI validation workflow should run automatically:

1. Go to **Actions** tab in your GitHub repository
2. You should see "CI - Validate & Lint" workflow running
3. Click on it to see the progress
4. Verify all jobs pass (green checkmarks)

## Step 5: Test Deployment Workflow

Try a manual deployment in check mode (dry-run):

1. Go to **Actions** tab
2. Click **Deploy Services** workflow
3. Click **Run workflow** button
4. Select:
   - Playbook: `playbooks/01_base_system.yaml`
   - Check mode: `true` ✅
   - Leave tags empty
5. Click **Run workflow**
6. Watch the deployment run in dry-run mode

## Step 6: Deploy a Service

Deploy an actual service:

1. Go to **Actions** tab
2. Click **Deploy Ocean Service** workflow
3. Click **Run workflow**
4. Select a service (e.g., `nginx`)
5. Check mode: `false` (or `true` to test first)
6. Click **Run workflow**

## Workflow Files Created

```
.github/
├── workflows/
│   ├── ci-validate.yml           # Automatic validation on push/PR
│   ├── deploy-services.yml       # Manual deployment of master playbooks
│   ├── deploy-ocean-service.yml  # Manual deployment of individual services
│   └── README.md                 # Workflow documentation
└── SETUP.md                      # This file
```

## Common Issues & Solutions

### Issue: "No self-hosted runner found"

**Cause**: Runners are offline or not registered

**Solution**:
```bash
# Redeploy runners
ansible-playbook playbooks/individual/infrastructure/github_docker_runners.yaml

# Verify they're running
ssh ocean "docker ps | grep github-runner"
```

### Issue: Workflow runs but skips jobs

**Cause**: Workflow path filters might not match changed files

**Solution**: Check workflow `on:` triggers match your file paths

### Issue: "Vault decryption failed"

**Cause**: `ANSIBLE_VAULT_PASSWORD` secret not set or incorrect

**Solution**:
1. Verify secret is set: Settings → Secrets and variables → Actions
2. Test locally: `ansible-vault view vault/vault_secrets.yaml`
3. Update secret if password changed

### Issue: SSH connection fails during deployment

**Cause**: SSH keys not available in runner

**Solution**:
```bash
# Verify SSH keys are mounted in runner deployment
# Check playbooks/individual/infrastructure/github_docker_runners.yaml
# Ensure SSH_PRIVATE_KEY_PATH is correctly configured
```

## Testing Your Setup

### Test 1: Validation Works
```bash
# Make a small change to a playbook
echo "# test comment" >> playbooks/01_base_system.yaml

# Commit and push
git add playbooks/01_base_system.yaml
git commit -m "Test CI workflow"
git push

# Check Actions tab - CI should run automatically
```

### Test 2: Deployment Works (Dry-run)
1. Actions → Deploy Services
2. Playbook: `playbooks/01_base_system.yaml`
3. Check mode: `true`
4. Run workflow
5. Verify it completes without errors

### Test 3: Service Deployment
1. Actions → Deploy Ocean Service
2. Service: `grafana` (or any deployed service)
3. Check mode: `false`
4. Run workflow
5. Verify service restarts successfully

## Best Practices

### For Development
- ✅ Always test with check mode first
- ✅ Review CI validation before merging PRs
- ✅ Use descriptive commit messages
- ✅ Keep vault files encrypted

### For Deployments
- ✅ Test in check mode before actual deployment
- ✅ Deploy during low-traffic periods
- ✅ Monitor logs during deployment
- ✅ Have rollback plan ready

### For Security
- 🔒 Never commit vault passwords or SSH keys
- 🔒 Rotate vault password periodically
- 🔒 Review workflow logs for sensitive data
- 🔒 Limit repository access appropriately

## Next Steps

After setup is complete:

1. ✅ Add status badges to README.md
2. ✅ Set up branch protection rules
3. ✅ Configure required status checks for PRs
4. ✅ Document deployment procedures for team
5. ✅ Set up notifications (Slack/Discord) for workflow failures

## Additional Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Self-hosted Runners Guide](https://docs.github.com/en/actions/hosting-your-own-runners)
- [Workflow Syntax Reference](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions)
- [Encrypted Secrets](https://docs.github.com/en/actions/security-guides/encrypted-secrets)

## Support

If you encounter issues:

1. Check workflow logs in Actions tab
2. Review runner logs: `docker logs github-runner-1`
3. Test playbooks locally first
4. Verify all prerequisites are met
5. Check GitHub Actions status page for outages
