#!/usr/bin/env bash
# Does the fleet notice when a drafting agent commits its own work?
# Run from repo root: bash .github/workflows/lib/agentbox-draft-detection.test.sh
#
# The watcher used to decide "did the agent produce anything?" with
# `[ -n "$(git status --porcelain)" ]`. opencode's build agent commits what it
# writes, so the tree was clean, the check read false, and a correct fix was
# thrown away and escalated to the premium lane. That is why the free lane
# looked like it never worked.
#
# has_draft is extracted from the real script and exercised against real git
# repos, because the failure only appears with an actual commit in place.
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

# Pull has_draft out of the watcher and define it here.
fn=$(sed -n '/^has_draft() {/,/^}/p' "$WATCHER")
if [[ -z "$fn" ]]; then
  bad "issue-watcher.sh defines no has_draft()"
else
  eval "$fn"
  ok "has_draft() extracted from the watcher"
fi

# A worktree the way the watcher leaves it: cloned, on agent/issue-N, with
# origin/HEAD resolvable.
make_repo() {  # $1 = destination
  local origin="$TMP/origin-$(basename "$1")"
  git init -q --bare "$origin"
  git init -q "$1" && git -C "$1" config user.email t@e.st && git -C "$1" config user.name t
  echo base > "$1/file.txt"
  git -C "$1" add -A && git -C "$1" commit -qm base
  git -C "$1" branch -M main
  git -C "$1" remote add origin "$origin" && git -C "$1" push -q -u origin main
  git -C "$1" remote set-head origin main
  git -C "$1" checkout -q -B agent/issue-1 origin/HEAD
}

if declare -F has_draft >/dev/null; then
  # 1. The agent did nothing at all.
  make_repo "$TMP/empty"
  has_draft "$TMP/empty" && bad "clean tree with no new commits must read as no draft" \
                         || ok "no work at all reads as no draft"

  # 2. The agent edited files and left them uncommitted (the case that worked).
  make_repo "$TMP/dirty"
  echo changed > "$TMP/dirty/file.txt"
  has_draft "$TMP/dirty" && ok "uncommitted changes read as a draft" \
                         || bad "uncommitted changes must read as a draft"

  # 3. The regression: the agent committed, so the tree is clean.
  make_repo "$TMP/committed"
  echo changed > "$TMP/committed/file.txt"
  git -C "$TMP/committed" add -A
  git -C "$TMP/committed" commit -qm "agent committed its own work"
  if [[ -n "$(git -C "$TMP/committed" status --porcelain)" ]]; then
    bad "fixture is wrong: tree should be clean after the agent's commit"
  elif has_draft "$TMP/committed"; then
    ok "a clean tree with a commit ahead of origin/HEAD reads as a draft"
  else
    bad "committed work was discarded (the bug that escalated a correct fix)"
  fi
fi

# A premium-lane run that produces nothing must hand the label back. It claims
# the issue as agent-working before invoking claude; the watcher skips that
# label and the drain only selects needs-escalation, so a quiet return orphans the
# issue forever. An expired OAuth session did exactly that to
# photonic_inventory#17.
if sed -n '/no diff for/,/return 0/p' "$ESCALATE" | grep -q 'add-label "$LABEL_HUMAN"'; then
  ok "a fruitless escalation run hands the issue to a human"
else
  bad "escalate.sh strands the issue in agent-working when it produces nothing"
fi

# Neither lane may gate solely on a dirty tree before pushing.
for f in "$WATCHER" "$ESCALATE"; do
  n=$(basename "$f")
  if grep -q 'log --oneline origin/HEAD..HEAD' "$f"; then
    ok "$n counts commits ahead of origin/HEAD, not just a dirty tree"
  else
    bad "$n decides success from the working tree alone"
  fi
done

echo
echo "passed: $PASS  failed: $FAIL"
[[ "$FAIL" -eq 0 ]]
