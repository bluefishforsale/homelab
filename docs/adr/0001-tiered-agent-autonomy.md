# 0001 - Tiered agent autonomy by blast radius

- Status: Accepted
- Date: 2026-06-29
- Deciders: Terrac

## Context

The agentbox fleet (see [CONTEXT.md](../../CONTEXT.md)) watches the open issues
on the deployable repos and drafts fixes as pull requests, on both the free
lane (opencode) and the premium lane (Claude Code). The question is what the
agent is allowed to do with a PR once it opens one.

Every deployable repo auto-deploys to prod on merge to its default branch:

- Shape-A container services build an image, push to GHCR, and
  `repository_dispatch` to homelab, which recreates the compose service.
- homelab itself deploys via `main-apply` on merge.

So a merge is a production change, not a checkpoint. Two facts make blanket
auto-merge wrong here:

- A green check means "it built," not "it is correct." This org's CI has known
  soft spots (see the memory on broken validate/dry-run jobs).
- The draft is written by a model on a free or rationed lane. It is a starting
  point, not a trusted author.

At the same time, requiring a human to babysit every step kills the point of
the fleet. The driver wants to merge from the phone with one gesture, not
review every line at a desk.

## Decision

Autonomy is tiered by blast radius, never blanket.

- **Default: open PR, label `needs-human-merge`, stop.** A human merges from the
  phone or desktop. One gesture, full control.
- **Auto-merge is opt-in and narrow.** `gh pr merge --auto --squash` fires only
  when the repo is in the `AGENTBOX_AUTOMERGE_REPOS` allowlist (default empty)
  **and** the diff touches no prod-affecting paths (docs, README, CONTEXT,
  `*.md`, test/spec dirs). The `no_prod_effect` gate lives in both
  `files/agentbox/issue-watcher.sh` and `files/agentbox/escalate-to-claude.sh`;
  the gate applies regardless of which lane produced the diff, because blast
  radius is a property of the change, not the author.
- **Escalation is failure-driven, not complexity-predicted.** The free lane
  hands an issue to the premium lane (`needs-claude`) only when the draft
  produces no usable diff, or when the opened PR's CI goes red. On red CI the
  watcher closes the PR and deletes the branch so the premium lane recreates
  `agent/issue-N` from a clean base.

## Consequences

- The safe path is the default. Adding a repo to the auto-merge allowlist is a
  deliberate act with a clear ceiling (no-prod-effect diffs only).
- Prod changes always pass through a human, so a wrong draft cannot ship itself.
- Escalation spend stays rationed: Claude is invoked on failure, not on a guess
  about difficulty.
- Cost: the human is in the loop for every prod-affecting merge. That is the
  intended trade, not a regression. If a repo proves it only ever receives
  no-prod-effect agent changes, the allowlist relaxes it without code changes.
