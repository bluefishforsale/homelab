# playbooks/archive

Completed one-time playbooks, kept as the record of what was torn down and when.

A decommission playbook lands here automatically: the **Decommission Service**
workflow does a `git mv` out of `playbooks/operations/decommission/` after the run
succeeds, and pushes it to master with `[skip ci]` so the move cannot trigger a
deploy. Filenames are prefixed with the run date.

Nothing in this directory is runnable. The deploy detector treats
`playbooks/archive/**` as never-auto-apply, so an archived play can never be
scheduled by a push, and a `files/` or `roles/` change can never reverse-map into
one. To re-run something here, copy it back out and dispatch it deliberately.

The matching receipts (what was actually removed, and where any data tarball
went) live on the target host under `/data01/decommissioned/`.
