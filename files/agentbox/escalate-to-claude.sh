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
WORKROOT="{{ home }}/repos"
LABEL_WORKING="agent-working"
LABEL_CLAUDE="needs-claude"

# shellcheck disable=SC1091
source "{{ home }}/.config/agentbox/agentbox.env"
unset ANTHROPIC_API_KEY  # the env file must not set it; enforce here too

for repo in ${AGENTBOX_REPOS:-}; do
  slug="$OWNER/$repo"
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

    (cd "$wt" && claude -p "$prompt" --permission-mode acceptEdits) || true

    if [ -n "$(git -C "$wt" status --porcelain)" ]; then
      git -C "$wt" add -A
      git -C "$wt" commit -q -m "fix: resolve #$num ($title)"
      git -C "$wt" push -q -u origin "agent/issue-$num"
      gh pr create --repo "$slug" --head "agent/issue-$num" \
        --title "fix: $title (#$num)" \
        --body "Resolves #$num. Shipped by the Claude Code premium lane." || true
      gh pr merge --repo "$slug" --auto --squash "agent/issue-$num" || true
    fi
  done
done
