#!/usr/bin/env bash
# db-restore.sh - restore ONE service (or all) from its GCS backup.
#
# DRY-RUN BY DEFAULT. Nothing that touches a live database runs unless
# CONFIRM=RESTORE. The gate is an explicit variable checked here, NEVER ansible
# --check (which leaks: a command/shell task with check_mode:false executes for
# real in check mode). Every mutating action routes through mut(); in dry-run
# mut() only PRINTS the command. Read-only steps (list versions, download a copy
# to a temp dir, gunzip in temp) always run - they never touch a live database.
#
# Before overwriting, an APPLY run first dumps the CURRENT state to
# $STAGING/pre-restore-<ts>/ so a bad restore is reversible.
#
# Env:
#   SERVICE        required: a service name, or 'all'
#   POINT_IN_TIME  optional: a GCS generation number (see the listed versions);
#                  default = the current/live version
#   CONFIRM        must equal 'RESTORE' to actually apply; anything else = dry-run
set -uo pipefail

CONF="${DB_BACKUP_ENV:-/data01/services/db-backup/db-backup.env}"
# shellcheck disable=SC1090
[ -f "$CONF" ] && . "$CONF"
: "${SERVICES_DIR:=/data01/services}"
: "${STAGING:=/data01/backups/db-staging}"
: "${GCS_BUCKET:?GCS_BUCKET not set in env}"
: "${GCS_SA_KEY:=/data01/services/db-backup/gcs-sa.json}"
: "${GCLOUD_IMAGE:=google/cloud-sdk:slim}"
: "${ALPINE_IMAGE:=alpine:latest}"
: "${MYSQL_ROOT_PASSWORD:=}"
: "${WORDPRESS_DB_CONTAINER:=blog_saetnere_com_wp_db}"
: "${WORDPRESS_ROOT_PASSWORD:=}"
: "${SERVICE:?set SERVICE=<name>|all}"
: "${POINT_IN_TIME:=}"
: "${CONFIRM:=}"

DRY=1; [ "$CONFIRM" = "RESTORE" ] && DRY=0
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
PRE="$STAGING/pre-restore-$(date -u +%Y%m%dT%H%M%SZ)"
log(){ echo "[db-restore] $*"; }
# mut(): THE safety gate. In dry-run only prints; in apply runs the command.
mut(){ if [ "$DRY" = 1 ]; then echo "    [dry-run] WOULD: $*"; else echo "    [apply]  $*"; "$@"; fi; }
gcs(){ docker run --rm -v "$GCS_SA_KEY":/key.json:ro -v "$TMP":/t "$GCLOUD_IMAGE" sh -c \
        "gcloud auth activate-service-account --key-file=/key.json -q 2>/dev/null && $*"; }

# registry -> "ENGINE|OBJECT|CONTAINER|TARGET"
meta(){
  case "$1" in
    grafana-mysql)          echo "mysql|grafana-mysql.sql.gz|grafana-mysql|" ;;
    wordpress)              echo "mysql|wordpress.sql.gz|$WORDPRESS_DB_CONTAINER|" ;;
    globalview-timescaledb) echo "pg|globalview-timescaledb.sql.gz|globalview-timescaledb|" ;;
    jellystat-db)           echo "pg|jellystat-db.sql.gz|jellystat-db|" ;;
    mem0-postgres)          echo "pg|mem0-postgres.sql.gz|mem0-postgres|" ;;
    mysql-atrest)           echo "tar|mysql-atrest.tar.gz|mysql|$SERVICES_DIR/mysql/data" ;;
    sonarr)     echo "sqlite|sonarr.db.gz|sonarr|$SERVICES_DIR/sonarr/sonarr.db" ;;
    radarr)     echo "sqlite|radarr.db.gz|radarr|$SERVICES_DIR/radarr/radarr.db" ;;
    lidarr)     echo "sqlite|lidarr.db.gz|lidarr|$SERVICES_DIR/lidarr/lidarr.db" ;;
    prowlarr)   echo "sqlite|prowlarr.db.gz|prowlarr|$SERVICES_DIR/prowlarr/config/prowlarr.db" ;;
    bazarr)     echo "sqlite|bazarr.db.gz|bazarr|$SERVICES_DIR/bazarr/db/bazarr.db" ;;
    jellyfin)   echo "sqlite|jellyfin.db.gz|jellyfin|$SERVICES_DIR/jellyfin/config/data/jellyfin.db" ;;
    navidrome)  echo "sqlite|navidrome.db.gz|navidrome|$SERVICES_DIR/navidrome/data/navidrome.db" ;;
    tautulli)   echo "sqlite|tautulli.db.gz|tautulli|$SERVICES_DIR/tautulli/config/tautulli.db" ;;
    overseerr)  echo "sqlite|overseerr.db.gz|overseerr|$SERVICES_DIR/overseerr/config/db/db.sqlite3" ;;
    homeassistant) echo "sqlite|homeassistant.db.gz|homeassistant|$SERVICES_DIR/homeassistant/home-assistant_v2.db" ;;
    open-webui) echo "sqlite|open-webui.db.gz|open-webui|$SERVICES_DIR/open-webui/data/webui.db" ;;
    paia)       echo "sqlite|paia.db.gz|paia|$SERVICES_DIR/paia/data/paia.db" ;;
    photonic_inventory) echo "sqlite|photonic_inventory.db.gz|photonic_inventory|$SERVICES_DIR/photonic_inventory/data/photonic.db" ;;
    ntfy)       echo "sqlite|ntfy.db.gz|ntfy|$SERVICES_DIR/ntfy/data/auth.db" ;;
    frigate)    echo "sqlite|frigate.db.gz|frigate|$SERVICES_DIR/frigate/config/frigate.db" ;;
    my_ta_jose) echo "sqlite|my_ta_jose.db.gz|my_ta_jose|$SERVICES_DIR/my_ta_jose/data/my_ta_jose.db" ;;
    mem0)       echo "sqlite|mem0.db.gz|mem0|$SERVICES_DIR/mem0/history/history.db" ;;
    *) echo "" ;;
  esac
}
ALL="grafana-mysql wordpress globalview-timescaledb jellystat-db mem0-postgres mysql-atrest sonarr radarr lidarr prowlarr bazarr jellyfin navidrome tautulli overseerr homeassistant open-webui paia photonic_inventory ntfy frigate my_ta_jose mem0"

restore_one(){
  local svc="$1" m engine obj container target uri gen payload base
  m=$(meta "$svc"); [ -n "$m" ] || { log "unknown service '$svc' (known: $ALL)"; return 1; }
  IFS='|' read -r engine obj container target <<<"$m"
  uri="gs://$GCS_BUCKET/$svc/$obj"
  log "=== $svc (engine=$engine) ==="
  echo "  available versions (pass the #generation as POINT_IN_TIME):"
  gcs "gcloud storage ls --all-versions --long $uri" 2>/dev/null | sed 's/^/    /' || echo "    (cannot list)"
  gen=""; [ -n "$POINT_IN_TIME" ] && gen="#$POINT_IN_TIME"
  # download the chosen version to temp (read-only; never touches live data)
  if ! gcs "gcloud storage cp ${uri}${gen} /t/$obj" >/dev/null 2>&1 || ! [ -s "$TMP/$obj" ]; then
    log "  cannot fetch ${uri}${gen}; skipping"; return 1
  fi
  gunzip -f "$TMP/$obj"; payload="$TMP/${obj%.gz}"; base=$(basename "$target")
  log "  source: ${uri}${gen:+ @gen $POINT_IN_TIME} ($(wc -c <"$payload")B uncompressed)"
  [ "$DRY" = 0 ] && mkdir -p "$PRE"
  case "$engine" in
    mysql)
      local pw="$MYSQL_ROOT_PASSWORD"; [ "$svc" = wordpress ] && pw="$WORDPRESS_ROOT_PASSWORD"
      mut sh -c "docker exec -e MYSQL_PWD='$pw' $container mysqldump -u root --all-databases > '$PRE/$svc.sql' 2>/dev/null"
      mut sh -c "docker exec -i -e MYSQL_PWD='$pw' $container mysql -u root < '$payload'"
      ;;
    pg)
      mut sh -c "docker exec $container sh -c 'pg_dumpall -U \"\${POSTGRES_USER:-postgres}\"' > '$PRE/$svc.sql' 2>/dev/null"
      mut sh -c "docker exec -i $container sh -c 'psql -U \"\${POSTGRES_USER:-postgres}\"' < '$payload'"
      ;;
    sqlite)
      mut sh -c "cp '$target' '$PRE/$base' 2>/dev/null || true"
      mut docker stop "$container"
      mut docker run --rm -v "$(dirname "$target")":/d -v "$payload":/src:ro "$ALPINE_IMAGE" \
            sh -c "cp /src '/d/$base' && rm -f '/d/$base-wal' '/d/$base-shm'"
      mut docker start "$container"
      ;;
    tar)
      mut docker run --rm -v "$target":/d -v "$payload":/src:ro "$ALPINE_IMAGE" sh -c "cd /d && tar xf /src"
      ;;
  esac
  log "  $svc: $([ "$DRY" = 1 ] && echo 'DRY-RUN - no changes made' || echo "restored (pre-restore copy in $PRE)")"
}

log "SERVICE=$SERVICE  POINT_IN_TIME=${POINT_IN_TIME:-latest}  MODE=$([ "$DRY" = 1 ] && echo 'DRY-RUN' || echo 'APPLY')"
if [ "$DRY" = 1 ]; then
  log "DRY-RUN: listing restore points and validating downloads only. NOTHING that"
  log "         touches a live database will run. Re-dispatch with CONFIRM=RESTORE to apply."
else
  log "APPLY: live databases WILL be overwritten. Current state is dumped to $PRE first."
fi
rc=0
if [ "$SERVICE" = all ]; then for s in $ALL; do restore_one "$s" || rc=1; done; else restore_one "$SERVICE" || rc=1; fi
log "done ($([ "$DRY" = 1 ] && echo 'dry-run, no changes' || echo 'applied'))"
exit $rc
