# CI Automation Guide

Guide for automated PR testing and production deployment workflows.

---

## Overview

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `pr-test.yml` | PR opened | Test playbooks on ephemeral VM |
| `pr-cleanup.yml` | PR closed | Destroy test VM |
| `main-apply.yml` | Push to main | Auto-deploy merged changes |

---

## Flow

```text
PR Created → Test VM Provisioned → Playbooks Tested → Results Posted → VM Destroyed
                                        ↓ (merge)
                              Main Apply → Deploy to Production
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

---

## Prerequisites

### GitHub Secrets

Required secrets in Settings → Secrets and variables → Actions:

| Secret | Description |
|--------|-------------|
| `ANSIBLE_VAULT_PASSWORD` | Vault decryption password (plain text) |
| `PROXMOX_SSH_KEY` | SSH key for node005 (base64-encoded) |

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
ssh root@192.168.1.105 "cat ~/.ssh/id_rsa" | base64
```

Add the base64 string to GitHub secrets.

### Self-Hosted Runners

Deployed on gh-runner-01 (192.168.1.250) with labels:

- `self-hosted`
- `homelab`
- `ansible`

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

Triggers testing:

```text
playbooks/individual/**/*.yaml
playbooks/0[0-9]_*.yaml
```

Also triggers if these change:

```text
roles/**
group_vars/**
files/**
vars/**
```

---

## Host Targeting

Playbooks declare their targets via `hosts:` directive:

```yaml
- name: Configure Nginx
  hosts: ocean  # Target host
```

Production inventory used for `main-apply.yml`:

```bash
inventories/production/hosts.ini
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
