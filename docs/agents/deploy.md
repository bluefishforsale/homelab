# Deploy gotchas (READ before shipping a change)

These bite every agent that doesn't know them up front:

- **Default branch is `master`, not `main`.** Open PRs against `master`.
- **Merge to `master` auto-deploys to production.** `main-apply.yml` runs a real
  `ansible-playbook` apply (not a dry run) on push to master. Merge IS deploy —
  there is no separate deploy step to gate.
- **CI green is lint/syntax only.** `ci-validate.yml` never touches live hosts, so a
  playbook can pass CI and still fail at deploy. After merging, watch the
  `main-apply.yml` run — that is where the deploy actually succeeds or fails.
- **`main-apply`'s own post-deploy health is `continue-on-error`.** A DEGRADED deploy
  still reports the job green. For real health verification use `health-check.yml`
  (exits 1 on unhealthy), the `scripts/*-check.sh` tools, or `scripts/homelab-health.sh`
  (fleet-wide diff).
- **Squash-merge only.** Keeps one commit per merge, which is what the rest of the
  tooling (rollback's `HEAD~1`, the deploy summary) assumes.
- **A push deploy applies master as it is now, and covers everything since the last
  clean deploy.** `main-apply` diffs from the `deployed` branch (moved forward only
  after an apply with no failures) to the pushed commit, and the apply job checks out
  the tip of master rather than the pushed commit. So a run that waited in the queue
  can't render older templates over a newer deploy, and a run cancelled while
  pending, or one that failed, has its changes picked up by the next push. Only paths
  in main-apply's push filter count, so an inventory or vault commit in the range
  still deploys nothing. Every fleet deploy workflow holds `deploy-ocean`.
  A playbook that keeps failing holds `deployed` back, and every push re-runs it until
  it is fixed. Fix forward. If a change truly must be skipped, fast-forward the
  marker past it by hand (`git push origin <sha>:refs/heads/deployed`; the remote
  refuses anything but a fast-forward). If `deployed` is ever off master's history
  the diff falls back to one commit.
- **Deploy path filter.** `main-apply` runs only on `playbooks/**`, `files/**`,
  `roles/**`, `vars/**/*.yaml` and `group_vars/**`. CI (`ci-validate.yml`) covers most
  of those plus `vault/**` and `.github/workflows/**`, but not `group_vars/**`, so a
  vault-only or workflow-only change runs CI and deploys nothing (dispatch
  `main-apply` by hand for a vault fix). Changes outside both (e.g. `scripts/`,
  `docs/`, `.claude/`) silently get no CI and no apply.
