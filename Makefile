.PHONY: validate validate-yaml validate-ansible lint-ansible validate-templates security-scan check-vault clean help setup setup-brew setup-python

# Default target
.DEFAULT_GOAL := help

# Colors for output
CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m

help: ## Show this help message
	@echo "$(CYAN)Homelab CI Validation Targets:$(RESET)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(RESET) %s\n", $$1, $$2}'

# ============================================================================
# Setup Targets
# ============================================================================

setup: setup-brew setup-python ## Setup complete development environment
	@echo "$(GREEN)✓ Development environment setup complete$(RESET)"

setup-brew: ## Install Homebrew dependencies
	@echo "$(CYAN)Installing Homebrew dependencies...$(RESET)"
	@if ! command -v brew &> /dev/null; then \
		echo "$(RED)ERROR: Homebrew not installed. Install from https://brew.sh$(RESET)"; \
		exit 1; \
	fi
	@echo "Installing ansible..."
	@brew list ansible &> /dev/null || brew install ansible
	@echo "$(GREEN)✓ Homebrew dependencies installed$(RESET)"

setup-python: ## Install Python dependencies from requirements.txt
	@echo "$(CYAN)Installing Python dependencies...$(RESET)"
	@if [ ! -f requirements.txt ]; then \
		echo "$(RED)ERROR: requirements.txt not found$(RESET)"; \
		exit 1; \
	fi
	@if command -v pip3 &> /dev/null; then \
		pip3 install --user --break-system-packages -r requirements.txt || \
		pip3 install --user -r requirements.txt || \
		pip3 install -r requirements.txt; \
	else \
		echo "$(RED)ERROR: pip3 not found. Install Python 3 first.$(RESET)"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Python dependencies installed$(RESET)"
	@echo ""
	@echo "$(YELLOW)Add to your PATH:$(RESET)"
	@echo "  export PATH=\"\$$HOME/Library/Python/3.13/bin:\$$PATH\""
	@echo ""
	@echo "$(YELLOW)Or permanently:$(RESET)"
	@echo "  echo 'export PATH=\"\$$HOME/Library/Python/3.13/bin:\$$PATH\"' >> ~/.zshrc"
	@echo "  source ~/.zshrc"

# ============================================================================
# Validation Targets
# ============================================================================

validate: validate-yaml validate-ansible validate-templates security-scan check-vault ## Run all validation checks
	@echo "$(GREEN)✓ All validation checks passed$(RESET)"

validate-yaml: ## Validate YAML syntax
	@echo "$(CYAN)Validating YAML syntax...$(RESET)"
	@if ! python3 -c "import yaml" 2>/dev/null; then \
		echo "$(YELLOW)⚠ PyYAML not installed. Run: pip3 install pyyaml$(RESET)"; \
		exit 1; \
	fi
	@find playbooks -name "*.yaml" -o -name "*.yml" | while read file; do \
		echo "Checking $$file"; \
		python3 -c "import yaml; yaml.safe_load(open('$$file'))" || exit 1; \
	done
	@echo "$(GREEN)✓ All YAML files are valid$(RESET)"

validate-ansible: ## Validate Ansible playbook syntax
	@echo "$(CYAN)Validating Ansible playbook syntax...$(RESET)"
	@for playbook in playbooks/*.yaml; do \
		if [ -f "$$playbook" ]; then \
			echo "Checking $$playbook"; \
			ansible-playbook --syntax-check "$$playbook" || exit 1; \
		fi; \
	done
	@find playbooks/individual -name "*.yaml" | while read playbook; do \
		echo "Checking $$playbook"; \
		ansible-playbook --syntax-check "$$playbook" || exit 1; \
	done
	@echo "$(GREEN)✓ All Ansible playbooks have valid syntax$(RESET)"

lint-ansible: ## Lint Ansible playbooks (requires ansible-lint)
	@echo "$(CYAN)Linting Ansible playbooks...$(RESET)"
	@if ! command -v ansible-lint &> /dev/null; then \
		echo "$(YELLOW)⚠ ansible-lint not installed. Run: pip3 install ansible-lint$(RESET)"; \
		exit 1; \
	fi
	@ansible-lint playbooks/*.yaml || true
	@echo "$(GREEN)✓ Ansible lint complete$(RESET)"

validate-templates: ## Validate Jinja2 templates
	@echo "$(CYAN)Validating Jinja2 templates...$(RESET)"
	@if ! python3 -c "import jinja2" 2>/dev/null; then \
		echo "$(YELLOW)⚠ Jinja2 not installed. Run: pip3 install jinja2$(RESET)"; \
		exit 1; \
	fi
	@find files -name "*.j2" | while read template; do \
		echo "Checking $$template"; \
		python3 -c "from jinja2 import Template; Template(open('$$template').read())" || exit 1; \
	done
	@echo "$(GREEN)✓ All Jinja2 templates are valid$(RESET)"

security-scan: ## Scan for hardcoded secrets
	@echo "$(CYAN)Scanning for potential hardcoded credentials...$(RESET)"
	@echo "$(YELLOW)⚠ Review the following matches (if any):$(RESET)"
	@grep -r -i "password\|secret\|token" playbooks/ files/ --include="*.yaml" --include="*.yml" | \
		grep -v "vault" | \
		grep -v "ask-vault-pass" | \
		grep -v "{{ " || echo "$(GREEN)✓ No obvious hardcoded credentials found$(RESET)"
	@echo ""
	@echo "Note: All secrets should be in encrypted vault files"

check-vault: ## Verify vault files are encrypted
	@echo "$(CYAN)Checking vault files are encrypted...$(RESET)"
	@for vault in vault/*.yaml; do \
		if [ -f "$$vault" ] && \
		   [ "$$vault" != "vault/secrets.yaml.template" ] && \
		   [[ "$$vault" != *"_plain"* ]]; then \
			if ! head -n 1 "$$vault" | grep -q "ANSIBLE_VAULT"; then \
				echo "$(RED)ERROR: $$vault is not encrypted!$(RESET)"; \
				exit 1; \
			else \
				echo "$(GREEN)✓ $$vault is encrypted$(RESET)"; \
			fi; \
		fi; \
	done
	@echo "$(GREEN)✓ All vault files are properly encrypted$(RESET)"

clean: ## Clean temporary files
	@echo "$(CYAN)Cleaning temporary files...$(RESET)"
	@find . -name "*.pyc" -delete
	@find . -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true
	@find . -name ".pytest_cache" -type d -exec rm -rf {} + 2>/dev/null || true
	@echo "$(GREEN)✓ Cleaned$(RESET)"
