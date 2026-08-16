#!/usr/bin/env bash
# check-no-zfs-mutations.sh
#
# Data-safety guardrail: NO committed playbook may mutate ZFS. /data01 is the
# most critical infrastructure in the fleet and an automated pool/dataset change
# (mount, unmount, import, export, set, destroy, rollback, ...) can break
# services holding open file handles or lose data. Pool changes are coordinated,
# hand-run, snapshot-first tasks — never a side effect of a commit or CI.
#
# Fails if any scanned file contains a MUTATING zfs/zpool verb, the
# community.general zfs/zpool modules, or a zfs-fstype mount. Allowed: reads
# (zpool list/status, zfs get/list), and non-ZFS mounts (e.g. tmpfs ramdisk).
# Comment lines are ignored so this policy's own prose doesn't trip it.
#
# Limitation: a mutation hidden inside a shell script a play *calls* is not
# visible here — those scripts must follow the same hand-run rule.
#
# Usage: check-no-zfs-mutations.sh [dir ...]   (defaults to playbooks/ roles/)
set -uo pipefail

if [[ $# -gt 0 ]]; then
  SCAN=("$@")
else
  cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
  SCAN=(playbooks roles)
fi

# Mutating verbs only — reads (list, get, status, iostat) are deliberately absent.
# Boundaries use [^[:alnum:]_] so punctuation (comma in inline YAML, brace, quote,
# semicolon) still bounds a match, and a leading boundary avoids matching inside
# a longer word (e.g. "myzpool"). `zfs-mount` never matches: the verb patterns
# require whitespace after `zfs`, and a hyphen is not whitespace.
B='[^[:alnum:]_]'
PATTERN="(^|$B)zpool[[:space:]]+(create|destroy|import|export|replace|attach|detach|add|remove|clear|offline|online|split|labelclear)($B|\$)"
PATTERN+="|(^|$B)zfs[[:space:]]+(create|destroy|set|mount|unmount|rollback|rename|receive|promote|share|unshare|inherit|allow|unallow)($B|\$)"
PATTERN+="|community\\.general\\.(zfs|zpool)($B|\$)"
PATTERN+="|fstype:[[:space:]]*zfs($B|\$)"

# -H forces the "file:line:content" format even for a single file (GNU grep omits
# the filename otherwise), so the comment-line filter below is portable. The
# filter drops lines whose content (after file:line:) starts with a YAML comment.
hits="$(grep -rHnE "$PATTERN" "${SCAN[@]}" 2>/dev/null | grep -vE '^[^:]*:[0-9]+:[[:space:]]*#' || true)"

if [[ -n "$hits" ]]; then
  echo "FAIL: ZFS mutation found in a playbook/role. Storage changes must be"
  echo "coordinated and hand-run (snapshot-first), never committed to an"
  echo "auto-runnable playbook. Offending lines:"
  echo "$hits" | sed 's/^/  /'
  exit 1
fi
echo "PASS: no ZFS-mutating verbs/modules in ${SCAN[*]}."
