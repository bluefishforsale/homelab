# Decommissioning a service

Deleting a service's playbook and `files/` dir maps to no owner, which the
detector treats as a clean no-op, so the repo forgets the service while the host
keeps running it. `ra_mirror` was the case: removed in #445, left up, unhealthy
for eight weeks, never alerted. Making removal auto-apply is the wrong fix, since
it turns the merge itself into an unconfirmable teardown. So a decommission is a
one-time, dispatch-only playbook that is gated, phased, verified, then archived.
Was ADR 0002 (2026-09-16).

| Piece | Owns |
|---|---|
| `.github/workflows/decommission.yml` | Dispatch inputs (`mode` report/apply, typed service name, phase toggles, `remove_data` + `DELETE-DATA`), the run, the archive push |
| `roles/decommission/` | Phases, guards, postflight, receipt. Mechanics and variables: its [README](../../roles/decommission/README.md) |
| `playbooks/operations/decommission/<svc>.yaml` | One per teardown, from `_TEMPLATE.yaml` |
| `playbooks/archive/` | Already-run teardowns, one file each, the record of what was destroyed |
| [`docs/operations/decommission.md`](../operations/decommission.md) | Operator runbook |

## Invariants

The gate, phase order, data token, guards and receipt are in the role
[README](../../roles/decommission/README.md). The one piece outside the role is the
archive step: on full success the job `git mv`s the playbook into `playbooks/archive/<date>-<svc>.yaml` and
pushes to master with `[skip ci]`, the one deliberate CI write to master.

NOTE: `playbooks/operations/decommission/*` and `playbooks/archive/*` must stay
excluded at the detector's `emit()` chokepoint
(`.github/workflows/lib/detect-impacted-playbooks.sh`). Lose that and a push,
even a `roles/` reverse-mapping, can schedule a teardown.

## Not in scope

Not a general uninstaller (DNS records, Cloudflare routes, GHCR images and
dashboard tiles are edits in the removal PR), not convergent infrastructure (run
once, never maintained), not a ZFS tool, and never automatic.

## Backout

A report run changes nothing. An apply cannot be un-run: restore data from the
verified tarball and the service from git history, then redeploy its playbook.
