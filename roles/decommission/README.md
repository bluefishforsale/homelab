# decommission

Shared teardown logic for one-time service removals. Consumed only by
`playbooks/operations/decommission/<service>.yaml`, which the **Decommission
Service** workflow dispatches.

Design and anti-goals: [ADR 0002](../../docs/adr/0002-service-decommissioning.md).
Operator runbook: [docs/operations/decommission.md](../../docs/operations/decommission.md).

## Safety model

Report-only unless armed. Nothing destructive runs until `decom_confirm` equals
`decom_service` exactly. Data removal needs a second token on top of that.

The gate is an explicit variable, deliberately not `ansible --check`: a command
task with `check_mode: false` still executes under `--check`, so `--check` is not
a safety boundary here. Same reasoning as `playbooks/operations/backup/db_restore.yaml`.

## Phases

Fixed order, each independently switchable.

| phase | toggle | does |
| --- | --- | --- |
| 1. alerts | `decom_phase_alerts` | time-boxed Alertmanager silence for the unit and container on that host |
| 2. services | `decom_phase_services` | stop, disable, `reset-failed`, remove unit file and drop-ins, daemon-reload, compose down, force-remove container, optionally remove image |
| 3. files | `decom_phase_files` | remove compose dir and config paths; data paths only when separately armed |

Alerts first so the teardown does not page. Services before files because the
unit's `ExecStop` is usually `docker compose down` and needs its compose file to
still exist.

Postflight re-reads live state and fails if the unit, container, or config paths
survived, then writes a receipt to `{{ decom_archive_dir }}/<service>-<stamp>.json`.

## Variables

See `defaults/main.yml`. The ones a per-service playbook sets:

| var | notes |
| --- | --- |
| `decom_service` | required, and the exact string `decom_confirm` must match |
| `decom_reason` | required, one line, lands in the receipt |
| `decom_unit` | `""` to skip the systemd half |
| `decom_container` | `""` to skip the docker half |
| `decom_compose_dir` | treated as CONFIG, removed whenever armed |
| `decom_config_paths` | extra config paths, same treatment |
| `decom_data_paths` | DATA. Tarred, verified, then removed, and only when `decom_remove_data` and `decom_confirm_data=DELETE-DATA` |
| `decom_image` | removed only with `decom_remove_image` |
| `decom_alert_matchers` | override the default silence matchers |

## Guards

- A config path must sit under `/data01/services`, `/etc`, `/opt` or `/srv`, and
  must not be a short top-level path.
- A data path that is a ZFS dataset mountpoint is refused. Dataset work stays a
  coordinated, hand-run, snapshot-first task.
- Data is never deleted behind an unverified tarball: the archive must list a
  non-zero number of members first.
