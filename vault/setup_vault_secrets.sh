#!/bin/bash

# =============================================================================
# Consolidated Secrets Setup Script
# =============================================================================
# This script helps set up the new consolidated secrets management system
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VAULT_FILE="$SCRIPT_DIR/vault_secrets.yaml"
TEMPLATE_FILE="$SCRIPT_DIR/vault_secrets.yaml.template"

echo "🔐 Homelab Consolidated Secrets Setup"
echo "======================================"

# Check if template exists
if [[ ! -f "$TEMPLATE_FILE" ]]; then
    echo "❌ Template file not found: $TEMPLATE_FILE"
    exit 1
fi

# Check if vault file already exists
if [[ -f "$VAULT_FILE" ]]; then
    echo "⚠️  Warning: $VAULT_FILE already exists!"
    read -p "Do you want to overwrite it? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 0
    fi
fi

# Copy template to vault file
echo "📋 Creating vault_secrets.yaml from template..."
cp "$TEMPLATE_FILE" "$VAULT_FILE"

echo "✅ Created: $VAULT_FILE"
echo ""
echo "📝 Next steps:"
echo "1. Edit the file and replace template values with real secrets:"
echo "   nano $VAULT_FILE"
echo ""
echo "2. Encrypt the file with ansible-vault:"
echo "   ansible-vault encrypt $VAULT_FILE"
echo ""
echo "3. Add to git (encrypted file is safe to commit):"
echo "   git add $VAULT_FILE"
echo "   git commit -m 'Add encrypted consolidated secrets'"
echo ""
echo "4. Update your playbooks to use: vars_files: - vault_secrets.yaml"
echo ""
echo "🔍 For detailed instructions, see:"
echo "   - SECRETS_MANAGEMENT.md"
echo "   - MIGRATION_EXAMPLE.md"
echo ""

# Optional: Open file for editing if editor is available
if command -v nano &> /dev/null; then
    read -p "Do you want to edit the file now? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "Opening $VAULT_FILE for editing..."
        nano "$VAULT_FILE"
        echo ""
        echo "Now run: ansible-vault encrypt $VAULT_FILE"
    fi
fi

echo "✨ Setup complete!"
