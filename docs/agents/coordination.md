# Multi-agent coordination

Rules for when more than one agent (or a human plus agents) changes this repo.
The repo drives a production homelab, so coordination is about preventing drift,
deploy races, and resource hijacking.

1. **One source of truth.** `master` is desired state; deployed state must equal
   it. Never hand-edit files on a host or run ad-hoc `docker`/`compose` — changes
   flow repo → branch → PR → merge → playbook. Drift is a defect, not a shortcut.

2. **Branch per change; isolated trees.** Never commit to `master`. One logical
   change per PR. Stage explicit paths (`git add <path>`), never `-A` / `.`. Use a
   separate git worktree or clone per concurrent agent; if a clone is shared, run
   `git status` first and leave any foreign uncommitted changes untouched.

3. **PRs are the coordination ledger.** Open a draft PR (or claim an issue) early
   to signal intent; scan open PRs and branches before starting overlapping work.

4. **Serialize deploys; check before triggering.** Host deploys run under the
   `deploy-ocean` concurrency group (`cancel-in-progress: false`) — keep it.
   Before triggering a deploy, confirm nothing else is actively managing or using
   that service. Don't deploy an unmerged branch except for deliberate testing.

5. **Explicit ownership; no hijacking.** Each service has one owner = its playbook
   plus compose path. A task must never rewrite or restart another service to
   borrow it; if you need another instance, run a separate one (own
   dir/container/port) with CPU/RAM caps. The single GPU is allocated
   deliberately — production keeps it; experiments run on CPU or a declared VRAM
   budget. Exception: a docker-compose collection, or a playbook deploying a
   service alongside its exporters/sidecars, is one owned unit — co-deployed
   from the same playbook.

6. **CI-mediated vs manual.** Ordinary ocean services deploy via CI/dispatch.
   The homelab runners deploy through CI too, but never from themselves:
   main-apply's `apply-runners` job applies each runner host from a runner on
   the other one, one host at a time, so the job never recreates the runner it
   is on. Anything else that would kill the job mid-run, and the first apply of
   a runner host that has no homelab runners yet, is applied **manually on the
   homelab network, never via CI**. "Manually on the homelab
   network" means from **agentbox**, not the laptop; before a mutating run,
   confirm no deploy is in flight, because running outside CI means running
   outside the `deploy-ocean` concurrency group that would otherwise serialize
   you. Non-mutating runs (`--check`, `--syntax-check`, report-mode plays) need
   no such ceremony and are the expected way to verify a play from agentbox.

7. **Propose → review → merge → deploy → verify.** Agents propose via PR and
   merge per the authority rule below. The merge is the deploy (`main-apply`
   runs on push to `master`), so the owning agent watches that run and reports
   the health evidence.

8. **Detect drift; fix shared CI first.** Run the apply playbooks in `--check` on
   a schedule so deployed-≠-repo surfaces on its own. When shared CI breaks,
   fixing it unblocks every agent — prioritize it and fix at the root.

**Merge authority:** a session shipping through `/homelab-ship` squash-merges
its own PR once the review gate has no open findings and CI is green, then
watches the deploy. An unhealthy or unverifiable result means halt and alert,
never an automatic rollback. Outside that pipeline, open the PR and leave the
merge to a human. Storage changes are never merged to deploy; see
[storage.md](storage.md).

**Isolation:** a git worktree or clone per concurrent agent is preferred;
shared-clone work is allowed only with strict branch discipline and
explicit-path staging.
