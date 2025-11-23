# GitHub Runner Registration Token Management

This playbook validates and generates GitHub Actions runner registration tokens for your homelab runners.

## Overview

Registration tokens are required to register self-hosted GitHub Actions runners. They:
- Expire after 1 hour
- Are single-use (consumed when a runner registers)
- Must be refreshed for each deployment

This playbook automates token validation and generation.

## Prerequisites

### Required
- Ansible vault with GitHub configuration
- Python 3 with `requests` library

### Optional (for automatic token generation)
- GitHub CLI (`gh`) installed
- Authenticated with GitHub: `gh auth login`

### Installing GitHub CLI

**macOS:**
```bash
brew install gh
gh auth login
```

**Linux:**
```bash
# See https://cli.github.com for installation
gh auth login
```

## Usage

### Basic Check

Check if your current vault token is defined:

```bash
ansible-playbook playbooks/individual/infrastructure/github_runner_token_check.yaml
```

### With Vault Password

If your vault is encrypted:

```bash
ansible-playbook playbooks/individual/infrastructure/github_runner_token_check.yaml \
  --ask-vault-pass
```

### Generate New Token

The playbook will:
1. Check if token is defined in vault
2. Validate token (if possible)
3. Prompt to generate new token if needed
4. Display the new token and expiration
5. Show URLs and next steps

## What The Playbook Does

### Step 1: Token Validation
- Checks if `github_registration_token` is defined in vault
- Displays token status and configuration

### Step 2: GitHub CLI Check
- Verifies if `gh` CLI is installed
- Checks if authenticated with GitHub
- Provides installation instructions if needed

### Step 3: Token Testing
- Attempts to validate token against GitHub API
- Detects expired or invalid tokens
- Reports validation status

### Step 4: Token Generation
- Prompts to generate new token (if needed)
- Uses GitHub API via `gh` CLI
- Generates fresh 1-hour token

### Step 5: Display Results
- Shows the new token (if generated)
- Displays expiration time
- Provides vault update commands
- Shows manual registration URLs

## Output Examples

### Token Valid
```
====================================================================
GitHub Runner Registration Token Status
====================================================================
Scope: repo
Repository: bluefishforsale/homelab
Token Defined: True
Token Length: 45 characters
====================================================================

✓ GitHub CLI is installed
✓ GitHub CLI is authenticated
✓ Token appears to be valid
```

### Token Invalid - New Generated
```
====================================================================
✓ NEW REGISTRATION TOKEN GENERATED
====================================================================

Scope: Repository
Repository: bluefishforsale/homelab

Token: AABBCCDD1234567890...

Expires At: 2025-11-22T15:30:00Z

====================================================================
NEXT STEPS
====================================================================

1. Update your vault with the new token:
   
   ansible-vault edit vault/secrets.yaml
   
   Then set:
   development:
     github:
       vault_github_registration_token: "AABBCCDD1234567890..."

2. Or pass the token at runtime:
   
   ansible-playbook -i inventories/github_runners/hosts.ini \
     playbooks/individual/infrastructure/github_docker_runners.yaml \
     --extra-vars "github_registration_token=AABBCCDD1234567890..."

====================================================================
⚠ IMPORTANT: This token expires in 1 hour!
====================================================================
```

### Manual Generation Needed
```
====================================================================
MANUAL TOKEN GENERATION
====================================================================

Repository-level token:
1. Visit: https://github.com/bluefishforsale/homelab/settings/actions/runners/new
2. Click "Generate token" or copy the displayed token
3. Token expires in 1 hour

Then update your vault:
  ansible-vault edit vault/secrets.yaml
====================================================================
```

## Updating Vault

### Method 1: Edit Vault Directly
```bash
ansible-vault edit vault/secrets.yaml
```

Then add/update:
```yaml
development:
  github:
    vault_github_registration_token: "YOUR_NEW_TOKEN_HERE"
```

### Method 2: Runtime Override
Skip vault update and pass token directly:
```bash
ansible-playbook -i inventories/github_runners/hosts.ini \
  playbooks/individual/infrastructure/github_docker_runners.yaml \
  --extra-vars "github_registration_token=YOUR_TOKEN"
```

## Manual Token Generation

If you prefer to generate tokens manually:

### Repository-Level
1. Visit: `https://github.com/OWNER/REPO/settings/actions/runners/new`
2. Copy the registration token from the page
3. Token is valid for 1 hour

### Organization-Level
1. Visit: `https://github.com/organizations/ORG/settings/actions/runners/new`
2. Copy the registration token from the page
3. Token is valid for 1 hour

## Workflow

### Typical Deployment Flow

1. **Check token:**
   ```bash
   ansible-playbook playbooks/individual/infrastructure/github_runner_token_check.yaml --ask-vault-pass
   ```

2. **Generate new token if needed** (playbook will prompt)

3. **Update vault or note token for runtime**

4. **Deploy runners:**
   ```bash
   ansible-playbook -i inventories/github_runners/hosts.ini \
     playbooks/individual/infrastructure/github_docker_runners.yaml \
     --ask-vault-pass
   ```

## Troubleshooting

### "GitHub CLI not authenticated"
```bash
gh auth login
# Follow prompts to authenticate
```

### "Token validation failed (401)"
Token is expired or invalid. Generate a new one:
- Run this playbook and choose 'yes' to generate
- Or visit GitHub manually to get a new token

### "Token validation failed (403)"
Your GitHub token lacks permissions. Ensure your authenticated user has:
- Admin access to the repository (repo scope)
- Admin access to the organization (org scope)

### "Could not validate token (404)"
Repository or organization doesn't exist or you lack access.

## Security Notes

- **Tokens expire in 1 hour** - generate fresh tokens before each deployment
- **Tokens are single-use** - consumed when runner registers
- **Don't commit tokens** - always use vault or runtime variables
- **Vault encryption** - keep `vault/secrets.yaml` encrypted

## Configuration

Tokens are sourced from vault path:
```yaml
development:
  github:
    vault_github_registration_token: "..."
```

Referenced in `inventories/github_runners/group_vars/github_runners.yml`:
```yaml
github_registration_token: "{{ development.github.vault_github_registration_token }}"
```

## Related Files

- `github_docker_runners.yaml` - Main runner deployment playbook
- `inventories/github_runners/hosts.ini` - Runner host inventory
- `inventories/github_runners/group_vars/github_runners.yml` - Runner configuration
- `vault/secrets.yaml` - Encrypted secrets (includes token)

## Support

For GitHub CLI issues:
- Documentation: https://cli.github.com
- Manual: `gh help`

For GitHub Actions runners:
- Documentation: https://docs.github.com/en/actions/hosting-your-own-runners
