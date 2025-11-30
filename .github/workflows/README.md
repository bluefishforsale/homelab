# GitHub Actions Workflows

CI/CD workflows for homelab infrastructure automation.

---

## Quick Reference

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `ci-validate.yml` | Push, PR | Validate and lint |
| `deploy-ocean-service.yml` | Manual | Deploy single service |
| `deploy-critical-service.yml` | Manual | Deploy DNS/Plex (approval required) |
| `deploy-services.yml` | Manual | Deploy master playbooks |
| `deploy-changed-services.yml` | Manual | Deploy changed playbooks |
| `main-apply.yml` | Push to main | Auto-deploy merged changes |
| `pr-test.yml` | PR | Test on ephemeral VM |
| `pr-cleanup.yml` | PR closed | Destroy test VM |

---

## Workflows

### ci-validate.yml

**Trigger**: Push to main/master/develop, Pull Requests

Validates all playbooks:

- YAML syntax check
- Ansible syntax check
- ansible-lint
- Jinja2 template validation
- Secret detection scan
- Vault encryption verification

### deploy-ocean-service.yml

**Trigger**: Manual (workflow_dispatch)

Deploy individual ocean services (non-critical):

| Category | Services |
|----------|----------|
| Network | nginx, cloudflared, cloudflare_ddns |
| Media | sonarr, radarr, prowlarr, bazarr, nzbget, tautulli, overseerr, tdarr |
| AI/ML | llamacpp, open-webui, comfyui |
| Services | nextcloud, tinacms, frigate, homeassistant |
| Monitoring | prometheus, grafana |

**Note**: Plex requires `deploy-critical-service.yml`

### deploy-critical-service.yml

**Trigger**: Manual (workflow_dispatch)

Deploy critical services with approval:

| Service | Host | Port |
|---------|------|------|
| DNS | dns01 (192.168.1.2) | 53 |
| Plex | ocean (192.168.1.143) | 32400 |

Flow:

```text
Validate → Lint → Health Check → Dry-Run → [Approval] → Deploy → Health Check
```

### deploy-services.yml

**Trigger**: Manual (workflow_dispatch)

Deploy master playbooks:

- `playbooks/00_site.yaml` - Full infrastructure
- `playbooks/01_base_system.yaml` - Base system
- `playbooks/02_core_infrastructure.yaml` - Core services
- `playbooks/03_ocean_services.yaml` - All ocean services

### deploy-changed-services.yml

**Trigger**: Manual (workflow_dispatch)

Detect and deploy changed playbooks:

1. Detect changed playbooks vs base branch
2. Validate all changes
3. Deploy sequentially (fail-fast)
4. Skip critical services (alert only)

### main-apply.yml

**Trigger**: Push to main/master

Auto-deploy merged changes:

1. Detect changed playbooks from commit
2. Apply to production
3. Generate summary
4. Upload logs as artifacts

### pr-test.yml

**Trigger**: Pull Request

Test playbooks on ephemeral VM:

1. Detect changed playbooks
2. Provision test VM on Proxmox
3. Run playbooks against test VM
4. Post results as PR comment
5. Destroy VM

### pr-cleanup.yml

**Trigger**: Pull Request closed

Cleanup test VMs when PR closes.

---

## Setup

### Self-Hosted Runners

Deploy runners on gh-runner-01:

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/github_docker_runners.yaml --ask-vault-pass
```

Runner features:

- 4 ephemeral Docker containers
- Ansible pre-installed
- SSH keys mounted
- Labels: `self-hosted`, `homelab`, `ansible`

### GitHub Secrets

Required secret:

| Secret | Description |
|--------|-------------|
| `ANSIBLE_VAULT_PASSWORD` | Vault decryption password |

Add at: Settings → Secrets and variables → Actions

### GitHub Environment

For critical service approval:

1. Settings → Environments
2. Create `critical-services`
3. Enable Required reviewers
4. Add reviewer(s)

---

## Usage

### Deploy Single Service

1. Actions → Deploy Ocean Service
2. Select service
3. Run workflow

### Deploy Critical Service

1. Actions → Deploy Critical Service (Protected)
2. Select DNS or Plex
3. Run workflow
4. Review dry-run output
5. Approve deployment

### Deploy All Services

1. Actions → Deploy Services
2. Select `playbooks/03_ocean_services.yaml`
3. Run workflow

---

## Troubleshooting

### No self-hosted runner found

```bash
ssh terrac@192.168.1.250 "docker ps | grep github-runner"
```

### Permission denied (SSH)

Verify SSH keys mounted in runner containers.

### Vault decryption fails

Verify `ANSIBLE_VAULT_PASSWORD` secret in repository settings.

---

## Related Documentation

- [SAFETY.md](../SAFETY.md) - Safety procedures
- [SETUP.md](../SETUP.md) - Full setup guide
