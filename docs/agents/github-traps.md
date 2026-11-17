# GitHub API / CLI traps (each of these cost hours)

- **The Checks API is restricted to GitHub Apps.** No PAT can read check runs, and
  the `Checks` permission is not offered in the fine-grained token UI at all — on a
  new token or an old one. Do not go looking for the checkbox, and do not "fix" it
  with a classic `repo`-scoped token. `gh pr list --json statusCheckRollup` expands
  to the per-check `contexts` and 403s forever; the GraphQL rollup **aggregate**
  (`commits(last:1){nodes{commit{statusCheckRollup{state}}}}`) needs no extra
  permission and is what the retired agentbox issue watcher used.
- **`gh`'s `--jq` is not jq.** One expression, no `--arg`. Passing one makes `gh`
  reject the whole command. Pipe to real jq: `gh ... --json ... | jq -r --arg ...`.
- **`x-accepted-github-permissions`** on any response names the exact permission an
  endpoint wants. Read it instead of guessing at a 403.
- **Listings lie.** Google's models endpoint advertises models that error on every
  call (`gemini-2.5-pro`, 2026-08-31). Round-trip a model before wiring it in, and
  remember reachability is not capability — make it do the real task.
- **Two similarly-named checks are not the same check.** This repo has both
  `Validate Ansible Playbooks` (`ci-validate.yml`) and `Validate Playbooks`
  (`pr-deploy.yml`). Match job names exactly via `gh run view <id> --json jobs`.
