#!/usr/bin/env bash
# agentbox issue watcher (free lane).
# Rendered from files/agentbox/issue-watcher.sh by playbooks/individual/agentbox/agentbox.yaml.
#
# Polls open issues on the deployable repos, drafts a fix on the FREE lanes via
# opencode (local GPU / Gemini per opencode.json), opens a PR with auto-merge,
# and escalates anything it can't resolve to the Claude Code premium lane by
# relabelling. Self-hosted on purpose: opencode's GitHub Action path bills
# metered API, this loop stays on the free tiers.
set -euo pipefail

OWNER="bluefishforsale"
WORKROOT="{{ home }}/repos"
LABEL_WORKING="agent-working"
LABEL_CLAUDE="needs-claude"
LABEL_REVIEW="needs-human-merge"

# shellcheck disable=SC1091
source "{{ home }}/.config/agentbox/agentbox.env"

mkdir -p "$WORKROOT"

# Paths that, if a diff touches anything outside them, mean the change can
# affect prod (Shape-A images, homelab plays, app code). Only when EVERY changed
# file is inside this set is a change "no-prod-effect" and eligible for auto-merge.
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
  # Per-repo/per-lane telemetry labels for everything opencode emits this pass.
  export OTEL_RESOURCE_ATTRIBUTES="repo=$repo,lane=free,service=agentbox"

  # Open issues not already claimed (agent-working) or escalated (needs-claude).
  issues=$(gh issue list --repo "$slug" --state open --json number,labels \
    --jq '.[] | select([.labels[].name] | (contains(["'"$LABEL_WORKING"'"]) or contains(["'"$LABEL_CLAUDE"'"])) | not) | .number') || continue

  for num in $issues; do
    title=$(gh issue view "$num" --repo "$slug" --json title --jq .title)
    body=$(gh issue view "$num" --repo "$slug" --json body --jq .body)
    gh issue edit "$num" --repo "$slug" --add-label "$LABEL_WORKING" >/dev/null

    wt="$WORKROOT/$repo"
    [ -d "$wt/.git" ] || gh repo clone "$slug" "$wt" -- -q
    git -C "$wt" fetch -q origin
    git -C "$wt" checkout -q -B "agent/issue-$num" origin/HEAD

    prompt="Resolve GitHub issue #$num in this repository.
Title: $title

$body

Make the minimal, correct change. Do not touch unrelated code. Keep the build and tests green."

    if (cd "$wt" && opencode run "$prompt") >"$wt/.agent-$num.log" 2>&1 \
       && [ -n "$(git -C "$wt" status --porcelain)" ]; then
      git -C "$wt" add -A
      git -C "$wt" commit -q -m "fix: resolve #$num ($title)"
      git -C "$wt" push -q -u origin "agent/issue-$num"
      gh pr create --repo "$slug" --head "agent/issue-$num" \
        --title "fix: $title (#$num)" \
        --body "Resolves #$num. Drafted by agentbox on the free lane." || true

      # Tiered autonomy (ADR 0001): default is open PR + label + stop, a human
      # merges from the phone. Auto-merge only when the repo opted in AND the
      # diff touches no prod-affecting paths.
      changed=$(git -C "$wt" diff --name-only origin/HEAD...HEAD)
      if printf ' %s ' "${AGENTBOX_AUTOMERGE_REPOS:-}" | grep -q " $repo " \
         && no_prod_effect "$changed"; then
        gh pr merge --repo "$slug" --auto --squash "agent/issue-$num" || true
      else
        gh issue edit "$num" --repo "$slug" --add-label "$LABEL_REVIEW" >/dev/null || true
      fi
    else
      # No usable diff on the free lane -> hand to the Claude Code premium lane.
      gh issue edit "$num" --repo "$slug" \
        --remove-label "$LABEL_WORKING" --add-label "$LABEL_CLAUDE" >/dev/null
    fi
  done

  # Failure-driven escalation: any open agent PR whose CI has gone red is closed
  # and its issue handed to the Claude Code premium lane. Close + delete branch
  # so the escalate lane recreates agent/issue-N from a clean base.
  red=$(gh pr list --repo "$slug" --state open \
    --json number,headRefName,statusCheckRollup \
    --jq '.[] | select(.headRefName|startswith("agent/issue-"))
            | select(any(.statusCheckRollup[]?; .conclusion=="FAILURE" or .state=="FAILURE"))
            | "\(.number) \(.headRefName)"') || red=""
  while read -r prnum ref; do
    [ -z "${ref:-}" ] && continue
    inum=${ref#agent/issue-}
    gh pr close "$prnum" --repo "$slug" --delete-branch >/dev/null 2>&1 || true
    gh issue edit "$inum" --repo "$slug" \
      --remove-label "$LABEL_WORKING" --remove-label "$LABEL_REVIEW" \
      --add-label "$LABEL_CLAUDE" >/dev/null 2>&1 || true
  done <<<"$red"
done
