#!/usr/bin/env bash
# detect-impacted-playbooks.sh
#
# Reads changed-file paths from stdin (one per line), writes a compact JSON
# array of playbook paths to apply on stdout.
#
# Ownership model: every changed input must resolve to exactly one owning
# playbook (or, for shared files, its precise set of consumers). We never
# replay the whole fleet just because a mapping was missed — an owned input
# that resolves to nothing is a hard error (exit 3), so the owner is forced to
# wire it. Only genuinely-global inputs replay the orchestrators.
#
# Rules (first match wins per input line):
#   playbooks/individual/**/*.ya?ml      -> that playbook
#   playbooks/0[0-9]_*.ya?ml, operations -> that orchestrator/playbook
#   files/cloudflared/**, vars_cloudflared -> cloudflared playbook
#   files/nginx-compose/**               -> nginx playbook
#   files/<svc>/**                       -> playbook referencing files/<svc>, else
#                                           the one declaring `service: <svc>`
#   roles/<role>/**                      -> playbooks referencing the role name
#   vars/vars_<name>.yaml                -> playbooks that load it by name
#                                           (shared file -> its real consumers)
#   inventories/**, group_vars/all*      -> orchestrator replay (truly global)
#   anything ending in .md               -> ignored
#   any other owned input that resolves  -> UNMAPPED -> exit 3 (hard error)
#     to zero playbooks
#
# Output: sorted, deduplicated JSON array on stdout. Empty input → "[]".
# Unmapped input → error on stderr, exit 3.
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

FALLBACK=(
  "playbooks/01_base_system.yaml"
  "playbooks/02_core_infrastructure.yaml"
  "playbooks/03_ocean_services.yaml"
)

CLOUDFLARED_PB="playbooks/individual/ocean/network/cloudflared.yaml"
NGINX_PB="playbooks/individual/ocean/network/nginx_compose.yaml"

# Use temp files for dedup (bash 3-compatible, no associative arrays)
SEEN_FILE="$(mktemp)"
UNMAPPED_FILE="$(mktemp)"
trap 'rm -f "$SEEN_FILE" "$UNMAPPED_FILE"' EXIT

# need_global is set only by genuinely site-wide inputs (inventories,
# group_vars/all). Everything else must resolve to a specific owning playbook or
# be recorded as unmapped — we never silently replay the whole site for an input
# we simply failed to map.
need_global=0

# Record a path that matched an owned-input rule (files/, vars/, roles/) but
# resolved to zero playbooks. Allowlisted paths are a no-op; everything else is
# a hard error so the run fails and the owner wires the input, rather than
# fanning out across the fleet.
unmap() {
  is_known_unowned "$1" && return 0
  printf '%s\n' "$1" >> "$UNMAPPED_FILE"
}

emit() {
  local pb="$1"
  # Skip playbooks that no longer exist (e.g., deleted/renamed in this commit).
  [[ -f "$REPO_ROOT/$pb" ]] || return 0
  # Terminalbench is a multi-hour GPU benchmark and must NEVER auto-apply on
  # push — not even when a shared vars file (e.g. vars_service_ports.yaml)
  # reverse-maps it into the impacted set. The per-path skip below only catches
  # edits to terminalbench's own files; this is the catch-all at the single
  # emit chokepoint. On-demand runs go through workflow_dispatch, which bypasses
  # this detector entirely.
  case "$pb" in
    playbooks/individual/ocean/ai/terminalbench*.yaml) return 0 ;;
  esac
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

# Grep playbooks declaring `service: <name>`. A few playbooks build their files
# dir via `files/{{ service }}` indirection (llamacpp, cloudflared, nginx), so a
# literal `files/<dir>` grep can't find them; this resolves <dir> as a service.
grep_playbooks_service() {
  local svc="$1"
  (cd "$REPO_ROOT" && grep -rlE "^[[:space:]]+service:[[:space:]]+${svc}[[:space:]]*$" playbooks/ 2>/dev/null \
    | grep -E '\.ya?ml$' \
    | grep -v '/tasks/' \
    || true)
}

# Match a playbook whose basename equals the dir/service name (e.g. files/gpu-test
# -> playbooks/individual/ocean/gpu-test.yaml). Last-resort resolver for services
# whose playbook neither references the dir literally nor declares `service:`.
grep_playbooks_basename() {
  local name="$1"
  (cd "$REPO_ROOT" && find playbooks -type f \( -name "${name}.yaml" -o -name "${name}.yml" \) 2>/dev/null \
    | grep -v '/tasks/' \
    | grep -v '/deprecated/' \
    || true)
}

# Inputs deliberately owned by no service playbook: dead dirs, or state managed
# inline by an orchestrator/role via a computed path the rules can't see. A
# change to one is a no-op (deploys nothing), NOT a hard error. This is the
# baseline of pre-existing debt — the coverage test keeps it in sync with
# reality, so it doubles as a cleanup ledger: verify each is dead-and-deletable
# or wire it to an owner, then delete it from here.
is_known_unowned() {
  case "$1" in
    files/gitlab-packages/*  \
    | files/isc-dhcp-server/* \
    | files/navidrome/*       \
    | files/netplan/*         \
    | files/ocean-cloudflared/* \
    | files/ocean-data01/*    \
    | files/ocean-docker/*    \
    | files/spotube/*         \
    | vars/vars_users.yaml) return 0 ;;
  esac
  return 1
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

  # files/<service-dir>/** — find playbooks that reference this dir. If none do
  # literally (the playbook builds the path via `files/{{ service }}`), resolve
  # <dir> as a service name instead of dropping to the site-wide fallback.
  if [[ "$path" =~ ^files/([^/]+)/ ]]; then
    dir="files/${BASH_REMATCH[1]}"
    out="$(grep_playbooks "$dir")"
    [[ -z "$out" ]] && out="$(grep_playbooks_service "${BASH_REMATCH[1]}")"
    [[ -z "$out" ]] && out="$(grep_playbooks_basename "${BASH_REMATCH[1]}")"
    if [[ -z "$out" ]]; then
      unmap "$path"
    else
      while IFS= read -r pb; do
        [[ -n "$pb" ]] && emit "$pb"
      done <<< "$out"
    fi
    continue
  fi

  # roles/<role>/** — grep playbooks that import the role by name
  if [[ "$path" =~ ^roles/([^/]+)/ ]]; then
    role="${BASH_REMATCH[1]}"
    out="$(grep_playbooks_role "$role")"
    if [[ -z "$out" ]]; then
      unmap "$path"
    else
      while IFS= read -r pb; do
        [[ -n "$pb" ]] && emit "$pb"
      done <<< "$out"
    fi
    continue
  fi

  # vars/vars_<name>.yaml — map to the playbook(s) that load it by name. A shared
  # vars file (vars_service_ports) still fans out to every referencing playbook;
  # a service-specific one (vars_llamacpp_models) maps only to its playbook. If
  # no playbook names it, it is unmapped (hard error), not a site-wide replay.
  if [[ "$path" =~ ^vars/(vars_[A-Za-z0-9_]+)\.ya?ml$ ]]; then
    out="$(grep_playbooks "${BASH_REMATCH[1]}")"
    if [[ -z "$out" ]]; then
      unmap "$path"
    else
      while IFS= read -r pb; do
        [[ -n "$pb" ]] && emit "$pb"
      done <<< "$out"
    fi
    continue
  fi

  # Genuinely site-wide inputs: inventory and global group_vars really do affect
  # every host, so the orchestrator replay is the correct mapping, not a guess.
  if [[ "$path" == inventories/* ]] \
     || [[ "$path" =~ ^group_vars/all ]]; then
    need_global=1
    continue
  fi

  # Anything else under a tracked path resolved to no owner: record it as
  # unmapped so the run fails loudly instead of replaying the whole fleet.
  unmap "$path"
done

# Hard-fail on any owned input we could not map to a playbook.
if [[ -s "$UNMAPPED_FILE" ]]; then
  {
    echo "ERROR: changed path(s) map to no playbook owner:"
    sed 's/^/  - /' "$UNMAPPED_FILE"
    echo ""
    echo "Add an owner: put the file under an existing service's files/<svc>/ or"
    echo "vars/vars_<svc>*.yaml, have its playbook declare 'service: <svc>', or map"
    echo "it explicitly in detect-impacted-playbooks.sh. Do NOT let it fall through"
    echo "to a site-wide replay."
  } >&2
  exit 3
fi

# Genuinely-global inputs replay the site-wide orchestrators, unioned with any
# specific playbooks also impacted this commit.
if [[ $need_global -eq 1 ]]; then
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
