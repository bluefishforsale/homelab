#!/usr/bin/env bash
# agentbox issue watcher (free lane).
# Rendered from files/agentbox/issue-watcher.sh by playbooks/individual/agentbox/agentbox.yaml.
#
# Polls open issues on the deployable repos and drafts a fix by walking a model
# ladder, cheapest first: the local GPU, then Gemini Flash. Opens a PR, and
# relabels anything neither could resolve for the escalation tier (Gemini Pro,
# agentbox-escalate.sh). Self-hosted on purpose: opencode's GitHub Action path
# bills metered API, and every tier here is free.
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
LABEL_ESCALATE="needs-escalation"
LABEL_REVIEW="needs-human-merge"

# The lane ladder, cheapest first. Local is free and unlimited, so it takes
# every issue; Google's free quota is only spent on what the GPU could not do.
# Measured: the local model resolved a real issue end to end in 81s, including
# running git commands, which is why it drafts rather than merely triages.
# NOTE: `gemini-flash-latest` is an alias on purpose. Pinning bit us — 2.5-pro
# is still advertised by the models endpoint but errors on every call. A draft
# quality shift from an alias moving is self-correcting here (bad draft -> red
# CI -> escalate), whereas a model that vanishes takes the tier out entirely.
LANES="local-llama/local google/gemini-flash-latest"
# Escalation (tier 3) is a separate service; see agentbox-escalate.sh.
# No working `-latest` alias exists for pro: gemini-pro-latest returns a server
# error, so this one is pinned and must be re-verified when it stops answering.
REVIEW_MODEL="google/gemini-3.1-pro-preview"

# shellcheck disable=SC1091
source "{{ home }}/.config/agentbox/agentbox.env"

mkdir -p "$WORKROOT" "$LOGDIR"

# gh add-label fails if the label doesn't exist in the repo; create them first.
ensure_labels() {
  local slug="$1"
  gh label create "$LABEL_WORKING" --repo "$slug" --color FBCA04 --description "Agent fleet is drafting a fix" --force >/dev/null 2>&1 || true
  gh label create "$LABEL_ESCALATE" --repo "$slug" --color 5319E7 --description "Both free lanes failed; escalated to the harder model" --force >/dev/null 2>&1 || true
  gh label create "$LABEL_REVIEW"  --repo "$slug" --color D93F0B --description "Agent PR open; a human merges" --force >/dev/null 2>&1 || true
}

# Paths that, if a diff touches anything outside them, mean the change can
# affect prod (Shape-A images, homelab plays, app code). Only when EVERY changed
# file is inside this set is a change "no-prod-effect" and eligible for auto-merge.
NOPROD_RE='^(docs/|README|CONTEXT|.*\.md$|.*_test\.|test/|tests/|spec/)'

# Did the drafter produce anything? The agent may or may not commit its own
# work — opencode's build agent usually does — so a clean tree does NOT mean it
# failed. Testing only `status --porcelain` discarded a correct, already
# committed fix and escalated it to the premium lane, which is why the free lane
# looked like it never succeeded.
has_draft() {  # $1 = worktree
  [ -n "$(git -C "$1" status --porcelain)" ] && return 0
  [ -n "$(git -C "$1" log --oneline origin/HEAD..HEAD)" ]
}

# One attempt per tier. A capability failure means escalate, not retry: the same
# model on the same prompt fails the same way, and eight tries buys eight
# identical failures. Only INFRASTRUCTURE failure is worth a second go, so that
# retries once in place — timeout (exit 124) or a rate limit in the output.
draft() {  # $1 = worktree, $2 = model, $3 = prompt, $4 = logfile
  local wt="$1" model="$2" prompt="$3" log="$4" rc
  printf '\n===== %s =====\n' "$model" >>"$log"
  ( cd "$wt" && timeout 900 opencode run -m "$model" "$prompt" ) >>"$log" 2>&1
  rc=$?
  if [ "$rc" -eq 124 ] || tail -40 "$log" | grep -qiE '429|rate limit|quota exceeded|RESOURCE_EXHAUSTED'; then
    echo "  $model: infrastructure failure (rc=$rc), retrying once" >&2
    printf '\n===== %s (retry) =====\n' "$model" >>"$log"
    ( cd "$wt" && timeout 900 opencode run -m "$model" "$prompt" ) >>"$log" 2>&1
    rc=$?
  fi
  return "$rc"
}

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

  # Open issues not already claimed (agent-working) or escalated (needs-escalation),
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
      | select([.labels[].name] | (contains(["'"$LABEL_WORKING"'"]) or contains(["'"$LABEL_ESCALATE"'"])) | not)
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

    # Walk the ladder. Each tier starts from a clean origin/HEAD so a failed
    # attempt's debris is never attributed to the model that follows it.
    log="$LOGDIR/$repo-issue-$num.log"
    : >"$log"
    drafted=""
    for model in $LANES; do
      git -C "$wt" checkout -q -B "agent/issue-$num" origin/HEAD
      git -C "$wt" clean -qfd
      echo "$slug#$num: drafting on $model" >&2
      if draft "$wt" "$model" "$prompt" "$log" && has_draft "$wt"; then
        drafted="$model"
        echo "$slug#$num: $model produced a diff" >&2
        break
      fi
      echo "$slug#$num: $model produced no usable diff" >&2
    done

    if [ -n "$drafted" ]; then
      # Only commit what the drafter left loose; committing nothing exits 1 and
      # would abort the run under set -e.
      if [ -n "$(git -C "$wt" status --porcelain)" ]; then
        git -C "$wt" add -A
        git -C "$wt" commit -q -m "fix: resolve #$num ($title)"
      fi
      git -C "$wt" push -q -u origin "agent/issue-$num"
      pr_url=$(gh pr create --repo "$slug" --head "agent/issue-$num" \
        --title "fix: $title (#$num)" \
        --body "Resolves #$num. Drafted by agentbox on \`$drafted\`.") || pr_url=""

      # Independent review by the strongest free model. A human still merges;
      # this only informs that decision.
      #
      # Runs from LOGDIR, not the worktree, with the diff inline: a reviewer
      # that can edit the branch it is reviewing is not a reviewer. --agent plan
      # is read-only, and the cwd holds no repo, so both belt and braces.
      if [ -n "$pr_url" ]; then
        diff=$(git -C "$wt" diff origin/HEAD...HEAD); diff=${diff:0:60000}
        review=$( cd "$LOGDIR" && OTEL_RESOURCE_ATTRIBUTES="repo=$repo,lane=review,service=agentbox" \
          timeout 600 opencode run --agent plan -m "$REVIEW_MODEL" \
          "Review this agent-drafted diff resolving issue #$num ($title) in $slug. It was written by $drafted. Assess correctness, security, and whether it actually fixes the issue. Be concise: bullet concrete problems, otherwise reply LGTM.

$diff" 2>/dev/null ) || review=""
        [ -n "$review" ] && gh pr comment "$pr_url" --repo "$slug" \
          --body "Independent review ($REVIEW_MODEL):

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
      # Every free lane on the ladder failed -> hand to the escalation tier.
      gh issue edit "$num" --repo "$slug" \
        --remove-label "$LABEL_WORKING" --add-label "$LABEL_ESCALATE" >/dev/null
    fi
  done

  # Failure-driven escalation: any open agent PR whose CI has gone red is closed
  # and its issue handed to the escalation tier. Close + delete branch
  # so the escalate lane recreates agent/issue-N from a clean base.
  #
  # Author-filtered for the same reason the issue list above is. This loop reads
  # an issue NUMBER out of a branch name and hands that issue to the premium
  # lane, so an unfiltered version lets anyone who can open a PR pick the issue
  # that gets escalated: fork the repo, push agent/issue-<N>, let CI go red, and
  # issue N is queued for an agent no matter who wrote it. The lane's own author
  # check is the backstop; this keeps the label from being forged in the first
  # place. Agent PRs are opened with the owner's PAT, so they pass.
  # NOTE: asks GraphQL for the rollup's aggregate `state`, not gh's canned
  # --json statusCheckRollup. They are not interchangeable: gh expands that field
  # into the per-check `contexts` list, which a fine-grained PAT cannot read
  # without the Checks permission — and that permission is not offered for
  # user-owned tokens at all, so this drain silently returned nothing forever:
  #   Resource not accessible by personal access token
  #     (repository.pullRequests.nodes.0.statusCheckRollup...contexts.nodes.0)
  # The aggregate state needs no extra permission, and red-or-not is all we want.
  red=$(gh api graphql -f owner="$OWNER" -f name="$repo" -f query='
      query($owner:String!, $name:String!) {
        repository(owner:$owner, name:$name) {
          pullRequests(first:50, states:OPEN) {
            nodes {
              number headRefName
              author { login }
              commits(last:1) { nodes { commit { statusCheckRollup { state } } } }
            }
          }
        }
      }' \
    | jq -r --arg allow "$ISSUE_AUTHOR_ALLOWLIST" '
      .data.repository.pullRequests.nodes[]
          | select(.headRefName|startswith("agent/issue-"))
          | select(.author.login as $a | ($allow | split(" ")) | index($a))
          | select((.commits.nodes[0].commit.statusCheckRollup // {state:"NONE"}).state
                   | . == "FAILURE" or . == "ERROR")
          | "\(.number) \(.headRefName)"') || { echo "pr list failed for $slug" >&2; red=""; }
  while read -r prnum ref; do
    [ -z "${ref:-}" ] && continue
    inum=${ref#agent/issue-}
    gh pr close "$prnum" --repo "$slug" --delete-branch >/dev/null 2>&1 || true
    gh issue edit "$inum" --repo "$slug" \
      --remove-label "$LABEL_WORKING" --remove-label "$LABEL_REVIEW" \
      --add-label "$LABEL_ESCALATE" >/dev/null 2>&1 || true
  done <<<"$red"
done
