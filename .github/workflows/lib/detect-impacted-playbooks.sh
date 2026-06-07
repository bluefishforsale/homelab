#!/usr/bin/env bash
# detect-impacted-playbooks.sh
#
# Reads changed-file paths from stdin (one per line), writes a compact JSON
# array of playbook paths to apply on stdout.
#
# Rules (first match wins per input line):
#   playbooks/individual/**/*.ya?ml      -> that playbook
#   playbooks/0[0-9]_*.ya?ml             -> that orchestrator
#   files/cloudflared/**                 -> cloudflared playbook
#   vars/vars_cloudflared.yaml           -> cloudflared playbook
#   files/nginx-compose/**               -> nginx playbook
#   files/<service-dir>/**               -> playbooks grep-referencing files/<service-dir>
#   roles/<role>/**                      -> playbooks grep-referencing the role name
#   vars/vars_service_ports.yaml         -> playbooks grep-referencing it
#   inventories/**, group_vars/all*.yaml -> fallback (orchestrator replay)
#   anything ending in .md               -> ignored
#   anything else under tracked paths    -> fallback
#
# Output: sorted, deduplicated JSON array. Empty input → "[]".
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

FALLBACK=(
  "playbooks/01_base_system.yaml"
  "playbooks/02_core_infrastructure.yaml"
  "playbooks/03_ocean_services.yaml"
)

CLOUDFLARED_PB="playbooks/individual/ocean/network/cloudflared.yaml"
NGINX_PB="playbooks/individual/ocean/network/nginx_compose.yaml"

# Use a temp file for dedup (bash 3-compatible, no associative arrays)
SEEN_FILE="$(mktemp)"
trap 'rm -f "$SEEN_FILE"' EXIT

need_fallback=0

emit() {
  local pb="$1"
  # Skip playbooks that no longer exist (e.g., deleted/renamed in this commit).
  [[ -f "$REPO_ROOT/$pb" ]] || return 0
  if ! grep -qxF "$pb" "$SEEN_FILE" 2>/dev/null; then
    printf '%s\n' "$pb" >> "$SEEN_FILE"
  fi
}

# Grep all playbooks (individual + orchestrator) for a literal substring.
# Prints matching playbook paths relative to repo root.
grep_playbooks() {
  local needle="$1"
  (cd "$REPO_ROOT" && grep -rlF "$needle" playbooks/ 2>/dev/null \
    | grep -E '\.ya?ml$' \
    | grep -v '/tasks/' \
    || true)
}

# Grep playbooks for a role name used as a YAML roles-list entry.
# Matches "- role: <name>" or "- <name>" (standalone, not a path component).
grep_playbooks_role() {
  local role="$1"
  (cd "$REPO_ROOT" && grep -rlE "^\s+-\s+(role:\s+)?${role}(\s|$)" playbooks/ 2>/dev/null \
    | grep -E '\.ya?ml$' \
    | grep -v '/tasks/' \
    || true)
}

while IFS= read -r path; do
  [[ -z "$path" ]] && continue

  # Ignored: any markdown
  if [[ "$path" == *.md ]]; then
    continue
  fi

  # Terminalbench is on-demand only — its playbook runs a multi-hour CPU
  # benchmark and must never auto-apply on push. Changes to its playbook,
  # tasks file, vars, or files dir are dispatched manually via
  # workflow_dispatch (with -e terminalbench_n_tasks=N for fast captures).
  if [[ "$path" == playbooks/individual/ocean/ai/terminalbench*.yaml ]] \
     || [[ "$path" == vars/vars_terminalbench.yaml ]] \
     || [[ "$path" == files/ocean-terminalbench/* ]]; then
    continue
  fi

  # Direct playbook edits
  if [[ "$path" =~ ^playbooks/individual/.*\.ya?ml$ ]] \
     && [[ "$path" != playbooks/individual/*/tasks/* ]]; then
    emit "$path"
    continue
  fi
  if [[ "$path" =~ ^playbooks/0[0-9]_.*\.ya?ml$ ]]; then
    emit "$path"
    continue
  fi
  # Operations playbooks (backup, etc.) are real playbooks too; mapping them
  # to themselves stops a change there from falling through to the 01/02/03
  # site-wide fallback replay.
  if [[ "$path" =~ ^playbooks/operations/.*\.ya?ml$ ]]; then
    emit "$path"
    continue
  fi

  # Cloudflared
  if [[ "$path" == files/cloudflared/* ]] \
     || [[ "$path" == vars/vars_cloudflared.yaml ]]; then
    emit "$CLOUDFLARED_PB"
    continue
  fi

  # Nginx
  if [[ "$path" == files/nginx-compose/* ]]; then
    emit "$NGINX_PB"
    continue
  fi

  # files/<service-dir>/** — find playbooks that reference this dir
  if [[ "$path" =~ ^files/([^/]+)/ ]]; then
    dir="files/${BASH_REMATCH[1]}"
    while IFS= read -r pb; do
      [[ -n "$pb" ]] && emit "$pb"
    done < <(grep_playbooks "$dir")
    continue
  fi

  # roles/<role>/** — grep playbooks that import the role by name
  if [[ "$path" =~ ^roles/([^/]+)/ ]]; then
    role="${BASH_REMATCH[1]}"
    while IFS= read -r pb; do
      [[ -n "$pb" ]] && emit "$pb"
    done < <(grep_playbooks_role "$role")
    continue
  fi

  # vars/vars_service_ports.yaml — grep playbooks that reference it
  if [[ "$path" == vars/vars_service_ports.yaml ]]; then
    while IFS= read -r pb; do
      [[ -n "$pb" ]] && emit "$pb"
    done < <(grep_playbooks "vars_service_ports")
    continue
  fi

  # Fallback triggers
  if [[ "$path" == inventories/* ]] \
     || [[ "$path" =~ ^group_vars/all ]] \
     || [[ "$path" == vars/* ]] \
     || [[ "$path" == roles/* ]] \
     || [[ "$path" == files/* ]]; then
    need_fallback=1
    continue
  fi

  # Anything else: fallback (defensive)
  need_fallback=1
done

if [[ $need_fallback -eq 1 ]] && [[ ! -s "$SEEN_FILE" ]]; then
  for pb in "${FALLBACK[@]}"; do
    emit "$pb"
  done
fi

# Emit sorted, deduped JSON array
if [[ ! -s "$SEEN_FILE" ]]; then
  echo "[]"
else
  sort -u "$SEEN_FILE" | jq -R . | jq -s -c .
fi
