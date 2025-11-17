# GitHub Actions Workflows

This directory contains GitHub Actions workflows for CI/CD automation of the homelab infrastructure.

## Available Workflows

### CI - Validate & Lint (`ci-validate.yml`)
**Triggers**: Push to main/master/develop, Pull Requests

Automatically validates:
- ✅ YAML syntax for all playbooks
- ✅ Ansible playbook syntax
- ✅ Ansible linting with ansible-lint
- ✅ Jinja2 template validation
- ⚠️ Security scan for hardcoded secrets
- 🔒 Vault file encryption verification

### Deploy Services (`deploy-services.yml`)
**Triggers**: Manual (workflow_dispatch)

Deploys master playbooks with options:
- Choose which master playbook to run
- Enable check mode (dry-run)
- Specify Ansible tags to run or skip
- Uses self-hosted runners with SSH access

**Available Playbooks:**
- `playbooks/00_site.yaml` - Complete infrastructure
- `playbooks/01_base_system.yaml` - Base system only
- `playbooks/02_core_infrastructure.yaml` - Core services only
- `playbooks/03_ocean_services.yaml` - Ocean services only

### Deploy Ocean Service (`deploy-ocean-service.yml`)
**Triggers**: Manual (workflow_dispatch)

Deploys individual **non-critical** services on the ocean server:

**Network Services:** nginx, cloudflared, cloudflare_ddns

**Media Services:** sonarr, radarr, prowlarr, bazarr, nzbget, tautulli, overseerr, plex-meta-manager, tdarr, audible-downloader
*(Note: Plex excluded - use deploy-critical-service.yml)*

**AI/ML Services:** llamacpp, open-webui, n8n, comfyui

**Cloud Services:** nextcloud, payloadcms, strapi, tinacms

**Monitoring:** prometheus, grafana

### Deploy Critical Service (`deploy-critical-service.yml`) **NEW**
**Triggers**: Manual (workflow_dispatch)

Deploys **CRITICAL** infrastructure services with maximum protection:

**Protected Services:**
- **DNS** - Network foundation (192.168.1.2)
- **DHCP** - Address management (192.168.1.2)
- **Plex** - Primary media service (192.168.1.143:32400)

**Safety Features:**
- ✅ Mandatory pre-deployment health check
- ✅ Mandatory dry-run validation
- ✅ **Manual approval required** (uses GitHub Environment)
- ✅ Automatic configuration backup
- ✅ Post-deployment health verification
- ✅ Rollback instructions if health check fails

**Workflow Flow:**
```
Validate → Lint → Health Check → Dry-Run → ⏸️ APPROVAL → Backup → Deploy → Health Check
```

### Deploy Changed Services (`deploy-changed-services.yml`) **NEW**
**Triggers**: Manual (workflow_dispatch)

Intelligently detects and deploys services that changed in commits/PRs:

**What It Does:**
1. 🔍 **Detects changed playbooks** by comparing against base branch
2. 📋 **Categorizes changes** into ocean services, critical services, and master playbooks
3. ✅ **Validates all changes** together
4. 🚀 **Deploys ocean services** automatically (one at a time, fail-fast)
5. ⚠️ **Alerts about critical services** (must deploy manually)
6. 📊 **Provides deployment summary**

**Safety Features:**
- ✅ Sequential deployment (one service at a time)
- ✅ Fail-fast (stops on first error)
- ✅ Mandatory validation of all changes
- ✅ Dry-run before deployment
- ✅ Critical services automatically skipped (requires manual workflow)

**Workflow Flow:**
```
Detect Changes → Validate All → Dry-Run Each → Deploy Each (sequential)
                                     ↓
                              Critical Services → Skip + Alert
```

**Perfect for:**
- Multi-service pull requests
- Deploying a set of related changes
- Batch updates after merging PR

## Setup Requirements

### 1. Self-Hosted Runners
The workflows use self-hosted runners with these labels:
- `self-hosted`
- `homelab`
- `ansible`

**Deployment:**
```bash
ansible-playbook playbooks/individual/infrastructure/github_docker_runners.yaml
```

**This deploys:**
- 4 ephemeral Docker-based runners
- Auto-cleanup after each job
- Ansible pre-installed
- SSH keys mounted from host
- Python 3 with required packages

**Runners have:**
- ✅ Ansible installed
- ✅ Python 3 with PyYAML, Jinja2
- ✅ SSH keys for accessing homelab hosts
- ✅ Access to inventory files
- ✅ Docker runtime for containerized operations

### 2. GitHub Secrets
Configure these secrets in repository settings:

**Required:**
- `ANSIBLE_VAULT_PASSWORD` - Password for decrypting vault files

**To add secrets:**
1. Go to repository **Settings** → **Secrets and variables** → **Actions**
2. Click **New repository secret**
3. Add `ANSIBLE_VAULT_PASSWORD` with your vault password

### 3. GitHub Environment (Critical Services Only)

For critical service deployments with approval:

1. Go to repository **Settings** → **Environments**
2. Click **New environment**
3. Name: `critical-services`
4. Enable **Required reviewers**
5. Add yourself (or team members) as reviewer
6. Save environment

This enables the manual approval gate for DNS/DHCP/Plex deployments.

### 3. Repository Configuration
Ensure the repository has:
- ✅ Self-hosted runners registered (see deployment in `playbooks/individual/infrastructure/github_docker_runners.yaml`)
- ✅ Branch protection rules (optional but recommended)
- ✅ Required status checks for PRs (optional)

## Usage Examples

### Automatic Validation (on Push/PR)
Workflows run automatically when you:
```bash
git add .
git commit -m "Update playbooks"
git push origin main
```

The CI workflow will validate all changes before merging.

### Manual Deployment
1. Go to **Actions** tab in GitHub repository
2. Select workflow (e.g., "Deploy Ocean Service")
3. Click **Run workflow**
4. Fill in the parameters:
   - Select service to deploy
   - Choose check mode for dry-run (recommended first)
   - Click **Run workflow**

### Example: Deploy nginx
1. Actions → Deploy Ocean Service
2. Service: `nginx`
3. Check mode: `false`
4. Run workflow

### Example: Deploy ComfyUI without models
1. Actions → Deploy Ocean Service
2. Service: `comfyui`
3. Skip model downloads: `true`
4. Run workflow

### Example: Full Infrastructure Deployment
1. Actions → Deploy Services
2. Playbook: `playbooks/00_site.yaml`
3. Check mode: `true` (dry-run first!)
4. Run workflow
5. Review output, then run again with check mode: `false`

### Example: Deploy All Changed Services (After PR Merge)
1. Merge PR with multiple service changes
2. Actions → Deploy Changed Services
3. Base ref: `main` (default)
4. Skip critical: `true` (default - critical services need manual workflow)
5. Check mode: `false`
6. Run workflow
7. Workflow will:
   - Detect which playbooks changed
   - Validate all changes
   - Deploy ocean services sequentially
   - Alert if critical services detected

**Output Example:**
```
📦 Ocean Services to Deploy:
  - sonarr
  - radarr
  - n8n

🔴 Critical Services Detected:
  - plex
⚠️  SKIPPED (use deploy-critical-service.yml workflow)

✅ Deploying sonarr... SUCCESS
✅ Deploying radarr... SUCCESS
✅ Deploying n8n... SUCCESS
```

### Example: Critical Service Deployment
1. Actions → Deploy Critical Service (Protected)
2. Service: `dns` or `dhcp` or `plex`
3. Require approval: `true`
4. Run workflow
5. **Review dry-run logs carefully**
6. **Approve deployment** when ready
7. Automatic health checks verify success

## Workflow Status Badges

Add these badges to your README.md:

```markdown
![CI Validate](https://github.com/bluefishforsale/homelab/actions/workflows/ci-validate.yml/badge.svg)
```

## Security Notes

### Vault Secrets
- ⚠️ **Never** commit unencrypted vault files
- ✅ All vault files must start with `$ANSIBLE_VAULT`
- ✅ Use `ansible-vault encrypt` before committing
- ✅ The CI workflow checks vault encryption automatically

### SSH Keys
- 🔑 SSH keys are mounted in the self-hosted runner containers
- 🔑 Keys are not stored in GitHub
- 🔑 Keys are managed via Ansible playbook deployment

### Secrets Management
- 🔒 `ANSIBLE_VAULT_PASSWORD` is stored as GitHub Secret
- 🔒 Never echo or print vault passwords in workflows
- 🔒 Vault password files are cleaned up after use

## Troubleshooting

### Workflow Fails: "No self-hosted runner found"
**Solution:** Ensure GitHub runners are deployed and online:
```bash
ansible-playbook playbooks/individual/infrastructure/github_docker_runners.yaml
ssh ocean "docker ps | grep github-runner"
```

### Workflow Fails: "Permission denied (SSH)"
**Solution:** Verify SSH keys are properly mounted in runner containers.

### Vault Decryption Fails
**Solution:** Verify `ANSIBLE_VAULT_PASSWORD` secret is set correctly in repository settings.

### Playbook Not Found
**Solution:** Verify playbook path in workflow matches actual file location.

## Best Practices

1. **Always test with check mode first** before deploying
2. **Review CI validation** before merging PRs
3. **Use tags** to deploy specific parts of playbooks
4. **Monitor workflow runs** for any failures or warnings
5. **Keep runners updated** with latest Ansible versions

## Future Enhancements

- [ ] Add notification webhooks (Slack/Discord)
- [ ] Add deployment rollback workflows
- [ ] Add automated testing workflows
- [ ] Add scheduled maintenance workflows
- [ ] Add integration with monitoring systems
