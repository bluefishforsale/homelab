#!/usr/bin/env bash
# db-backup.sh - dump every in-scope homelab service database into one tarball
# and (when configured) push it to GCS. Reads optional config from db-backup.env.
#
# Design notes (why it looks like this):
#  - Everything runs through docker, never host tooling: ocean has neither
#    `sqlite3` nor `gcloud`, and the DB files are owned by container UIDs the
#    invoking user can't read. A root-in-container process reads them fine.
#  - RDBMS get LOGICAL dumps (mysqldump / pg_dumpall via `docker exec`); copying
#    a live InnoDB/Postgres data dir raw yields a torn, unrestorable image.
#  - SQLite uses the online `.backup` API, which is consistent against a live
#    writer (a plain cp of a WAL db is not).
#  - Fails loud: any in-scope DB that fails is recorded and the script exits 1,
#    and the success timestamp metric is NOT advanced (so DbBackupStale fires).
#  - Retention lives in the GCS bucket lifecycle rule, not here; on a successful
#    upload the local tarball is deleted so backups never accumulate on the pool
#    they are protecting.
set -uo pipefail

CONF="${DB_BACKUP_ENV:-/data01/services/db-backup/db-backup.env}"
# shellcheck disable=SC1090
[ -f "$CONF" ] && . "$CONF"

: "${SERVICES_DIR:=/data01/services}"
: "${STAGING:=/data01/backups/db-staging}"
: "${TEXTFILE_DIR:=/data01/services/node-exporter/text_files}"
: "${GCS_BUCKET:=}"                                    # empty -> local-only (Phase 0)
: "${GCS_SA_KEY:=/data01/services/db-backup/gcs-sa.json}"
: "${SQLITE_IMAGE:=keinos/sqlite3:latest}"
: "${GCLOUD_IMAGE:=google/cloud-sdk:slim}"
: "${ALPINE_IMAGE:=alpine:latest}"
: "${MYSQL_ROOT_PASSWORD:=}"                           # grafana Percona root
: "${WORDPRESS_DB_CONTAINER:=}"                        # e.g. blog_saetnere_com_wp_db
: "${WORDPRESS_ROOT_PASSWORD:=}"

START=$(date +%s)
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
WORK="$STAGING/work-$STAMP"
TARBALL="$STAGING/homelab-db-$STAMP.tar.gz"
declare -a FAILED=()
log(){ echo "[db-backup] $*"; }
fail(){ FAILED+=("$1"); log "FAIL: $1"; }
running(){ docker ps --format '{{.Names}}' | grep -qx "$1"; }

mkdir -p "$WORK/sqlite" "$TEXTFILE_DIR" || { echo "cannot create staging $WORK"; exit 2; }

# --- Live RDBMS: MySQL / MariaDB (needs root pw, passed as MYSQL_PWD env) ---
dump_mysql_all(){ # container outfile pw
  local c="$1" out="$2" pw="$3"
  running "$c" || { log "skip mysql (not running): $c"; return; }
  if docker exec -e MYSQL_PWD="$pw" "$c" \
       mysqldump -u root --single-transaction --routines --triggers --all-databases > "$out" 2>/dev/null \
     && [ -s "$out" ]; then log "dumped mysql:$c ($(wc -c <"$out")B)"; else fail "mysql:$c"; fi
}
dump_mysql_all grafana-mysql "$WORK/grafana-mysql.sql" "$MYSQL_ROOT_PASSWORD"
[ -n "$WORDPRESS_DB_CONTAINER" ] && \
  dump_mysql_all "$WORDPRESS_DB_CONTAINER" "$WORK/wordpress.sql" "$WORDPRESS_ROOT_PASSWORD"

# --- Live RDBMS: Postgres (local superuser trust, no cred needed) ---
for pgc in globalview-timescaledb jellystat-db; do
  running "$pgc" || { log "skip pg (not running): $pgc"; continue; }
  # Use the container's OWN superuser (globalview's is 'globalview', not 'postgres'); local socket is trust-auth.
  if docker exec "$pgc" sh -c 'pg_dumpall -U "${POSTGRES_USER:-postgres}"' > "$WORK/$pgc.sql" 2>/dev/null && [ -s "$WORK/$pgc.sql" ]; then
    log "dumped pg:$pgc ($(wc -c <"$WORK/$pgc.sql")B)"; else fail "pg:$pgc"; fi
done

# --- SQLite: online .backup via a throwaway container (root reads any owner) ---
SQLITE_DBS=(
  sonarr/sonarr.db radarr/radarr.db lidarr/lidarr.db
  prowlarr/config/prowlarr.db bazarr/db/bazarr.db
  jellyfin/config/data/jellyfin.db navidrome/data/navidrome.db
  tautulli/config/tautulli.db overseerr/config/db/db.sqlite3
  homeassistant/home-assistant_v2.db open-webui/data/webui.db
  paia/data/paia.db photonic_inventory/data/photonic.db
  ntfy/data/auth.db frigate/config/frigate.db
  my_ta_jose/data/my_ta_jose.db
)
# Excluded: portainer.db — Portainer holds an exclusive lock on it so online
# .backup is flaky, and it's low-value recreatable Docker-UI state (endpoints/
# stacks/users), not irreplaceable data.
for rel in "${SQLITE_DBS[@]}"; do
  src="$SERVICES_DIR/$rel"
  if ! docker run --rm -v "$SERVICES_DIR":/svc:ro "$ALPINE_IMAGE" test -f "/svc/$rel" 2>/dev/null; then
    log "skip sqlite (missing): $rel"; continue
  fi
  d="/svc/$(dirname "$rel")"; f="$(basename "$rel")"; out="$(echo "$rel" | tr '/' '_')"
  # source dir mounted rw so sqlite can open a WAL db; .backup only reads source.
  # NOTE: keinos/sqlite3 runs the given argv AS the command (tini), so 'sqlite3' is
  # explicit; and it defaults to a NON-root user that can't read container-owned db
  # files or write the staging dir, so force --user root.
  if docker run --rm --user root -v "$SERVICES_DIR":/svc -v "$WORK/sqlite":/out "$SQLITE_IMAGE" \
       sqlite3 "$d/$f" ".backup /out/$out" 2>/dev/null && [ -s "$WORK/sqlite/$out" ]; then
    log "backed up sqlite:$rel"; else fail "sqlite:$rel"; fi
done

# --- At-rest RDBMS (container stopped): cold-copy the data dir via container ---
if [ -d "$SERVICES_DIR/mysql/data" ] && ! running mysql; then
  docker run --rm -v "$SERVICES_DIR/mysql":/src:ro -v "$WORK":/out "$ALPINE_IMAGE" \
    tar czf /out/mysql-atrest-datadir.tar.gz -C /src data 2>/dev/null \
    && log "cold-copied at-rest mysql" || fail "atrest:mysql"
fi

# --- Package one tarball ---
docker run --rm -v "$WORK":/w -v "$STAGING":/s "$ALPINE_IMAGE" \
  tar czf "/s/$(basename "$TARBALL")" -C /w . 2>/dev/null || fail "tar"
SIZE=$(docker run --rm -v "$STAGING":/s "$ALPINE_IMAGE" stat -c%s "/s/$(basename "$TARBALL")" 2>/dev/null || echo 0)

# --- Upload to GCS (skipped when unset -> Phase 0) ---
UPLOADED=0
if [ -n "$GCS_BUCKET" ] && [ -f "$GCS_SA_KEY" ] && [ "${#FAILED[@]}" -eq 0 ]; then
  if docker run --rm -v "$GCS_SA_KEY":/key.json:ro -v "$STAGING":/s:ro "$GCLOUD_IMAGE" sh -c \
      "gcloud auth activate-service-account --key-file=/key.json --quiet && \
       gcloud storage cp /s/$(basename "$TARBALL") gs://$GCS_BUCKET/db-backups/" 2>/dev/null; then
    UPLOADED=1; rm -f "$TARBALL"; log "uploaded to gs://$GCS_BUCKET/db-backups/"
  else fail "gcs-upload"; fi
fi

# Local staging retention: keep the 2 newest tarballs (bounds pool growth, and
# leaves a local fallback when GCS upload is off or failing).
ls -1t "$STAGING"/homelab-db-*.tar.gz 2>/dev/null | tail -n +3 | xargs -r rm -f

# --- Metric (atomic; carry last_success forward on failure so staleness grows) ---
END=$(date +%s)
prom="$TEXTFILE_DIR/db_backup.prom"; tmp="$prom.$$"
PREV=$(awk '/^homelab_db_backup_last_success_timestamp_seconds /{print $2}' "$prom" 2>/dev/null | tail -1)
if [ "${#FAILED[@]}" -eq 0 ]; then LAST=$END; else LAST="${PREV:-0}"; fi
{
  echo "# HELP homelab_db_backup_last_success_timestamp_seconds Unix time of last fully-successful backup"
  echo "# TYPE homelab_db_backup_last_success_timestamp_seconds gauge"
  echo "homelab_db_backup_last_success_timestamp_seconds $LAST"
  echo "# HELP homelab_db_backup_size_bytes Size of the last backup tarball in bytes"
  echo "# TYPE homelab_db_backup_size_bytes gauge"
  echo "homelab_db_backup_size_bytes ${SIZE:-0}"
  echo "# HELP homelab_db_backup_duration_seconds Wall-clock duration of the last run"
  echo "# TYPE homelab_db_backup_duration_seconds gauge"
  echo "homelab_db_backup_duration_seconds $((END-START))"
  echo "# HELP homelab_db_backup_failed Count of in-scope databases that failed in the last run"
  echo "# TYPE homelab_db_backup_failed gauge"
  echo "homelab_db_backup_failed ${#FAILED[@]}"
  echo "# HELP homelab_db_backup_uploaded 1 if the last tarball reached GCS"
  echo "# TYPE homelab_db_backup_uploaded gauge"
  echo "homelab_db_backup_uploaded $UPLOADED"
} > "$tmp" && mv -f "$tmp" "$prom"

rm -rf "$WORK"
if [ "${#FAILED[@]}" -ne 0 ]; then log "DONE with ${#FAILED[@]} failure(s): ${FAILED[*]}"; exit 1; fi
log "DONE ok: $TARBALL (${SIZE}B) uploaded=$UPLOADED"
