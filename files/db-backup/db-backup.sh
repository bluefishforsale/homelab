#!/usr/bin/env bash
# db-backup.sh - dump each in-scope homelab service database and upload it as a
# per-service object to GCS. One stable object per service, overwritten each run
# so bucket VERSIONING keeps the live copy forever and expires older versions
# after 30 days (a lifecycle daysSinceNoncurrentTime rule):
#     gs://<bucket>/<service>/<service>.<ext>.gz
#
# Everything runs through docker: ocean has no host sqlite3/gcloud, and the DB
# files are owned by container UIDs a normal user can't read. RDBMS get LOGICAL
# dumps (mysqldump / pg_dumpall via docker exec); SQLite uses the online .backup
# API (consistent against a live writer). Fails loud and emits homelab_db_backup_*
# to the node_exporter textfile collector.
set -uo pipefail

CONF="${DB_BACKUP_ENV:-/data01/services/db-backup/db-backup.env}"
# shellcheck disable=SC1090
[ -f "$CONF" ] && . "$CONF"

: "${SERVICES_DIR:=/data01/services}"
: "${STAGING:=/data01/backups/db-staging}"
: "${TEXTFILE_DIR:=/data01/services/node-exporter/text_files}"
: "${GCS_BUCKET:=}"
: "${GCS_SA_KEY:=/data01/services/db-backup/gcs-sa.json}"
: "${SQLITE_IMAGE:=keinos/sqlite3:latest}"
: "${GCLOUD_IMAGE:=google/cloud-sdk:slim}"
: "${ALPINE_IMAGE:=alpine:latest}"
: "${MYSQL_ROOT_PASSWORD:=}"
: "${WORDPRESS_DB_CONTAINER:=}"
: "${WORDPRESS_ROOT_PASSWORD:=}"

START=$(date +%s)
WORK="$STAGING/work-$(date -u +%Y%m%dT%H%M%SZ)"
declare -a FAILED=()
log(){ echo "[db-backup] $*"; }
fail(){ FAILED+=("$1"); log "FAIL: $1"; }
running(){ docker ps --format '{{.Names}}' | grep -qx "$1"; }
mkdir -p "$WORK/sqlite" "$TEXTFILE_DIR" || { echo "cannot create staging $WORK"; exit 2; }

# --- Live RDBMS: MySQL / MariaDB -> <service>.sql ---
dump_mysql(){ # container service pw
  local c="$1" svc="$2" pw="$3"
  running "$c" || { log "skip mysql (not running): $c"; return; }
  if docker exec -e MYSQL_PWD="$pw" "$c" \
       mysqldump -u root --single-transaction --routines --triggers --all-databases > "$WORK/$svc.sql" 2>/dev/null \
     && [ -s "$WORK/$svc.sql" ]; then log "dumped $svc ($(wc -c <"$WORK/$svc.sql")B)"; else fail "$svc"; rm -f "$WORK/$svc.sql"; fi
}
dump_mysql grafana-mysql grafana-mysql "$MYSQL_ROOT_PASSWORD"
[ -n "$WORDPRESS_DB_CONTAINER" ] && dump_mysql "$WORDPRESS_DB_CONTAINER" wordpress "$WORDPRESS_ROOT_PASSWORD"

# --- Live RDBMS: Postgres -> <service>.sql (container's own superuser, trust auth) ---
for pgc in globalview-timescaledb jellystat-db; do
  running "$pgc" || { log "skip pg (not running): $pgc"; continue; }
  if docker exec "$pgc" sh -c 'pg_dumpall -U "${POSTGRES_USER:-postgres}"' > "$WORK/$pgc.sql" 2>/dev/null && [ -s "$WORK/$pgc.sql" ]; then
    log "dumped $pgc ($(wc -c <"$WORK/$pgc.sql")B)"; else fail "$pgc"; rm -f "$WORK/$pgc.sql"; fi
done

# --- SQLite: online .backup -> <service>.db (service = first path component) ---
# keinos/sqlite3 runs argv AS the command (tini) and defaults to a non-root user
# that can't read container-owned files or write staging, so 'sqlite3' + --user root.
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
for rel in "${SQLITE_DBS[@]}"; do
  svc="${rel%%/*}"
  docker run --rm -v "$SERVICES_DIR":/svc:ro "$ALPINE_IMAGE" test -f "/svc/$rel" 2>/dev/null \
    || { log "skip sqlite (missing): $rel"; continue; }
  if docker run --rm --user root -v "$SERVICES_DIR":/svc -v "$WORK/sqlite":/out "$SQLITE_IMAGE" \
       sqlite3 "/svc/$rel" ".backup /out/$svc.db" 2>/dev/null && [ -s "$WORK/sqlite/$svc.db" ]; then
    log "backed up sqlite:$svc"; else fail "sqlite:$svc"; rm -f "$WORK/sqlite/$svc.db"; fi
done

# --- At-rest mysql (container stopped): cold copy -> mysql-atrest.tar ---
if [ -d "$SERVICES_DIR/mysql/data" ] && ! running mysql; then
  docker run --rm -v "$SERVICES_DIR/mysql":/src:ro -v "$WORK":/out "$ALPINE_IMAGE" \
    tar cf /out/mysql-atrest.tar -C /src data 2>/dev/null && log "cold-copied at-rest mysql" || fail "mysql-atrest"
fi

# Flatten sqlite backups into $WORK, then gzip every payload.
mv "$WORK"/sqlite/*.db "$WORK"/ 2>/dev/null || true; rmdir "$WORK/sqlite" 2>/dev/null || true
docker run --rm -v "$WORK":/w "$ALPINE_IMAGE" sh -c 'for f in /w/*.sql /w/*.db /w/*.tar; do [ -e "$f" ] && gzip -f "$f"; done'

# --- Upload each payload to gs://bucket/<service>/<file> (service = name before first dot) ---
# Stable object names -> each run is a new version; retention is the bucket's
# noncurrent-version lifecycle, so nothing is pruned here.
UPLOADED=0
if [ -n "$GCS_BUCKET" ] && [ -f "$GCS_SA_KEY" ] && [ "${#FAILED[@]}" -eq 0 ]; then
  if docker run --rm -v "$GCS_SA_KEY":/key.json:ro -v "$WORK":/w "$GCLOUD_IMAGE" sh -c '
        gcloud auth activate-service-account --key-file=/key.json --quiet || exit 1
        rc=0
        for f in /w/*.gz; do
          [ -e "$f" ] || continue
          b=$(basename "$f"); svc=${b%%.*}
          gcloud storage cp "$f" "gs://'"$GCS_BUCKET"'/$svc/$b" || rc=1
        done
        exit $rc' 2>/dev/null; then
    UPLOADED=1; log "uploaded per-service to gs://$GCS_BUCKET/"
  else fail "gcs-upload"; fi
fi
SIZE=$(docker run --rm -v "$WORK":/w "$ALPINE_IMAGE" sh -c 'du -cb /w/*.gz 2>/dev/null | tail -1 | cut -f1' 2>/dev/null || echo 0)

# --- Metric (atomic; carry last_success forward on failure so staleness grows) ---
END=$(date +%s)
prom="$TEXTFILE_DIR/db_backup.prom"; tmp="$prom.$$"
PREV=$(awk '/^homelab_db_backup_last_success_timestamp_seconds /{print $2}' "$prom" 2>/dev/null | tail -1)
if [ "${#FAILED[@]}" -eq 0 ]; then LAST=$END; else LAST="${PREV:-0}"; fi
{
  echo "# HELP homelab_db_backup_last_success_timestamp_seconds Unix time of last fully-successful backup"
  echo "# TYPE homelab_db_backup_last_success_timestamp_seconds gauge"
  echo "homelab_db_backup_last_success_timestamp_seconds $LAST"
  echo "# HELP homelab_db_backup_size_bytes Total compressed bytes uploaded in the last run"
  echo "# TYPE homelab_db_backup_size_bytes gauge"
  echo "homelab_db_backup_size_bytes ${SIZE:-0}"
  echo "# HELP homelab_db_backup_duration_seconds Wall-clock duration of the last run"
  echo "# TYPE homelab_db_backup_duration_seconds gauge"
  echo "homelab_db_backup_duration_seconds $((END-START))"
  echo "# HELP homelab_db_backup_failed Count of in-scope databases that failed in the last run"
  echo "# TYPE homelab_db_backup_failed gauge"
  echo "homelab_db_backup_failed ${#FAILED[@]}"
  echo "# HELP homelab_db_backup_uploaded 1 if the last run uploaded to GCS"
  echo "# TYPE homelab_db_backup_uploaded gauge"
  echo "homelab_db_backup_uploaded $UPLOADED"
} > "$tmp" && mv -f "$tmp" "$prom"

rm -rf "$WORK"
if [ "${#FAILED[@]}" -ne 0 ]; then log "DONE with ${#FAILED[@]} failure(s): ${FAILED[*]}"; exit 1; fi
log "DONE ok: uploaded=$UPLOADED size=${SIZE}B"
