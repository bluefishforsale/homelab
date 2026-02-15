# CI Automation Guide

Complete guide for automated PR testing, production deployment, and health verification workflows.

---

## Overview

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `ci-validate.yml` | Push/PR | Lint and validate playbooks |
| `pr-test.yml` | PR opened | Test playbooks on ephemeral VM |
| `pr-cleanup.yml` | PR closed | Destroy test VM |
| `main-apply.yml` | Push to main | Auto-deploy + health verify |
| `health-check.yml` | Manual/Callable | Verify service health |
| `rollback.yml` | Manual | Rollback to previous commit |
| `deploy-services.yml` | Manual | Deploy master playbooks |
| `deploy-ocean-service.yml` | Manual | Deploy individual services |
| `deploy-critical-service.yml` | Manual | Protected deploy (DNS/DHCP/Plex) |
| `deploy-changed-services.yml` | Manual | Deploy all changed services |

---

## GitOps Flow

```text
┌──────────────────────────────────────────────────────────────────┐
│                        DEVELOPMENT                                │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│   PR Created ──► ci-validate ──► pr-test (ephemeral VM)          │
│       │                              │                            │
│       │                              ▼                            │
│       │                      Results posted to PR                 │
│       │                              │                            │
│       ▼                              ▼                            │
│   PR Merged ──────────────────► pr-cleanup                       │
│       │                                                           │
└───────┼───────────────────────────────────────────────────────────┘
        │
        ▼
┌──────────────────────────────────────────────────────────────────┐
│                        PRODUCTION                                 │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│   main-apply ──► Deploy Playbooks ──► Health Verification        │
│       │                                       │                   │
│       │                                       ▼                   │
│       │                               ✅ Healthy  ──► Done        │
│       │                               ❌ Degraded ──► Alert       │
│       │                                                           │
│       ▼                                                           │
│   [On Failure] ──► rollback.yml (manual trigger)                 │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

---

## Test VM Specifications

| Property | Value |
|----------|-------|
| Proxmox Host | node005 (192.168.1.105) |
| Template | VMID 9999 |
| Resources | 4 cores, 2GB RAM |
| Naming | `ci-test-pr-{PR_NUMBER}` |
| VMID | 8000 + (PR_NUMBER % 1000) |
| Static IP | 192.168.1.25 |

---

## Prerequisites

### GitHub Secrets

Required secrets in Settings → Secrets and variables → Actions:

| Secret | Description | Format |
|--------|-------------|--------|
| `ANSIBLE_VAULT_PASSWORD` | Vault decryption password | Plain text |
| `PROXMOX_SSH_KEY` | SSH key for node005 | Base64-encoded |

**Important**: Use Repository secrets, not Environment secrets.

### Setup ANSIBLE_VAULT_PASSWORD

```bash
# Your vault password (plain text)
cat ~/.vault_pass
```

Add to GitHub: Settings → Secrets → New repository secret

### Setup PROXMOX_SSH_KEY

```bash
# Base64 encode the SSH key
ssh root@192.168.1.105 "cat ~/.ssh/id_rsa" | base64 -w0
```

Add the base64 string to GitHub secrets.

### GitHub Environment

Create environment `Github Actions CI` with protection rules if desired:
- Settings → Environments → New environment
- Name: `Github Actions CI`
- Optional: Add required reviewers for production deploys

### Self-Hosted Runners

Deployed on gh-runner-01 (192.168.1.250) with labels:

- `self-hosted`
- `homelab`
- `ansible`

---

## Workflow Details

### ci-validate.yml

Runs on every push and PR. Validates:
- YAML syntax
- Ansible playbook syntax
- Jinja2 template syntax
- Security scan for hardcoded secrets
- Vault file encryption

### main-apply.yml

Triggers on push to `main`/`master`. Features:
- Auto-detects changed playbooks
- Applies to production inventory
- **NEW**: Post-deployment health verification
- Generates detailed summary
- Uploads playbook logs as artifacts

### health-check.yml

Standalone and callable workflow. Checks:
- **Ocean**: nginx, plex, prometheus, grafana, sonarr, radarr, prowlarr
- **DNS**: Internal resolution, external forwarding, DHCP server
- **Runners**: Container count, host reachability

Run manually: Actions → Health Check → Run workflow

### rollback.yml

Emergency rollback capability:
- Reverts to specified commit (or HEAD~1)
- Requires confirmation (`ROLLBACK`) for live mode
- Supports dry-run preview
- Re-applies playbooks from target commit

---

## Reusable Components

### SSH Setup Action

Located at `.github/actions/setup-ssh/action.yml`

Configures SSH access for all homelab hosts. Usage:

```yaml
- name: Setup SSH
  uses: ./.github/actions/setup-ssh
  with:
    ssh-key: ${{ secrets.PROXMOX_SSH_KEY }}
```

Provides environment variables:
- `SSH_CONFIG` - Path to SSH config file
- `SSH_KEY_FILE` - Path to private key
- `ANSIBLE_SSH_PRIVATE_KEY_FILE` - For Ansible

---

## Testing PR Workflow

### Create Test PR

```bash
git checkout -b test-ci
echo "# test" >> playbooks/individual/ocean/media/plex.yaml
git add -A && git commit -m "Test CI"
git push origin test-ci
```

Open PR on GitHub and watch Actions tab.

### Verify

```bash
# Check VM exists during test
ssh root@192.168.1.105 "qm list | grep ci-test"

# VM should be gone after completion
```

---

## Changed Playbook Detection

Triggers testing when these paths change:

```text
playbooks/individual/**/*.yaml
playbooks/0[0-9]_*.yaml
```

Also triggers when shared resources change:

```text
roles/**
group_vars/**
files/**
vars/**
```

---

## Troubleshooting

### VM already exists

```bash
PR_NUM=123
VMID=$((8000 + (PR_NUM % 1000)))
ssh root@192.168.1.105 "qm stop $VMID && qm destroy $VMID"
```

### SSH connection refused

Verify cloud-init on template 9999 has SSH keys configured.

### Playbook fails on test VM

Some playbooks may fail if they depend on existing state. Basic validation is the goal.

### Health check shows unhealthy

1. Run `Health Check` workflow manually to verify
2. Check service logs: `ssh ocean "docker logs <service>"`
3. Consider rollback if recent deployment caused issue

### Rollback needed

1. Go to Actions → Rollback
2. Leave target commit empty (defaults to HEAD~1) or specify SHA
3. Run with `dry_run=true` first to preview
4. If preview looks good, run with `dry_run=false` and `confirm=ROLLBACK`

---

## Skip CI

Add `[skip ci]` to commit message:

```bash
git commit -m "docs: Update README [skip ci]"
```

---

## Related Documentation

- [workflows/README.md](workflows/README.md) - Workflow reference
- [SAFETY.md](SAFETY.md) - Safety procedures
- [SETUP.md](SETUP.md) - Full setup guide
