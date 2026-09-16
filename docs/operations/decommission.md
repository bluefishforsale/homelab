# Decommissioning a service

Retiring a service is two halves. Removing it from the repo does **not** remove it
from the host: a deleted path that maps to no playbook is a clean no-op, so the
repo stops describing the service while the host keeps running it. This is the
half that touches the host.

Design rationale and anti-goals: [ADR 0002](../adr/0002-service-decommissioning.md).

## The sequence

1. **Open the removal PR.** Delete the playbook, `files/<svc>/`, and any deploy
   workflow. Sweep the references listed below.
2. **Write the decommission playbook.** Copy
   `playbooks/operations/decommission/_TEMPLATE.yaml` to `<service>.yaml` and fill
   in the unit, container, compose dir, and any data paths.
3. **Dispatch in report mode** and read the plan.
4. **Dispatch in apply mode.** The workflow archives the playbook itself on success.
5. **Confirm the alert is gone** with `scripts/alerts.sh`.

Steps 1 and 2 can ship in the same PR. Merging the PR never runs the teardown.

## Running it

Actions -> **Decommission Service** -> Run workflow.

| input | meaning |
| --- | --- |
| `service` | must match `playbooks/operations/decommission/<service>.yaml` |
| `mode` | `report` (default, changes nothing) or `apply` |
| `confirm` | type the service name exactly; required for `apply` |
| `phase_alerts` / `phase_services` / `phase_files` | run phases independently |
| `remove_data` + `confirm_data` | data removal, needs the literal `DELETE-DATA` |
| `remove_image` | also drop the image from the host |
| `archive` | `git mv` the playbook to `playbooks/archive/` on success |

Report mode walks all three phases and prints exactly what it would do. Always run
it first.

To stage a teardown, run with `phase_alerts=true` and the other two false, then come
back later. A partial run is never archived, so the playbook stays put until the job
is finished.

## What each phase does

**1. Alerts.** Creates a time-boxed Alertmanager silence (default 72h) matching the
unit and container on that host, so the teardown does not page. Time-boxed on
purpose: an abandoned teardown surfaces again rather than hiding forever.

**2. Services.** Stops the unit (its `ExecStop` is usually `docker compose down`),
disables it, runs `systemctl reset-failed`, removes the unit file and any drop-in
dir, reloads systemd, then force-removes the container and optionally the image.

**3. Files.** Removes the compose dir and declared config paths. Data paths are
separate: they are only touched with `remove_data` **and** `confirm_data=DELETE-DATA`,
are tarred to `/data01/decommissioned/` and the tarball verified to list a non-zero
number of members before anything is deleted, and a path that is a ZFS dataset
mountpoint is refused outright.

Postflight re-reads live state and fails if anything survived. A receipt lands at
`/data01/decommissioned/<service>-<stamp>.json`.

## Reference sweep for the removal PR

Grep the service name and its port across the repo. The usual homes:

- `playbooks/individual/**/<service>.yaml` and `files/<service>/`
- `.github/workflows/deploy-<service>.yml`, and any mapping in `pr-deploy.yml`
- `vars/vars_service_ports.yaml`
- `files/ocean-prometheus/prometheus.yml.j2` (scrape job) and `alert_rules.yml.j2`
- blackbox targets, `files/gethomepage/config/services.yaml.j2` (dashboard tile)
- nginx vhosts under `files/nginx-compose/`, and any cloudflared hostname
- `files/db-backup/db-backup.sh` **and** `db-restore.sh` if it has a database

Things the teardown deliberately does **not** do: delete GHCR images, remove DNS or
Cloudflare records, or drop vault keys. Dropping vault keys fails CI and needs a
human. If the service's public hostname moved somewhere else, confirm what serves it
now before deleting anything.

## If the archive push fails

The teardown already succeeded; only the bookkeeping failed, most likely branch
protection on master. The job prints the exact command. Move it by hand:

    git mv playbooks/operations/decommission/<svc>.yaml \
           playbooks/archive/$(date -u +%Y-%m-%d)-<svc>.yaml
