# 0002 - Service decommissioning is a gated one-time playbook, not a merge

- Status: Accepted
- Date: 2026-09-16
- Deciders: Terrac

## Context

Removing a service from this repo does not remove it from the fleet.

The deploy detector maps a changed file to the playbook that owns it. Deleting a
service's playbook and `files/` dir maps to **nothing**, by design: a deleted
path that resolves to no owner is a clean no-op, not an error. So the repo stops
describing the service while the host keeps running it.

`ra_mirror` is the worked example. radiantatmospheres.com was rebuilt on
Cloudflare Pages, making the on-host nginx mirror redundant. Deleting its
definition (PR #445) left the container up, the unit enabled, and the compose dir
in place. It had already been `unhealthy` for eight weeks with a failing streak of
166049 and never alerted, because nothing re-pulled its image to trip the refresh
gate. An orphan is worse than a live service: nothing owns it, nothing updates
it, and the thing that would have reported it is the monitoring it fell out of.

The obvious fix, "make the removal auto-apply", is wrong. It makes *merging* the
destructive act, with no report step, no typed confirmation, and no way to stop
after silencing alerts. Merge-is-deploy is acceptable for convergent changes
because a bad apply can be re-applied; a teardown cannot be un-run.

## Decision

A decommission is a **one-time, dispatch-only playbook** that is gated, phased,
verified, and then retired.

**Report by default.** Nothing destructive happens until `decom_confirm` equals
the service name exactly. A default run walks all three phases and prints the
plan. The gate is an explicit variable, never `ansible --check`: a command task
with `check_mode: false` still executes under `--check`, so `--check` is not a
safety boundary (the same reasoning already applied in `db_restore.yaml`).

**Fixed phase order: alerts, then services, then files.**

1. *Alerts* first, so the teardown never pages. The silence is time-boxed, so an
   abandoned teardown surfaces again instead of hiding forever.
2. *Services* next. The unit is stopped before its files are deleted, because its
   `ExecStop` is usually `docker compose down` and it needs the compose file to
   still exist. Deleting files first orphans the container instead of removing it.
   `systemctl reset-failed` runs here, which is what actually clears the alert.
3. *Files* last, split in two. Config is removed whenever the run is armed; data
   needs its own second token (`DELETE-DATA`), is tarred and the tarball verified
   to list a non-zero number of members before anything is deleted, and refuses
   outright to touch a ZFS dataset mountpoint.

**Each phase is independently switchable**, so "silence it today, tear it down on
Friday" is a supported sequence rather than an all-or-nothing button.

**Verify, then retire.** Postflight re-reads live state and fails if the unit,
container, or config paths survived. On a complete successful apply, the CI job
`git mv`s the playbook into `playbooks/archive/` and pushes to master with
`[skip ci]`. A receipt of what was actually removed lands on the target host in
`/data01/decommissioned/`.

**Nothing under `operations/decommission/**` or `archive/**` can auto-apply.**
Enforced at the detector's single `emit()` chokepoint and pinned by four tests,
so not even a `roles/` reverse-mapping can schedule a teardown from a push.

## Anti-goals

- **Not a general uninstaller.** It removes what the playbook declares. It does
  not hunt for stray DNS records, Cloudflare routes, GHCR images, or dashboard
  tiles. Those are repo-side edits in the removal PR, and the runbook lists them.
- **Not a replacement for the removal PR.** The repo change and the teardown are
  two halves. This is the half that touches the host.
- **Not idempotent infrastructure.** A decommission playbook is written once, run
  once, and archived. It is not maintained afterwards and must never be part of a
  convergent apply.
- **Not a ZFS tool.** It will not remove a dataset mountpoint. Pool and dataset
  work stays a coordinated, hand-run, snapshot-first task.
- **Not automatic.** No schedule, no push trigger, no "clean up orphans" sweep. A
  human decides, types the name, and watches it.

## Consequences

- Retiring a service is now two merges plus one dispatch: the removal PR, the
  dispatched teardown, and the archive commit the workflow pushes itself.
- `playbooks/archive/` grows one file per decommission. That is the point: it is
  the record of what was destroyed and why.
- The archive push writes to master from CI, which is the one deliberate
  exception to "never commit to master". It is confined to moving an already-run
  playbook, carries `[skip ci]`, and fails loudly if branch protection blocks it.
