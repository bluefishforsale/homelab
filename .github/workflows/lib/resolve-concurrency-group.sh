#!/usr/bin/env bash
# resolve-concurrency-group.sh
#
# Reads a JSON array of playbook paths from stdin.
# For each playbook, extracts the value of the first `hosts:` directive.
# Playbooks with no `hosts:` line (import-style orchestrators) default to "all".
# Emits a single concurrency-group key on stdout: "deploy-<sorted-hosts-joined-by-dash>"
# Empty input → "deploy-none".
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

input="$(cat)"
[[ -z "$input" ]] && input='[]'

count=$(echo "$input" | jq 'length')
if [[ "$count" -eq 0 ]]; then
  echo "deploy-none"
  exit 0
fi

# Use a temp file for dedup (bash 3-compatible, no associative arrays)
HOSTS_FILE="$(mktemp)"
trap 'rm -f "$HOSTS_FILE"' EXIT

while IFS= read -r pb; do
  [[ -z "$pb" ]] && continue
  pb_path="$REPO_ROOT/$pb"
  if [[ ! -f "$pb_path" ]]; then
    continue
  fi
  host=$({ grep -E '^[[:space:]]*-?[[:space:]]*hosts:' "$pb_path" || true; } \
         | head -1 \
         | sed -E 's/^[[:space:]]*-?[[:space:]]*hosts:[[:space:]]*//' \
         | sed -E 's/[[:space:]]*(#.*)?$//' \
         | tr -d '"' \
         | tr -d "'")
  [[ -z "$host" ]] && host="all"
  if ! grep -qxF "$host" "$HOSTS_FILE" 2>/dev/null; then
    printf '%s\n' "$host" >> "$HOSTS_FILE"
  fi
done < <(echo "$input" | jq -r '.[]')

if [[ ! -s "$HOSTS_FILE" ]]; then
  echo "deploy-none"
  exit 0
fi

joined=$(sort -u "$HOSTS_FILE" | paste -sd '-' -)
echo "deploy-$joined"
