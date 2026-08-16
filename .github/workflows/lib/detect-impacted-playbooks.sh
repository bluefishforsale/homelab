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
  # A path DELETED in this commit that maps to nothing is a clean removal of a
  # dead input (e.g. deleting an orphan dir) — no-op, not an error. Only a
  # still-present unmapped input is a hard error (a new service left unwired).
  [[ -e "$REPO_ROOT/$1" ]] || return 0
  printf '%s\n' "$1" >> "$UNMAPPED_FILE"
}

emit() {
  local pb="$1"
  # Skip playbooks that no longer exist (e.g., deleted/renamed in this commit).
  [[ -f "$REPO_ROOT/$pb" ]] || return 0
  # Never-auto-apply playbooks, enforced at the single emit chokepoint so nothing
  # (not even a shared vars file reverse-mapping in) can schedule them on push.
  # On-demand runs go through workflow_dispatch, which bypasses this detector.
  #   - terminalbench: multi-hour GPU benchmark.
  #   - *zfs* (storage): touching the ZFS pool must be a coordinated, hand-run,
  #     snapshot-first task — an automated mount/import/remount can break open
  #     file handles or lose data. Name any storage play *zfs*.yaml to inherit
  #     this guard. (The play itself is read-only; a CI guardrail also forbids
  #     mutating ZFS verbs in any playbook.)
  case "$pb" in
    playbooks/individual/ocean/ai/terminalbench*.yaml) return 0 ;;
    playbooks/*zfs*.yaml | playbooks/*zfs*.yml) return 0 ;;
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
  # No inputs are currently allowlisted — every files/, vars/, roles/ input maps
  # to an owning playbook. Add a case here (e.g. `files/foo/*) return 0 ;;`) ONLY
  # for an input genuinely deployed by nothing (dead-but-kept), and treat any
  # entry as debt to clear.
  return 1
}

# vars_service_ports.yaml is the one shared vars file every service reads, so a
# naive filename map fans it out to ~35 playbooks. Instead diff WHICH port keys
# changed and map each to just the playbooks/templates that reference
# `service_ports.<key>`. A single port bump deploys one service; a comment edit
# deploys nothing; if the diff can't be computed we fall back to every consumer
# (never under-deploy). Base version comes from git (override for tests via
# DETECT_PORTS_BASE_FILE; base ref via DETECT_BASE, default HEAD~1).
map_service_ports_change() {
  local head base keys key rc helper d owner
  head="$REPO_ROOT/vars/vars_service_ports.yaml"
  helper="$REPO_ROOT/.github/workflows/lib/changed-service-port-keys.py"
  local base_tmp=""
  if [[ -n "${DETECT_PORTS_BASE_FILE:-}" ]]; then
    base="$DETECT_PORTS_BASE_FILE"
  else
    base_tmp="$(mktemp)"; base="$base_tmp"
    (cd "$REPO_ROOT" && git show "${DETECT_BASE:-HEAD~1}:vars/vars_service_ports.yaml") >"$base" 2>/dev/null || base=""
  fi

  keys=""; rc=0
  if [[ -n "$base" ]]; then
    keys="$(python3 "$helper" "$base" "$head" 2>/dev/null)" || rc=$?
  else
    rc=1
  fi
  [[ -n "$base_tmp" ]] && rm -f "$base_tmp"

  if [[ $rc -ne 0 ]]; then
    # Could not compute the key diff (no base ref, parse error) -> be safe and
    # deploy every consumer, the pre-key-diff behavior.
    while IFS= read -r pb; do [[ -n "$pb" ]] && emit "$pb"; done < <(grep_playbooks "vars_service_ports")
    return 0
  fi

  # keys empty (only comments/whitespace changed) -> deploy nothing.
  while IFS= read -r key; do
    [[ -z "$key" ]] && continue
    # Playbooks that reference this port key directly.
    while IFS= read -r pb; do [[ -n "$pb" ]] && emit "$pb"; done \
      < <(cd "$REPO_ROOT" && grep -rlF "service_ports.$key" playbooks/ 2>/dev/null | grep -E '\.ya?ml$' | grep -v '/tasks/' || true)
    # Templates that reference it -> resolve their owning playbook(s).
    while IFS= read -r f; do
      [[ "$f" =~ ^files/([^/]+)/ ]] || continue
      d="${BASH_REMATCH[1]}"
      owner="$(grep_playbooks "files/$d")"
      [[ -z "$owner" ]] && owner="$(grep_playbooks_service "$d")"
      [[ -z "$owner" ]] && owner="$(grep_playbooks_basename "$d")"
      while IFS= read -r pb; do [[ -n "$pb" ]] && emit "$pb"; done <<< "$owner"
    done < <(cd "$REPO_ROOT" && grep -rlF "service_ports.$key" files/ 2>/dev/null || true)
  done <<< "$keys"
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

  # Storage/ZFS playbooks are dispatch-only — touching the pool must be a
  # coordinated, hand-run task, never a push side effect. Editing one maps to
  # nothing; run it deliberately via workflow_dispatch.
  if [[ "$path" == playbooks/*zfs*.yaml ]] || [[ "$path" == playbooks/*zfs*.yml ]]; then
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

  # vars_service_ports.yaml — the shared port registry. Map by WHICH key changed
  # (one service each), not by the filename (all ~35 consumers).
  if [[ "$path" == vars/vars_service_ports.yaml ]]; then
    map_service_ports_change
    continue
  fi

  # vars/vars_<name>.yaml — map to the playbook(s) that load it by name. A
  # service-specific one (vars_llamacpp_models) maps only to its playbook. If
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

  # Anything else (.github/, scripts/, docs/, .claude/, root files, or a loose
  # path outside the owned files/<svc> | vars/vars_<name> | roles/<role> shapes)
  # does not deploy to hosts — ignore it. Hard-fail is reserved for OWNED inputs
  # that resolve to nothing, which their branches above already record via unmap.
  : # no-op: non-deploying path
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
