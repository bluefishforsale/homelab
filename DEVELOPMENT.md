# Development Setup

## Quick Start

```bash
# One-time setup: Install all dependencies
make setup

# Add Python bin directory to your PATH
echo 'export PATH="$HOME/Library/Python/3.13/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc

# Run all validation checks
make validate
```

## Prerequisites

- **macOS** with Homebrew installed
- **Python 3** (comes with macOS)
- **Homebrew**: Install from https://brew.sh

## Setup Commands

### Complete Setup
```bash
make setup              # Install all dependencies (Homebrew + Python)
```

### Individual Setup
```bash
make setup-brew         # Install Homebrew packages (Ansible)
make setup-python       # Install Python packages from requirements.txt
```

## Validation Commands

### Run All Checks
```bash
make validate           # Run all validation checks
```

### Individual Checks
```bash
make validate-yaml      # Validate YAML syntax
make validate-ansible   # Validate Ansible playbook syntax
make validate-templates # Validate Jinja2 templates
make security-scan      # Scan for hardcoded secrets
make check-vault        # Verify vault files are encrypted
```

### Optional Linting
```bash
make lint-ansible       # Lint Ansible playbooks (requires ansible-lint)
```

## Dependencies

### Homebrew Packages
- `ansible` - Ansible automation tool

### Python Packages (from requirements.txt)
- `pyyaml>=6.0` - YAML parsing and validation
- `jinja2>=3.1` - Jinja2 template validation
- `ansible>=2.15` - Ansible for playbook execution

## PATH Configuration

After installing Python packages, add the Python bin directory to your PATH:

```bash
# For zsh (default on macOS)
echo 'export PATH="$HOME/Library/Python/3.13/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc

# For bash
echo 'export PATH="$HOME/Library/Python/3.13/bin:$PATH"' >> ~/.bash_profile
source ~/.bash_profile
```

**Note:** The Python version (3.13) may vary depending on your system. Check with:
```bash
python3 --version
```

## Troubleshooting

### Permission Denied: ~/Library/Python
If you get permission errors:
```bash
sudo chown -R $USER ~/Library/Python
```

### Command Not Found: ansible
Make sure the Python bin directory is in your PATH:
```bash
echo $PATH | grep "Library/Python"
```

If not found, add it as shown in the PATH Configuration section above.

### PyYAML Not Installed
Run the setup:
```bash
make setup-python
```

## CI/CD

These same validation checks run automatically in GitHub Actions on every push. See `.github/workflows/ci-validate.yml` for details.
