# Agent autonomy (agentbox)

What an agentbox draft may do once it opens a PR. Every deployable repo deploys
on merge, so a merge is a production change, and a green check only means it
built. Autonomy is tiered by blast radius, never blanket. Was ADR 0001
(2026-06-29); this is the deployed form.

## Lanes

All unattended work runs on free models. A model that already failed twice is
the worst thing to spend money on.

| Tier | Where | Model |
|---|---|---|
| 1, 2 | `files/agentbox/issue-watcher.sh` (`LANES`) | `local-llama/local`, then `google/gemini-flash-latest` |
| review | same, `REVIEW_MODEL` | `google/gemini-3.1-pro-preview` must answer `LGTM` |
| 3 | `files/agentbox/escalate.sh` | strongest free model |
| human | `needs-human-merge` (PR open, human merges); `needs-human` when tier 3 also fails (`escalate.sh`) | |

Claude Code is not a lane. Headless it cannot run shell commands and its OAuth
expires needing an interactive login, which stalled this tier for a month
(`escalate.sh` header). It stays installed for RC sessions, where a human is
present.

## Rules

- Default: open PR, label `needs-human-merge`, stop. A human merges.
- Auto-merge (`gh pr merge --auto --squash`) only when the repo is in
  `AGENTBOX_AUTOMERGE_REPOS` (default empty, homelab is not in it) and
  `no_prod_effect` accepts every changed path
  (`NOPROD_RE='^(docs/|README|CONTEXT|.*\.md$|.*_test\.|test/|tests/|spec/)'`).
  Both `issue-watcher.sh` and `escalate.sh` apply that gate, because blast
  radius belongs to the change, not the author. The watcher also requires the
  review to pass; a failed review un-readies the PR and labels it
  `agent-review-flagged`. `escalate.sh` has no review step.
- Escalation is failure-driven: every free lane yielding no diff, or an agent
  PR's CI rollup going `FAILURE`/`ERROR`. On red CI the watcher closes the PR with
  `--delete-branch` and relabels the issue `needs-escalation`, so tier 3
  recreates `agent/issue-N` from a clean base.
- Issues labelled `alert`, `hardware` or `needs-human` reach a drafter only once a
  human adds `agent-ok`. Alert-born PRs merged 32% against 61% for human-written
  issues, and a model told to resolve a CPU alert raises the threshold (#588).
- Only PRs from `ISSUE_AUTHOR_ALLOWLIST` authors are escalated, so a stranger's
  PR cannot queue work.

NOTE: widening `NOPROD_RE` or adding a repo to `AGENTBOX_AUTOMERGE_REPOS` lets
model-written diffs reach production with no human. Both are deliberate acts.

## Backout

Empty `AGENTBOX_AUTOMERGE_REPOS` and every PR waits for a human. Stopping the
`agentbox-issue-watcher` and `agentbox-escalate` timers stops drafting.
