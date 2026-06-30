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

for repo in ${AGENTBOX_REPOS:-}; do
  slug="$OWNER/$repo"
  # Per-repo/per-lane telemetry labels for everything claude emits this pass.
  export OTEL_RESOURCE_ATTRIBUTES="repo=$repo,lane=claude,service=agentbox"
  ensure_labels "$slug"
  issues=$(gh issue list --repo "$slug" --state open --label "$LABEL_CLAUDE" \
    --json number --jq '.[].number') || continue

  for num in $issues; do
    title=$(gh issue view "$num" --repo "$slug" --json title --jq .title)
    body=$(gh issue view "$num" --repo "$slug" --json body --jq .body)
    gh issue edit "$num" --repo "$slug" \
      --remove-label "$LABEL_CLAUDE" --add-label "$LABEL_WORKING" >/dev/null

    wt="$WORKROOT/$repo"
    [ -d "$wt/.git" ] || gh repo clone "$slug" "$wt" -- -q
    git -C "$wt" fetch -q origin
    git -C "$wt" checkout -q -B "agent/issue-$num" origin/HEAD

    prompt="Resolve GitHub issue #$num: $title

$body

Make the minimal, correct change; keep the build and tests green."

    # Worktrees are cloned on the fly; trust each before claude reads it
    # (no flag for the workspace-trust gate).
    /usr/local/bin/agentbox-trust-dir.sh "$wt" || true
    (cd "$wt" && claude -p "$prompt" --permission-mode acceptEdits) || true

    if [ -n "$(git -C "$wt" status --porcelain)" ]; then
      git -C "$wt" add -A
      git -C "$wt" commit -q -m "fix: resolve #$num ($title)"
      git -C "$wt" push -q -u origin "agent/issue-$num"
      gh pr create --repo "$slug" --head "agent/issue-$num" \
        --title "fix: $title (#$num)" \
        --body "Resolves #$num. Shipped by the Claude Code premium lane." || true

      changed=$(git -C "$wt" diff --name-only origin/HEAD...HEAD)
      if printf ' %s ' "${AGENTBOX_AUTOMERGE_REPOS:-}" | grep -q " $repo " \
         && no_prod_effect "$changed"; then
        gh pr merge --repo "$slug" --auto --squash "agent/issue-$num" || true
      else
        gh issue edit "$num" --repo "$slug" --add-label "$LABEL_REVIEW" >/dev/null || true
      fi
    fi
  done
done
