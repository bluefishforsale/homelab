# Automated Vault Token Update

## Approach 1: Semi-Automated (Recommended)

The playbook now outputs the exact token value. To update the vault:

```bash
# 1. Run the token check playbook and capture the token
TOKEN=$(ansible-playbook playbooks/individual/infrastructure/github_runner_token_check.yaml 2>&1 | grep "Token:" | head -1 | awk '{print $2}')

# 2. Update vault file directly (requires vault password)
ansible-vault edit vault/secrets.yaml
# Then manually update the vault_github_registration_token value

# OR use sed with temporary decryption (more automated)
ansible-vault decrypt vault/secrets.yaml
sed -i.bak "s/vault_github_registration_token:.*/vault_github_registration_token: \"$TOKEN\"/" vault/secrets.yaml
ansible-vault encrypt vault/secrets.yaml
```

## Approach 2: Fully Automated Script

Create a helper script `update-runner-token.sh`:

```bash
#!/bin/bash
set -e

VAULT_FILE="vault/secrets.yaml"
PLAYBOOK="playbooks/individual/infrastructure/github_runner_token_check.yaml"

echo "Generating new GitHub runner token..."
OUTPUT=$(ansible-playbook "$PLAYBOOK" 2>&1)

# Extract token from playbook output
TOKEN=$(echo "$OUTPUT" | grep "Token:" | head -1 | awk '{print $2}')

if [ -z "$TOKEN" ]; then
    echo "ERROR: Failed to generate token"
    exit 1
fi

echo "Token generated: $TOKEN"
echo "Updating vault..."

# Decrypt vault
ansible-vault decrypt "$VAULT_FILE"

# Update token
sed -i.bak "s/vault_github_registration_token:.*/vault_github_registration_token: \"$TOKEN\"/" "$VAULT_FILE"

# Re-encrypt vault
ansible-vault encrypt "$VAULT_FILE"

echo "✓ Vault updated successfully!"
echo ""
echo "Next step - Deploy runners:"
echo "  ansible-playbook -i inventories/github_runners/hosts.ini \\"
echo "    playbooks/individual/infrastructure/github_docker_runners.yaml \\"
echo "    --ask-vault-pass"
```

Make it executable:
```bash
chmod +x update-runner-token.sh
```

## Approach 3: Use Extra Vars (No Vault Update)

Skip vault updates entirely and pass the token directly:

```bash
# Generate token
TOKEN=$(ansible-playbook playbooks/individual/infrastructure/github_runner_token_check.yaml 2>&1 | grep "Token:" | head -1 | awk '{print $2}')

# Deploy runners with token
ansible-playbook -i inventories/github_runners/hosts.ini \
  playbooks/individual/infrastructure/github_docker_runners.yaml \
  --extra-vars "github_registration_token=$TOKEN" \
  --ask-vault-pass
```

This approach avoids vault management entirely and is more idempotent.

## Recommendation

**Use Approach 3** - it's:
- ✅ Fully automated
- ✅ Idempotent (no file modifications)
- ✅ Works with CI/CD
- ✅ Follows Ansible best practices (extra-vars > vault vars)
- ✅ No vault editing complexity

The vault should store long-lived credentials (passwords, API keys), while short-lived tokens (1-hour expiry) are better passed as runtime parameters.
