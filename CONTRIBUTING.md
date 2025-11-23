# Contributing to Homelab Infrastructure

Thank you for your interest in contributing! This document provides guidelines for making changes to the homelab infrastructure.

## 🎯 Core Principles

1. **Idempotency First** - All playbooks must be safe to run multiple times
2. **Fail Fast** - Errors should stop execution, never ignore failures
3. **Test Before Deploy** - Always dry-run locally before pushing
4. **Document Changes** - Update documentation when changing functionality
5. **Safety Gates** - Use the CI/CD pipeline, don't bypass protections

## 🚀 Getting Started

### Prerequisites

- Ansible installed locally
- SSH access to homelab hosts
- Vault password for encrypted secrets
- Git configured for commits

### Local Development Setup

```bash
# Clone repository
git clone https://github.com/bluefishforsale/homelab.git
cd homelab

# Set up vault password
export ANSIBLE_VAULT_PASSWORD_FILE=~/.ansible_vault_pass
echo "your-vault-password" > ~/.ansible_vault_pass
chmod 600 ~/.ansible_vault_pass

# Verify Ansible installation
ansible --version
ansible-playbook --version
ansible-lint --version  # Optional but recommended
```

## 📝 Making Changes

### 1. Create a Branch

```bash
git checkout -b feature/service-name-update
```

### 2. Make Your Changes

Edit playbooks, templates, or configuration files as needed.

### 3. Test Locally

**Always test before committing:**

```bash
# Syntax check
ansible-playbook --syntax-check playbooks/individual/ocean/media/sonarr.yaml

# Dry-run (check mode)
ansible-playbook --check playbooks/individual/ocean/media/sonarr.yaml --ask-vault-pass

# Lint (optional but recommended)
ansible-lint playbooks/individual/ocean/media/sonarr.yaml
```

### 4. Commit Changes

```bash
git add .
git commit -m "feat: update sonarr configuration for XYZ"
git push origin feature/service-name-update
```

**Commit Message Format:**
- `feat:` - New feature or service
- `fix:` - Bug fix
- `docs:` - Documentation updates
- `refactor:` - Code refactoring
- `chore:` - Maintenance tasks

### 5. Create Pull Request

1. Go to GitHub repository
2. Click "New Pull Request"
3. Select your branch
4. Fill in description with:
   - What changed
   - Why it changed
   - How to test it
5. Wait for CI validation
6. Request review if needed

## 🔒 Safety Rules

### Critical Infrastructure

These services require **extra caution**:

🔴 **DNS** (192.168.1.2)
- Network foundation
- Use `deploy-critical-service.yml` workflow
- Requires manual approval
- Automatic health checks

🔴 **DHCP** (192.168.1.2)
- Address management
- Use `deploy-critical-service.yml` workflow
- Requires manual approval
- Automatic health checks

🔴 **Plex** (192.168.1.143:32400)
- Primary media service
- Use `deploy-critical-service.yml` workflow
- Requires manual approval
- Automatic health checks

### Protected Hosts

Never bypass safety gates for these hosts:
- **ocean** (192.168.1.143) - Primary Docker host
- **node005** - Secondary infrastructure
- **dns01** (192.168.1.2) - DNS/DHCP server

## ✅ Code Quality Standards

### Ansible Playbooks

1. **Must pass syntax check**
   ```bash
   ansible-playbook --syntax-check playbook.yaml
   ```

2. **Must pass ansible-lint** (warnings acceptable, errors not)
   ```bash
   ansible-lint playbook.yaml
   ```

3. **Must be idempotent**
   - Running twice should not change state second time
   - Use appropriate Ansible modules
   - Check state before making changes

4. **No error suppression** without justification
   - Avoid `ignore_errors: true`
   - Avoid `failed_when: false` unless checking state
   - Document why errors are suppressed
   - See [Error Handling Audit](.github/PLAYBOOK_ERROR_HANDLING_AUDIT.md)

5. **Proper error handling**
   ```yaml
   # BAD
   - name: Do something
     shell: command
     ignore_errors: true
   
   # GOOD
   - name: Check if service exists
     shell: systemctl status servicename
     register: service_check
     failed_when: false
     changed_when: false
   
   - name: Do something only if service exists
     shell: command
     when: service_check.rc == 0
   ```

### Docker Compose Templates

1. **Use Jinja2 templating** for dynamic values
2. **Pin image versions** (no `latest` tags)
3. **Include health checks** for all services
4. **Set resource limits** appropriately
5. **Use secrets for credentials** (vault variables)

### Documentation

1. **Update README** when adding services
2. **Update playbook README** for new playbooks
3. **Add inline comments** for complex logic
4. **Document variables** in playbook or group_vars

## 🧪 Testing Guidelines

### Before Committing

- [ ] Syntax check passes
- [ ] Dry-run completes successfully
- [ ] No hardcoded secrets
- [ ] Documentation updated
- [ ] Lint warnings addressed

### After Pushing

- [ ] CI validation passes
- [ ] Dry-run workflow succeeds (for deployments)
- [ ] Manual testing in check mode
- [ ] Production deployment (if approved)

## 🚢 Deployment Process

### Standard Services

Use the GitHub Actions workflow:

1. Go to **Actions** → **Deploy Ocean Service**
2. Select service
3. Enable check mode for first run
4. Review logs
5. Deploy for real if check mode passed

### Critical Services

Use the protected workflow:

1. Go to **Actions** → **Deploy Critical Service (Protected)**
2. Select DNS/DHCP/Plex
3. Review validation logs
4. Review dry-run logs **carefully**
5. **Approve or reject** deployment
6. Monitor health checks

### Emergency Rollback

If deployment fails:

1. Check logs in GitHub Actions
2. Verify service status manually
3. Locate backup (critical services only):
   ```bash
   ssh dns01 "ls -lh /tmp/*-backup-*.tar.gz"
   ```
4. Restore if needed:
   ```bash
   ssh dns01 "tar -xzf /tmp/bind9-backup-*.tar.gz -C /"
   ssh dns01 "systemctl restart bind9"
   ```

## 🐛 Troubleshooting

### Workflow Failures

1. **Check workflow logs** in Actions tab
2. **Check runner logs** on ocean host:
   ```bash
   ssh ocean "docker logs github-runner-1"
   ```
3. **Test locally** with verbose output:
   ```bash
   ansible-playbook -vvv playbooks/path/to/playbook.yaml
   ```

### Common Issues

**Syntax validation fails:**
- Check YAML indentation
- Verify Jinja2 template syntax
- Run `yamllint` for detailed errors

**Dry-run fails:**
- Check vault password is set correctly
- Verify inventory hosts are accessible
- Check for undefined variables

**Deployment fails:**
- Review error message carefully
- Check target host logs
- Verify required files/directories exist
- Test playbook idempotency

## 📚 Additional Resources

- [GitHub Actions Setup](.github/SETUP.md)
- [Safety Guide](.github/SAFETY.md)
- [Error Handling Audit](.github/PLAYBOOK_ERROR_HANDLING_AUDIT.md)
- [Workflow README](.github/workflows/README.md)
- [Playbooks README](playbooks/README.md)
- [Ansible Best Practices](https://docs.ansible.com/ansible/latest/user_guide/playbooks_best_practices.html)

## 🤝 Code Review Process

### For Reviewers

When reviewing pull requests:

1. **Verify tests pass** - CI must be green
2. **Check for safety** - No hardcoded secrets
3. **Review changes** - Are they necessary and minimal?
4. **Test if critical** - Critical services require extra scrutiny
5. **Approve or request changes**

### For Contributors

When requesting review:

1. **Provide context** - Explain what and why
2. **Show testing** - Include test results
3. **Document concerns** - Note any worries
4. **Be responsive** - Address feedback quickly

## ⚖️ License

By contributing, you agree that your contributions will be licensed under the same license as the project.

## 🙋 Getting Help

- Open an issue for bugs or feature requests
- Check existing documentation first
- Provide detailed information (logs, steps, environment)
- Be patient and respectful

---

Thank you for contributing to making this homelab better! 🎉
