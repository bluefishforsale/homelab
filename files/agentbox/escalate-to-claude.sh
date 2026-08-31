#!/usr/bin/env bash
# agentbox escalation drain (premium lane).
# Rendered from files/agentbox/escalate-to-claude.sh.
#
# Resolves issues the free lane labelled needs-claude using Claude Code on the
# Pro/Max subscription. ANTHROPIC_API_KEY MUST stay unset so Claude Code uses
# the subscription (fixed cost) and not metered API. Throughput is bounded by
# the weekly subscription cap; Claude Code hard-stops rather than overaging.
set -euo pipefail
unset ANTHROPIC_API_KEY

OWNER="bluefishforsale"
# Separate from repos/ (the RC sessions' cwd) so escalate's clone + commits can't
# collide with a live remote-control session on the same repo.
WORKROOT="{{ home }}/work"
LABEL_WORKING="agent-working"
LABEL_CLAUDE="needs-claude"
LABEL_REVIEW="needs-human-merge"
# Fail closed if the env file predates this setting: the owner alone, never all.
ISSUE_AUTHOR_ALLOWLIST="${ISSUE_AUTHOR_ALLOWLIST:-$OWNER}"

# shellcheck disable=SC1091
source "{{ home }}/.config/agentbox/agentbox.env"
unset ANTHROPIC_API_KEY  # the env file must not set it; enforce here too

# gh add-label fails if the label doesn't exist in the repo; create them first.
ensure_labels() {
  local slug="$1"
  gh label create "$LABEL_WORKING" --repo "$slug" --color FBCA04 --force >/dev/null 2>&1 || true
  gh label create "$LABEL_CLAUDE"  --repo "$slug" --color 5319E7 --force >/dev/null 2>&1 || true
  gh label create "$LABEL_REVIEW"  --repo "$slug" --color D93F0B --force >/dev/null 2>&1 || true
}

# Tiered autonomy (ADR 0001) applies regardless of lane: auto-merge only an
# opted-in repo whose diff touches no prod-affecting paths; else open + label.
NOPROD_RE='^(docs/|README|CONTEXT|.*\.md$|.*_test\.|test/|tests/|spec/)'

no_prod_effect() {  # $1 = newline-separated changed files
  [ -n "$1" ] || return 1
  while IFS= read -r f; do
    [ -z "$f" ] && continue
    printf '%s\n' "$f" | grep -Eq "$NOPROD_RE" || return 1
  done <<<"$1"
  return 0
}

resolve_issue() {  # $1 = repo (short name); $2 = issue number
  local repo="$1" num="$2" slug="$OWNER/$1"
  # Per-repo/per-lane telemetry labels for everything claude emits this pass.
  export OTEL_RESOURCE_ATTRIBUTES="repo=$repo,lane=claude,service=agentbox"
  ensure_labels "$slug"

  # The label is not the authority; the issue's author is. Anything that can put
  # needs-claude on an issue would otherwise be feeding a prompt straight to a
  # premium-lane agent running --permission-mode acceptEdits, which commits and
  # pushes. Both entry points (the periodic drain below and the alert receiver's
  # triggered mode) funnel through here, so this one check covers both.
  local author
  author=$(gh issue view "$num" --repo "$slug" --json author --jq .author.login) || return 0
  if ! printf ' %s ' "$ISSUE_AUTHOR_ALLOWLIST" | grep -qF " $author "; then
    # Left labelled on purpose. Stripping it would quietly erase the evidence,
    # and only a human with write access can label, so this line IS the alert.
    echo "refusing $slug#$num: author '$author' is not on ISSUE_AUTHOR_ALLOWLIST" >&2
    return 0
  fi

  local title body
  title=$(gh issue view "$num" --repo "$slug" --json title --jq .title)
  body=$(gh issue view "$num" --repo "$slug" --json body --jq .body)
  gh issue edit "$num" --repo "$slug" \
    --remove-label "$LABEL_CLAUDE" --add-label "$LABEL_WORKING" >/dev/null 2>&1 || true

  local wt="$WORKROOT/$repo"
  [ -d "$wt/.git" ] || gh repo clone "$slug" "$wt" -- -q
  git -C "$wt" fetch -q origin
  git -C "$wt" checkout -q -B "agent/issue-$num" origin/HEAD

  local prompt="Resolve GitHub issue #$num: $title

$body

Make the minimal, correct change; keep the build and tests green."

  # Worktrees are cloned on the fly; trust each before claude reads it
  # (no flag for the workspace-trust gate).
  /usr/local/bin/agentbox-trust-dir.sh "$wt" || true
  # Log rather than swallow. This is where an expired OAuth session shows up,
  # and a silent `|| true` made a dead premium lane look like a lane that simply
  # had nothing to say. Output is captured so the notifier can read it, then
  # echoed so the journal keeps what it always had.
  local out
  if ! out=$(cd "$wt" && claude -p "$prompt" --permission-mode acceptEdits 2>&1); then
    echo "premium lane failed for $slug#$num" >&2
    /usr/local/bin/agentbox-notify-auth-expired.sh "escalate $slug#$num" "$out" || true
  fi
  printf '%s\n' "$out"

  # Claude may commit its own work rather than leaving the tree dirty, so a
  # clean tree is not the same as "produced nothing". Ask whether the branch
  # moved off origin/HEAD at all; the watcher had this wrong and threw away a
  # correct fix because of it.
  if [ -z "$(git -C "$wt" status --porcelain)" ] \
     && [ -z "$(git -C "$wt" log --oneline origin/HEAD..HEAD)" ]; then
    # Put it back in the queue instead of stranding it. This function claims the
    # issue as agent-working up front, and the watcher skips that label while
    # this drain only selects needs-claude — so returning here without handing
    # the label back orphans the issue permanently. That is exactly what an
    # expired OAuth session did to photonic_inventory#17.
    echo "no diff for $slug#$num, returning it to the queue" >&2
    gh issue edit "$num" --repo "$slug" \
      --remove-label "$LABEL_WORKING" --add-label "$LABEL_CLAUDE" >/dev/null 2>&1 || true
    return 0
  fi
  if [ -n "$(git -C "$wt" status --porcelain)" ]; then
    git -C "$wt" add -A
    git -C "$wt" commit -q -m "fix: resolve #$num ($title)"
  fi
  git -C "$wt" push -q -u origin "agent/issue-$num"
  gh pr create --repo "$slug" --head "agent/issue-$num" \
    --title "fix: $title (#$num)" \
    --body "Resolves #$num. Shipped by the Claude Code premium lane." || true

  local changed
  changed=$(git -C "$wt" diff --name-only origin/HEAD...HEAD)
  if printf ' %s ' "${AGENTBOX_AUTOMERGE_REPOS:-}" | grep -q " $repo " \
     && no_prod_effect "$changed"; then
    gh pr merge --repo "$slug" --auto --squash "agent/issue-$num" || true
  else
    gh issue edit "$num" --repo "$slug" --add-label "$LABEL_REVIEW" >/dev/null || true
  fi
}

# Triggered single-issue mode: `escalate-to-claude.sh <repo> <issue#>`. Used by
# the alert receiver for immediate critical-alert remediation, and it works for
# ANY repo (the infra repo is deliberately absent from AGENTBOX_REPOS, so the
# periodic drain below never touches it). Never auto-merges unless the repo is
# opted into AGENTBOX_AUTOMERGE_REPOS — homelab isn't, so it opens a PR a human
# merges, the correct autonomy tier for infra (ADR 0001).
if [ "$#" -ge 2 ]; then
  resolve_issue "$1" "$2"
  exit 0
fi

# Periodic drain: every needs-claude issue across the deployable repos.
for repo in ${AGENTBOX_REPOS:-}; do
  issues=$(gh issue list --repo "$OWNER/$repo" --state open --label "$LABEL_CLAUDE" \
    --json number --jq '.[].number') || continue
  for num in $issues; do
    resolve_issue "$repo" "$num"
  done
done
