# Vault Token Management

Options for managing GitHub runner tokens in Ansible Vault.

---

## Recommended: PAT Approach (No Token Management)

**Use a GitHub Personal Access Token (PAT) instead of registration tokens.**

The `myoung34/github-runner` image automatically generates registration tokens from your PAT, eliminating manual token management.

```yaml
# vault/secrets.yaml
development:
  github:
    token: "ghp_xxxxxxxxxxxx"  # Your GitHub PAT
```

See [SETUP_GITHUB_RUNNERS.md](SETUP_GITHUB_RUNNERS.md) for complete setup.

---

## Legacy: Registration Token Approaches

If you need to manage registration tokens directly:

### Option 1: Automated Vault Update Playbook

Use the dedicated playbook that generates a token and updates the vault automatically:

```bash
ansible-playbook playbooks/individual/infrastructure/github_runner_token_update_vault.yaml \
  --ask-vault-pass
```

**Requires**: GitHub CLI (`gh`) authenticated

### Option 2: Runtime Extra Vars

Skip vault updates and pass the token at deploy time:

```bash
# Generate token
TOKEN=$(ansible-playbook playbooks/individual/infrastructure/github_runner_token_check.yaml \
  --ask-vault-pass 2>&1 | grep "Token:" | head -1 | awk '{print $2}')

# Deploy with token
ansible-playbook -i inventories/github_runners/hosts.ini \
  playbooks/individual/infrastructure/github_docker_runners.yaml \
  --extra-vars "github_registration_token=$TOKEN" \
  --ask-vault-pass
```

### Option 3: Manual Vault Edit

```bash
ansible-vault edit vault/secrets.yaml --ask-vault-pass
```

Update:

```yaml
development:
  github:
    vault_github_registration_token: "AXXXX..."
```

---

## Summary

| Approach | Vault Changes | Automation | Recommended |
|----------|---------------|------------|-------------|
| PAT | Once (store PAT) | Full | **Yes** |
| Update playbook | Each deploy | Full | For legacy |
| Extra vars | None | Full | For legacy |
| Manual edit | Each deploy | Manual | No |

## Related Files

- [SETUP_GITHUB_RUNNERS.md](SETUP_GITHUB_RUNNERS.md) - Complete setup guide
- `github_runner_token_update_vault.yaml` - Auto-update vault playbook
- `github_runner_token_check.yaml` - Token validation playbook
