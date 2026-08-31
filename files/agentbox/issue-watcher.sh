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
# Whose issues the fleet is willing to take instructions from, space separated.
# GitHub cannot restrict who opens an issue on a public repo, so this is where
# that boundary has to live. Overridable from agentbox.env for the day a second
# trusted human exists; defaulting to the owner alone is the safe direction to
# be wrong in.
ISSUE_AUTHOR_ALLOWLIST="${ISSUE_AUTHOR_ALLOWLIST:-$OWNER}"
# Separate from repos/ (the RC sessions' cwd) so the watcher's clone + commits
# can't collide with a live remote-control session on the same repo.
WORKROOT="{{ home }}/work"
# Draft logs live OUTSIDE the worktree so `git add -A` never commits them.
LOGDIR="{{ home }}/agent-logs"
LABEL_WORKING="agent-working"
LABEL_CLAUDE="needs-claude"
LABEL_REVIEW="needs-human-merge"

# shellcheck disable=SC1091
source "{{ home }}/.config/agentbox/agentbox.env"

mkdir -p "$WORKROOT" "$LOGDIR"

# gh add-label fails if the label doesn't exist in the repo; create them first.
ensure_labels() {
  local slug="$1"
  gh label create "$LABEL_WORKING" --repo "$slug" --color FBCA04 --description "Agent fleet is drafting a fix" --force >/dev/null 2>&1 || true
  gh label create "$LABEL_CLAUDE"  --repo "$slug" --color 5319E7 --description "Escalated to the Claude Code premium lane" --force >/dev/null 2>&1 || true
  gh label create "$LABEL_REVIEW"  --repo "$slug" --color D93F0B --description "Agent PR open; a human merges" --force >/dev/null 2>&1 || true
}

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
  ensure_labels "$slug"

  # Open issues not already claimed (agent-working) or escalated (needs-claude),
  # AND authored by someone on the allowlist.
  #
  # The author check is the security boundary. An issue title and body are fed
  # verbatim into the prompt below, and the agent that reads them clones the
  # repo, writes code and opens a PR, which for an allowlisted repo can then
  # auto-merge. That makes an issue an instruction channel into a process with
  # write access, so it must only accept instructions from people entitled to
  # give them.
  #
  # Every watched repo is private today, which is the only reason this was not
  # already exploitable. That is an accident of configuration, not a control:
  # the day one goes public, or an outside collaborator is added, anyone could
  # queue work for the fleet. Trust belongs in the code, stated out loud.
  #
  # NOTE: gh's own --jq takes a single expression and does NOT accept --arg;
  # passing one makes gh reject the entire command, so the allowlist has to
  # reach jq through a real pipe. Get this wrong and the loop below silently
  # processes nothing.
  issues=$(gh issue list --repo "$slug" --state open --json number,labels,author \
    | jq -r --arg allow "$ISSUE_AUTHOR_ALLOWLIST" '
      .[]
      | select([.labels[].name] | (contains(["'"$LABEL_WORKING"'"]) or contains(["'"$LABEL_CLAUDE"'"])) | not)
      | select(.author.login as $a | ($allow | split(" ")) | index($a))
      | .number') || { echo "issue list failed for $slug, skipping" >&2; continue; }

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

    if (cd "$wt" && opencode run "$prompt") >"$LOGDIR/$repo-issue-$num.log" 2>&1 \
       && [ -n "$(git -C "$wt" status --porcelain)" ]; then
      git -C "$wt" add -A
      git -C "$wt" commit -q -m "fix: resolve #$num ($title)"
      git -C "$wt" push -q -u origin "agent/issue-$num"
      pr_url=$(gh pr create --repo "$slug" --head "agent/issue-$num" \
        --title "fix: $title (#$num)" \
        --body "Resolves #$num. Drafted by agentbox on the free lane.") || pr_url=""

      # Independent premium-lane review: a stronger model (Claude, on the
      # fixed-cost subscription) reviews the free lane's draft and comments. The
      # human still merges — this only informs that decision. Diff is passed in
      # the prompt (no tools), so no extra permissions are needed.
      if [ -n "$pr_url" ]; then
        /usr/local/bin/agentbox-trust-dir.sh "$wt" || true
        diff=$(git -C "$wt" diff origin/HEAD...HEAD); diff=${diff:0:60000}
        review=$( cd "$wt" && OTEL_RESOURCE_ATTRIBUTES="repo=$repo,lane=claude,service=agentbox" \
          claude -p "Review this agent-drafted diff resolving issue #$num ($title) in $slug. Assess correctness, security, and whether it actually fixes the issue. Be concise: bullet concrete problems, otherwise reply LGTM.

$diff" 2>/dev/null ) || review=""
        [ -n "$review" ] && gh pr comment "$pr_url" --repo "$slug" \
          --body "Premium-lane review (Claude):

$review" >/dev/null 2>&1 || true
      fi

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
  #
  # Author-filtered for the same reason the issue list above is. This loop reads
  # an issue NUMBER out of a branch name and hands that issue to the premium
  # lane, so an unfiltered version lets anyone who can open a PR pick the issue
  # that gets escalated: fork the repo, push agent/issue-<N>, let CI go red, and
  # issue N is queued for an agent no matter who wrote it. The lane's own author
  # check is the backstop; this keeps the label from being forged in the first
  # place. Agent PRs are opened with the owner's PAT, so they pass.
  red=$(gh pr list --repo "$slug" --state open \
    --json number,headRefName,statusCheckRollup,author \
    | jq -r --arg allow "$ISSUE_AUTHOR_ALLOWLIST" '
      .[] | select(.headRefName|startswith("agent/issue-"))
          | select(.author.login as $a | ($allow | split(" ")) | index($a))
          | select(any(.statusCheckRollup[]?; .conclusion=="FAILURE" or .state=="FAILURE"))
          | "\(.number) \(.headRefName)"') || { echo "pr list failed for $slug" >&2; red=""; }
  while read -r prnum ref; do
    [ -z "${ref:-}" ] && continue
    inum=${ref#agent/issue-}
    gh pr close "$prnum" --repo "$slug" --delete-branch >/dev/null 2>&1 || true
    gh issue edit "$inum" --repo "$slug" \
      --remove-label "$LABEL_WORKING" --remove-label "$LABEL_REVIEW" \
      --add-label "$LABEL_CLAUDE" >/dev/null 2>&1 || true
  done <<<"$red"
done
