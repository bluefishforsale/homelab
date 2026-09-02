# Restoring a service database

Every service database on ocean is dumped nightly and pushed to
`gs://homelab-db-backups`. This is how you get one back.

The whole thing is dry-run by default. You have to say `restore_confirm=RESTORE`
before anything touches a live database, and even then the script dumps the
current state first, so a bad restore is reversible.

## The model

One stable object per service, overwritten every run:

```text
gs://homelab-db-backups/<service>/<service>.<ext>.gz
```

Nothing is pruned by the backup job. The bucket has versioning on, so each run
becomes a new generation of the same object and older generations expire after
30 days by a `daysSinceNoncurrentTime` lifecycle rule. That is what makes
point-in-time restore possible: you pick a generation, not a filename.

Everything runs through docker, because ocean has no host `sqlite3` and no host
`gcloud`, and the database files are owned by container UIDs a normal user
cannot read.

## Running one

Restores are dispatch-only. `playbooks/operations/backup/**` never auto-applies
on push, so a merge can never quietly restore a database. Go through
`main-apply.yml` → Run workflow:

```text
playbooks  = playbooks/operations/backup/db_restore.yaml
extra_vars = restore_service=mem0-postgres
```

or from the CLI:

```bash
# Dry run: list the available generations, fetch the current one, change nothing
gh workflow run main-apply.yml \
  --field playbooks=playbooks/operations/backup/db_restore.yaml \
  --field extra_vars='restore_service=mem0-postgres'

# Apply: overwrite the live database from the current backup
gh workflow run main-apply.yml \
  --field playbooks=playbooks/operations/backup/db_restore.yaml \
  --field extra_vars='restore_service=mem0-postgres restore_confirm=RESTORE'

# Apply a specific point in time (generation number from the dry-run listing)
gh workflow run main-apply.yml \
  --field playbooks=playbooks/operations/backup/db_restore.yaml \
  --field extra_vars='restore_service=mem0-postgres restore_point_in_time=1788108347123456 restore_confirm=RESTORE'

# Dry-run everything, e.g. to prove the whole bucket is fetchable
gh workflow run main-apply.yml \
  --field playbooks=playbooks/operations/backup/db_restore.yaml \
  --field extra_vars='restore_service=all'
```

`restore_service` is mandatory. The playbook asserts on an empty value rather
than guessing, so a dispatch with no target fails fast instead of doing
something surprising.

The same playbook is what installs `db-restore.sh` on ocean. `db_backup.yaml`
deploys only the backup half, so on a host that has never run a restore the
script is simply absent until you dispatch this once.

## What a dry run actually does

Read-only steps always run, because they never touch a live database: it lists
every generation of the object, downloads the chosen one to a temp dir, and
gunzips it there. Only the mutating steps are gated, and in dry-run they print
the exact command they would have run instead:

```text
[dry-run] WOULD: docker exec -i mem0-postgres sh -c 'psql -U "${POSTGRES_USER:-postgres}"' < /tmp/.../mem0-postgres.sql
```

That makes a dry run a genuine test of the backup, not just of the tooling. If
the object is missing, truncated, or not valid gzip, the dry run fails there.

A real one, against `mem0-postgres`:

```text
[db-restore] SERVICE=mem0-postgres  POINT_IN_TIME=latest  MODE=DRY-RUN
[db-restore] === mem0-postgres (engine=pg) ===
  available versions (pass the #generation as POINT_IN_TIME):
      10472332  2026-08-30T16:45:42Z  gs://homelab-db-backups/mem0-postgres/mem0-postgres.sql.gz#1788108342043745
[db-restore]   source: gs://homelab-db-backups/mem0-postgres/mem0-postgres.sql.gz (26970905B uncompressed)
    [dry-run] WOULD: docker exec mem0-postgres sh -c 'pg_dumpall ...' > '/data01/backups/db-staging/pre-restore-20260830T171713Z/mem0-postgres.sql'
    [dry-run] WOULD: docker exec -i mem0-postgres sh -c 'psql ...' < '/tmp/tmp.4gxoOnfvKh/mem0-postgres.sql'
[db-restore]   mem0-postgres: DRY-RUN - no changes made
```

That 26970905B matches the size the backup job logged when it made the dump, so
the round trip is confirmed rather than assumed. Worth doing after adding a
service, and worth doing occasionally for the ones you would actually miss.

## What an apply does, per engine

Before any of this, an apply dumps the current state to
`/data01/backups/db-staging/pre-restore-<timestamp>/`. That copy is your undo.

- **mysql** (`grafana-mysql`, `wordpress`): `mysqldump --all-databases` for the
  pre-restore copy, then pipes the backup into `mysql` in the running container.
  The container stays up.
- **pg** (`globalview-timescaledb`, `jellystat-db`, `mem0-postgres`):
  `pg_dumpall` for the pre-restore copy, then pipes the dump into `psql`. Auth
  is the container's local socket (trust), so no password is needed even though
  these all set `POSTGRES_PASSWORD`. The container stays up.
- **sqlite** (everything else): copies the current file aside, **stops the
  container**, replaces the file and removes any stale `-wal` / `-shm`
  sidecars, then starts the container again. Expect a short outage for that one
  service.
- **tar** (`mysql-atrest`): unpacks the archive over the data directory. Only
  for the cold at-rest mysql copy.

## The registry

Service names are exactly these. A name that is not listed is rejected with
`unknown service`.

| Service | Engine | GCS object | Container | Target on disk |
| --- | --- | --- | --- | --- |
| `grafana-mysql` | mysql | `grafana-mysql/grafana-mysql.sql.gz` | `grafana-mysql` |  |
| `wordpress` | mysql | `wordpress/wordpress.sql.gz` | (from `WORDPRESS_DB_CONTAINER`) |  |
| `globalview-timescaledb` | pg | `globalview-timescaledb/globalview-timescaledb.sql.gz` | `globalview-timescaledb` |  |
| `jellystat-db` | pg | `jellystat-db/jellystat-db.sql.gz` | `jellystat-db` |  |
| `mem0-postgres` | pg | `mem0-postgres/mem0-postgres.sql.gz` | `mem0-postgres` |  |
| `mysql-atrest` | tar | `mysql-atrest/mysql-atrest.tar.gz` | `mysql` | `/data01/services/mysql/data` |
| `sonarr` | sqlite | `sonarr/sonarr.db.gz` | `sonarr` | `/data01/services/sonarr/sonarr.db` |
| `radarr` | sqlite | `radarr/radarr.db.gz` | `radarr` | `/data01/services/radarr/radarr.db` |
| `lidarr` | sqlite | `lidarr/lidarr.db.gz` | `lidarr` | `/data01/services/lidarr/lidarr.db` |
| `prowlarr` | sqlite | `prowlarr/prowlarr.db.gz` | `prowlarr` | `/data01/services/prowlarr/config/prowlarr.db` |
| `bazarr` | sqlite | `bazarr/bazarr.db.gz` | `bazarr` | `/data01/services/bazarr/db/bazarr.db` |
| `jellyfin` | sqlite | `jellyfin/jellyfin.db.gz` | `jellyfin` | `/data01/services/jellyfin/config/data/jellyfin.db` |
| `navidrome` | sqlite | `navidrome/navidrome.db.gz` | `navidrome` | `/data01/services/navidrome/data/navidrome.db` |
| `tautulli` | sqlite | `tautulli/tautulli.db.gz` | `tautulli` | `/data01/services/tautulli/config/tautulli.db` |
| `overseerr` | sqlite | `overseerr/overseerr.db.gz` | `overseerr` | `/data01/services/overseerr/config/db/db.sqlite3` |
| `homeassistant` | sqlite | `homeassistant/homeassistant.db.gz` | `homeassistant` | `/data01/services/homeassistant/home-assistant_v2.db` |
| `open-webui` | sqlite | `open-webui/open-webui.db.gz` | `open-webui` | `/data01/services/open-webui/data/webui.db` |
| `paia` | sqlite | `paia/paia.db.gz` | `paia` | `/data01/services/paia/data/paia.db` |
| `photonic_inventory` | sqlite | `photonic_inventory/photonic_inventory.db.gz` | `photonic_inventory` | `/data01/services/photonic_inventory/data/photonic.db` |
| `ntfy` | sqlite | `ntfy/ntfy.db.gz` | `ntfy` | `/data01/services/ntfy/data/auth.db` |
| `my_ta_jose` | sqlite | `my_ta_jose/my_ta_jose.db.gz` | `my_ta_jose` | `/data01/services/my_ta_jose/data/my_ta_jose.db` |
| `mem0` | sqlite | `mem0/mem0.db.gz` | `mem0` | `/data01/services/mem0/history/history.db` |

mem0 is two entries on purpose. `mem0-postgres` is the memory store itself,
vectors and users and api keys. `mem0` is the SQLite audit trail behind
`memory_history`. Losing the second one loses the change history, not the
memories. Restoring the store without the trail is fine; the reverse is not
useful on its own.

## Adding a service

Both halves have to be edited or the service is silently half-covered:

1. `files/db-backup/db-backup.sh` — add the container to the pg or mysql loop,
   or the relative path to `SQLITE_DBS`.
2. `files/db-backup/db-restore.sh` — add a `meta()` arm **and** the name to
   `ALL`. A name in `ALL` with no `meta()` arm is skipped with `unknown service`
   during `restore_service=all`, which looks like a pass unless you read the log.

Then dispatch `db_backup.yaml` to land the script. Neither file deploys on
merge.

## Things that will bite you

- **Uploads are all-or-nothing.** The backup script only uploads when every
  in-scope database dumped cleanly (`${#FAILED[@]} -eq 0`). One broken dump and
  the whole run stays local, and the metric carries the old
  `last_success` forward so staleness keeps growing. Check
  `homelab_db_backup_failed` before trusting a night's objects.
- **Timestamps in the bucket listing are UTC; ocean is PDT.** A listing that
  looks seven hours stale is probably the previous night's run, or your own from
  minutes ago. Compare against `date -u`, not `date`.
- **A sqlite restore stops the container.** Fine for `paia`, less fine for
  `homeassistant` mid-automation. Pick your moment.
- **The pre-restore dump is on `/data01`,** the same pool as everything else. It
  protects against a bad restore, not against losing the pool.

## Related

- Backup job: `playbooks/operations/backup/db_backup.yaml`,
  `files/db-backup/db-backup.sh`
- Alerts: `DbBackupStale` and `DbBackupFailing` in
  `files/ocean-prometheus/alert_rules.yml.j2`
- Metrics: `homelab_db_backup_*` via the node-exporter textfile collector
