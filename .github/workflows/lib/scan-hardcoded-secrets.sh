#!/usr/bin/env bash
# Fail the build on a credential literal committed to this repo.
#
# The previous version of this check could not fail. It ended in
# `|| echo "✓ No obvious hardcoded credentials found"`, so grep finding a secret
# and grep finding nothing both exited 0 — advisory output wearing the name of a
# gate. It also searched only playbooks/ and files/, only *.yaml/*.yml, and only
# for password|secret|token, so on 2026-09-01 it read neither of the two files
# that were actually publishing a key: `vars/vars_terminalbench.yaml` (wrong
# directory) and `prometheus.yml.j2` (wrong extension), and would have missed
# `api_key` on the pattern anyway.
#
# Usage: scan-hardcoded-secrets.sh [paths...]   (default: the owned config tree)
# Exit 0 = clean, 1 = findings printed to stdout.
set -uo pipefail

PATHS=("$@")
[ ${#PATHS[@]} -eq 0 ] && PATHS=(playbooks files vars roles inventories group_vars .github)

# A finding is an assignment of a credential-ish NAME to a literal VALUE.
# Matching the name alone is what made the old check noisy and useless; the
# value is what decides whether something is actually published.
NAME='(pass(word)?|secret|token|api_?key|access_?key|credential|credentials|auth)'
# Value must not be a jinja/env/shell reference, and must have some substance.
LITERAL='[^ "'"'"']{6,}'

# The scanner's own test plants realistic secrets on purpose; excluding it by
# exact name (not a wildcard over tests) keeps that one file from tripping the
# gate without opening a hole for real secrets hidden in other test files.
hits=0
for p in "${PATHS[@]}"; do
  [ -e "$p" ] || continue
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    file="${line%%:*}"
    rest="${line#*:}"
    lineno="${rest%%:*}"
    text="${rest#*:}"

    # Skip comments outright: a commented example is not a published credential.
    printf '%s' "$text" | grep -qE '^[[:space:]]*([#;]|//)' && continue
    # Templated, env-sourced, or vault-sourced values are the correct pattern.
    printf '%s' "$text" | grep -qE '\{\{|\{%|\$\{|\$\(|\$[A-Z_]+|lookup\(|vault|ANSIBLE_VAULT' && continue
    # Documented placeholders, not secrets.
    printf '%s' "$text" | grep -qiE '(example|placeholder|changeme|your[-_]|xxx|<[^>]+>|redacted|_here|\.\.\.)' && continue
    # A value that is a variable reference (token_data.token) or a boolean is
    # not a published credential. Both showed up as false positives on the
    # first run against the real tree.
    printf '%s' "$text" | grep -qiE "[:=][[:space:]]*[\"']?(true|false|null|none|yes|no)[\"',]?[[:space:]]*$" && continue
    printf '%s' "$text" | grep -qE "[:=][[:space:]]*[a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*[[:space:]]*,?[[:space:]]*$" && continue
    # Empty or trivially short values.
    printf '%s' "$text" | grep -qE "[:=][[:space:]]*([\"']{2})?[[:space:]]*$" && continue

    echo "  $file:$lineno: $(printf '%s' "$text" | sed 's/^[[:space:]]*//' | cut -c1-100)"
    hits=$((hits + 1))
  done < <(grep -rInE "$NAME\"?'?[[:space:]]*[:=][[:space:]]*[\"']?$LITERAL" "$p" \
             --include='*.yaml' --include='*.yml' --include='*.j2' \
             --include='*.sh' --include='*.py' --include='*.env' \
             --exclude='*.example' --exclude='*secrets.yaml*' \
             --exclude='scan-hardcoded-secrets.test.sh' 2>/dev/null)
done

if [ "$hits" -gt 0 ]; then
  echo
  echo "FAIL: $hits hardcoded credential literal(s). Move them to vault/secrets.yaml"
  echo "and reference them, e.g. \"{{ ai_services.llamacpp.api_key }}\"."
  exit 1
fi
echo "clean: no hardcoded credential literals in ${PATHS[*]}"
exit 0
