#!/usr/bin/env bash
# Does the fleet commit only the fix, or the agent's leavings too?
# Run from repo root: bash .github/workflows/lib/agentbox-scratch-staging.test.sh
#
# Both lanes used to stage with `git add -A`, which sweeps whatever the drafting
# agent left in the worktree. It shipped twice, months apart:
#   photonic_inventory#2  (commit 57fc0c46) added .agent-1.log +26 beside a
#                         2-line CSS fix. The log was the watcher's OWN tool
#                         transcript, written into the worktree back then.
#   homelab#429           (commit c4fb44a7) added check.py +153 and NOTHING
#                         else, so a tree holding only scratch read as a
#                         successful draft and opened a PR of pure debris.
#
# stage_draft is extracted from the real scripts and run against real git repos,
# because the failure is entirely in what git ends up with in the index.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WATCHER="$ROOT/files/agentbox/issue-watcher.sh"
ESCALATE="$ROOT/files/agentbox/escalate.sh"
PASS=0
FAIL=0
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

ok()  { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# The house rule (agents.md): stage explicit paths, never -A / . — and it binds
# the agents this repo runs at least as hard as the ones it hosts.
for f in "$WATCHER" "$ESCALATE"; do
  n=$(basename "$f")
  if grep -qE 'git .*add (-A|\.)( |$)' "$f"; then
    bad "$n still stages with a blanket add"
  else
    ok "$n stages explicit paths, not -A"
  fi
done

# Logs must live outside the worktree; writing one into it is how #2 happened.
if grep -q 'opencode run.*>.*\$wt/' "$WATCHER"; then
  bad "the watcher redirects its transcript into the worktree"
else
  ok "the watcher's transcript is written outside the worktree"
fi

eval "$(sed -n '/^SCRATCH_RE=/p' "$WATCHER")"
fn=$(sed -n '/^stage_draft() {/,/^}/p' "$WATCHER")
if [[ -z "${SCRATCH_RE:-}" || -z "$fn" ]]; then
  bad "issue-watcher.sh defines no SCRATCH_RE / stage_draft()"
else
  eval "$fn"
  ok "stage_draft() extracted from the watcher"
fi

# Both lanes must share one definition; a fix in one is not a fix in the other.
if [[ "$fn" == "$(sed -n '/^stage_draft() {/,/^}/p' "$ESCALATE")" && -n "$fn" ]]; then
  ok "escalate.sh carries the same stage_draft()"
else
  bad "escalate.sh's stage_draft() has drifted from the watcher's"
fi

make_repo() {  # $1 = destination
  local origin="$TMP/origin-$(basename "$1")"
  git init -q --bare "$origin"
  git init -q "$1" && git -C "$1" config user.email t@e.st && git -C "$1" config user.name t
  mkdir -p "$1/app/static"
  echo base > "$1/app/static/mobile.css"
  echo base > "$1/README.md"
  git -C "$1" add app/static/mobile.css README.md && git -C "$1" commit -qm base
  git -C "$1" branch -M main
  git -C "$1" remote add origin "$origin" && git -C "$1" push -q -u origin main
  git -C "$1" remote set-head origin main
  git -C "$1" checkout -q -B agent/issue-1 origin/HEAD
}

staged() { git -C "$1" diff --cached --name-only | sort | tr '\n' ' '; }

if declare -F stage_draft >/dev/null; then
  # 1. photonic_inventory#2: a real fix plus the watcher's own transcript.
  r="$TMP/pr2"; make_repo "$r"
  echo fixed > "$r/app/static/mobile.css"
  printf 'Error: Could not find oldString in the file\n' > "$r/.agent-1.log"
  out=$(stage_draft "$r")
  [[ "$(staged "$r")" == "app/static/mobile.css " ]] \
    && ok "the fix is staged and .agent-1.log is not" \
    || bad "staged '$(staged "$r")', expected only app/static/mobile.css"
  [[ "$out" == ".agent-1.log" ]] \
    && ok "the withheld transcript is reported, not dropped silently" \
    || bad "withheld set was '$out', expected .agent-1.log"

  # 2. homelab#429: scratch only. Nothing staged means no commit, which is what
  #    turns "the agent produced junk" back into "no usable draft".
  r="$TMP/pr429"; make_repo "$r"
  printf 'import sys\nprint("scratch")\n' > "$r/check.py"
  out=$(stage_draft "$r")
  git -C "$r" diff --cached --quiet \
    && ok "a scratch-only tree stages nothing" \
    || bad "staged '$(staged "$r")' from a tree holding only check.py"
  [[ "$out" == "check.py" ]] \
    && ok "check.py is reported as withheld" \
    || bad "withheld set was '$out', expected check.py"

  # 3. A new file in a real source directory IS the fix. Withholding it would
  #    be the dangerous failure: a PR that looks complete and is not.
  r="$TMP/newfile"; make_repo "$r"
  echo 'body{}' > "$r/app/static/desktop.css"
  out=$(stage_draft "$r")
  [[ "$(staged "$r")" == "app/static/desktop.css " ]] \
    && ok "a new file under a source dir is staged" \
    || bad "staged '$(staged "$r")', expected app/static/desktop.css"
  [[ -z "$out" ]] || bad "withheld a legitimate new file: $out"

  # 4. Deletions and renames are part of a fix too, and are not untracked.
  r="$TMP/deleted"; make_repo "$r"
  rm "$r/app/static/mobile.css"
  stage_draft "$r" >/dev/null
  [[ "$(staged "$r")" == "app/static/mobile.css " ]] \
    && ok "a deletion is staged" \
    || bad "staged '$(staged "$r")', expected the deleted path"

  # 5. A path with a space must not split into two bogus paths.
  r="$TMP/spaces"; make_repo "$r"
  echo x > "$r/app/static/two words.css"
  stage_draft "$r" >/dev/null
  [[ "$(staged "$r")" == "app/static/two words.css " ]] \
    && ok "a path with a space is staged whole" \
    || bad "staged '$(staged "$r")', expected 'app/static/two words.css'"
fi

echo
echo "passed: $PASS  failed: $FAIL"
[[ "$FAIL" -eq 0 ]]
