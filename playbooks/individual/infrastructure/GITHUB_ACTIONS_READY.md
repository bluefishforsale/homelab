# GitHub Actions CI/CD Readiness

## ✅ Status: PRODUCTION READY

The `github_runner_token_check.yaml` playbook is now **fully compatible** with both local execution and GitHub Actions CI/CD pipelines.

## Cross-Platform Support

### ✓ macOS (Local Development)
- Uses `gh` CLI when available
- Falls back to manual instructions
- Compatible with Homebrew package manager

### ✓ Linux (Debian/Ubuntu Runners)
- Uses `gh` CLI when available  
- Uses native `ansible.builtin.uri` for API calls
- Compatible with apt package manager
- **Optimized for GitHub Actions runners**

## Authentication Methods

The playbook supports **three authentication methods** with automatic fallback:

### 1. GitHub Actions (Preferred for CI/CD)
- **Detection**: `GITHUB_ACTIONS=true` environment variable
- **Auth**: `GITHUB_TOKEN` (automatically provided)
- **Method**: `ansible.builtin.uri` module (no external dependencies)
- **Platform**: Any (Linux, macOS, Windows runners)

```yaml
env:
  GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  GITHUB_ACTIONS: 'true'
```

### 2. GitHub CLI (Preferred for Local)
- **Detection**: `gh` command available
- **Auth**: `gh auth login`
- **Method**: `gh api` command
- **Platform**: macOS, Linux, Windows

```bash
brew install gh  # macOS
apt-get install gh  # Linux
gh auth login
```

### 3. Manual Token Generation (Fallback)
- **Detection**: No automated method available
- **Auth**: N/A
- **Method**: Manual browser-based token generation
- **Platform**: Any

## How It Works

### Local Execution Flow
```
1. Check vault for existing token
2. Validate token via GitHub API
3. Detect if gh CLI is installed & authenticated
4. IF gh available:
   → Generate token via gh api
   ELSE:
   → Display manual instructions with URL
5. Report status and next steps
```

### GitHub Actions Flow
```
1. Check vault for existing token (optional)
2. Validate token via GitHub API (if exists)
3. Detect GITHUB_ACTIONS=true environment
4. IF GITHUB_TOKEN available:
   → Generate token via ansible.builtin.uri
   → ✓ AUTOMATIC SUCCESS
5. Report status and provide deployment command
```

## GitHub Actions Integration

### Example Workflow

```yaml
name: Runner Token Validation
on: push

jobs:
  validate:
    runs-on: ubuntu-latest  # Debian-based
    permissions:
      actions: write  # Required for runner token generation
    
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: '3.11'
      
      - name: Install Ansible
        run: pip install ansible
      
      - name: Run validation
        run: |
          ansible-playbook \
            playbooks/individual/infrastructure/github_runner_token_check.yaml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

See `.github/workflows/runner-token-check.yml.example` for complete example.

## No User Interaction Required

### ✓ Fully Automated
- No prompts or pauses
- No manual input needed  
- Runs unattended from start to finish

### ✓ Safe Failure Handling
- Never crashes mid-playbook
- Graceful degradation when tools unavailable
- Clear error messages and next steps

### ✓ Idempotent
- Safe to run multiple times
- No side effects
- Predictable outcomes

## Platform Compatibility Matrix

| Feature | macOS Local | Linux Local | GitHub Actions (Linux) | GitHub Actions (Other) |
|---------|-------------|-------------|----------------------|----------------------|
| **Ansible** | ✅ | ✅ | ✅ | ✅ |
| **gh CLI** | ✅ Optional | ✅ Optional | ❌ Not needed | ❌ Not needed |
| **GITHUB_TOKEN** | ❌ | ❌ | ✅ Auto | ✅ Auto |
| **Manual fallback** | ✅ | ✅ | ✅ | ✅ |
| **Auto-generate** | ✅ (with gh) | ✅ (with gh) | ✅ (native) | ✅ (native) |
| **No interaction** | ✅ | ✅ | ✅ | ✅ |

## Dependencies

### Required (All Platforms)
- Ansible 2.9+
- Python 3.6+

### Optional (For Auto-Generation)
- **Local**: GitHub CLI (`gh`)
- **CI/CD**: GITHUB_TOKEN (auto-provided)

### No Additional Packages Needed
- Uses built-in Ansible modules
- No Python pip packages required
- No system packages required (for GitHub Actions)

## Environment Variables

### Automatically Detected

| Variable | Source | Purpose |
|----------|--------|---------|
| `GITHUB_ACTIONS` | GitHub Actions | Detect CI/CD environment |
| `GITHUB_TOKEN` | GitHub Actions | API authentication |
| `RUNNER_OS` | GitHub Actions | Platform detection (informational) |

### User-Provided (Optional)

| Variable | Format | Purpose |
|----------|--------|---------|
| `github_registration_token` | Vault or --extra-vars | Pre-existing token |

## Testing

### Test Locally (macOS/Linux)
```bash
ansible-playbook playbooks/individual/infrastructure/github_runner_token_check.yaml
```

### Test in GitHub Actions
```bash
# Push to repository with workflow enabled
git push origin main
```

### Expected Output

**Local (no gh CLI)**:
```
Environment: Local Execution
✗ Cannot auto-generate: No auth method available
FINAL STATUS: ⚠ ACTION REQUIRED
```

**GitHub Actions**:
```
Environment: GitHub Actions CI/CD  
✓ Will generate token using GitHub Actions GITHUB_TOKEN
✓✓✓ SUCCESS: New token generated and ready to use!
FINAL STATUS: ✓ SUCCESS
```

## Security Considerations

### ✓ Token Safety
- Tokens never committed to repository
- Tokens expire after 1 hour
- GITHUB_TOKEN has limited scope

### ✓ Vault Protection
- Vault files encrypted
- GitHub Actions can use secrets
- No plaintext credentials

### ✓ Minimal Permissions
- Read access for validation
- Write access only for token generation
- No repository modification

## Troubleshooting

### Issue: "Cannot find vault file"
**Cause**: Playbook uses relative paths  
**Solution**: Run from repository root

### Issue: "GITHUB_TOKEN not available"
**Cause**: Missing permissions in workflow  
**Solution**: Add `permissions: actions: write`

### Issue: "Token generation failed (403)"
**Cause**: Insufficient permissions  
**Solution**: Verify user/token has admin access to repo/org

## Next Steps After Token Generation

### Automated Workflow
```bash
# Token automatically available in playbook
ansible-playbook -i inventories/github_runners/hosts.ini \
  playbooks/individual/infrastructure/github_docker_runners.yaml
```

### Manual Update
```bash
# Update vault
ansible-vault edit vault/secrets.yaml

# Or use runtime override
ansible-playbook ... --extra-vars "github_registration_token=TOKEN"
```

## Summary

✅ **Platform Ready**: macOS, Linux, GitHub Actions  
✅ **Zero Interaction**: Fully automated execution  
✅ **Multi-Auth**: gh CLI, GITHUB_TOKEN, manual fallback  
✅ **CI/CD Optimized**: Native GitHub Actions integration  
✅ **Production Safe**: Idempotent, error-handled, secure  

The playbook is **ready for production use** in both local development and automated CI/CD pipelines without any modifications.
