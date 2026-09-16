---
description: Pull the next open GitHub issue and take it through implement → test → PR.
---

# /next-task

Find and execute the next actionable GitHub issue in the current repo. Issue-driven
(the agentbox loop's model), adapted from the plex `dev-loop`/`next-task` pattern.

1. `gh issue list --state open` — pick the next actionable issue. Respect labels and
   any stated PR sequence; skip anything blocked or awaiting review.
2. Read the issue, then the repo's conventions (`CLAUDE.md` / `agents.md`) and 2–3
   existing source files closest to what's needed.
3. **If the task needs >200 lines of new code:** spawn a `Plan` sub-agent (`model:
   opus`) for signatures / data flow / edge cases before writing any code.
4. **Implement in a worktree + branch** (worktree rule is non-negotiable in homelab,
   default elsewhere). Follow the repo's existing patterns; stage explicit paths.
5. Run the repo's test command; fix failures before proceeding.
6. Open a PR that closes the issue (`Closes #N`). Follow the repo's ship/CI flow — for
   homelab that means `/homelab-ship`; elsewhere a normal PR against the default branch.
7. Report what was built and the next actionable issue.

Model assignments: planning → `opus`, implementation/troubleshooting → `sonnet`,
pure mechanical edits → `haiku`.
