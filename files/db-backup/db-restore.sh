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
#   MODE           'verify' loads each backup into a throwaway, network-less
#                  container instead of the live one, checks it, and writes
#                  homelab_db_restore_test_* to TEXTFILE_DIR. It never restores.
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
: "${MODE:=restore}"
: "${TEXTFILE_DIR:=}"
: "${SQLITE_IMAGE:=keinos/sqlite3:latest}"

if [ "$MODE" = verify ]; then
  [ "$CONFIRM" = RESTORE ] && { echo "[db-restore] MODE=verify never restores; unset CONFIRM" >&2; exit 2; }
  : "${TEXTFILE_DIR:?TEXTFILE_DIR not set (verify mode writes its metrics there)}"
fi
DRY=1; [ "$CONFIRM" = "RESTORE" ] && [ "$MODE" != verify ] && DRY=0
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
    homeassistant) echo "sqlite|homeassistant.db.gz|homeassistant|$SERVICES_DIR/homeassistant/config/home-assistant_v2.db" ;;
    open-webui) echo "sqlite|open-webui.db.gz|open-webui|$SERVICES_DIR/open-webui/data/webui.db" ;;
    paia)       echo "sqlite|paia.db.gz|paia|$SERVICES_DIR/paia/data/paia.db" ;;
    photonic_inventory) echo "sqlite|photonic_inventory.db.gz|photonic_inventory|$SERVICES_DIR/photonic_inventory/data/photonic.db" ;;
    ntfy)       echo "sqlite|ntfy.db.gz|ntfy|$SERVICES_DIR/ntfy/data/auth.db" ;;
    my_ta_jose) echo "sqlite|my_ta_jose.db.gz|my_ta_jose|$SERVICES_DIR/my_ta_jose/data/my_ta_jose.db" ;;
    mem0)       echo "sqlite|mem0.db.gz|mem0|$SERVICES_DIR/mem0/history/history.db" ;;
    *) echo "" ;;
  esac
}
ALL="grafana-mysql wordpress jellystat-db mem0-postgres mysql-atrest sonarr radarr lidarr prowlarr bazarr jellyfin navidrome tautulli overseerr homeassistant open-webui paia photonic_inventory ntfy my_ta_jose mem0"

# verify registry -> "IMAGE|SQL|MAX_AGE_HOURS". SQL returns the epoch of the newest
# record; NULL (empty table) fails. MAX_AGE only for services that write on their
# own schedule, since a quiet app's newest row is legitimately old.
probe(){
  case "$1" in
    grafana-mysql) echo "percona/percona-server:5.7|SELECT MAX(last_execution) FROM grafana.server_lock|48" ;;
    wordpress)     echo "mariadb:10.11|SELECT UNIX_TIMESTAMP(MAX(post_modified_gmt)) FROM wordpress.wp_posts|" ;;
    jellystat-db)  echo 'postgres:15.2|\connect jfstat \\ SELECT EXTRACT(EPOCH FROM MAX("TimeRun"))::bigint FROM jf_logging|48' ;;
    mem0-postgres) echo 'ankane/pgvector:v0.5.1|\connect mem0_app \\ SELECT EXTRACT(EPOCH FROM MAX(created_at))::bigint FROM request_logs|48' ;;
    mysql-atrest)  echo "percona/percona-server:5.7|SELECT UNIX_TIMESTAMP(MAX(last_update)) FROM mysql.innodb_table_stats|" ;;
    sonarr|radarr|prowlarr) echo "|SELECT strftime('%s',substr(MAX(LastStartTime),1,19)) FROM ScheduledTasks|48" ;;
    lidarr)        echo "|SELECT strftime('%s',substr(MAX(LastStartTime),1,19)) FROM ScheduledTasks|" ;;
    bazarr)        echo "|SELECT MAX(timestamp) FROM table_history|" ;;
    jellyfin)      echo "|SELECT strftime('%s',substr(MAX(DateCreated),1,19)) FROM ActivityLogs|" ;;
    navidrome)     echo "|SELECT strftime('%s',substr(MAX(last_scan_at),1,19)) FROM library|" ;;
    tautulli)      echo "|SELECT MAX(started) FROM session_history|" ;;
    overseerr)     echo "|SELECT strftime('%s',substr(MAX(updatedAt),1,19)) FROM media|" ;;
    homeassistant) echo "|SELECT CAST(MAX(last_updated_ts) AS INTEGER) FROM states|48" ;;
    open-webui)    echo "|SELECT MAX(updated_at) FROM message|" ;;
    paia)          echo "|SELECT strftime('%s',substr(MAX(started_at),1,19)) FROM turn|" ;;
    photonic_inventory) echo "|SELECT strftime('%s',substr(MAX(updated_at),1,19)) FROM reservation|" ;;
    ntfy)          echo "|SELECT MAX(created) FROM user|" ;;
    my_ta_jose)    echo "|SELECT strftime('%s',substr(MAX(created_at),1,19)) FROM grade_job|" ;;
    mem0)          echo "|SELECT strftime('%s',substr(MAX(created_at),1,19)) FROM history|" ;;
  esac
}

# Throwaway containers carry this label so a run that died mid-test is swept by the next.
VLABEL=db-restore-test
sweep(){ docker ps -aq --filter "label=$VLABEL" | xargs -r docker rm -fv >/dev/null 2>&1; }
# wait_log <container> <pattern>: the image's init has finished (or the container died).
wait_log(){
  for _ in $(seq 1 120); do
    docker logs "$1" 2>&1 | grep -q "$2" && return 0
    [ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" = true ] || break
    sleep 2
  done
  log "  $1 never logged '$2':"; docker logs --tail 15 "$1" 2>&1 | sed 's/^/    /'; return 1
}
until_ok(){ for _ in $(seq 1 30); do "$@" >/dev/null 2>&1 && return 0; sleep 2; done; return 1; }

declare -A V_OK=() V_LAST=()
verify_one(){
  local svc="$1" engine="$2" payload="$3" image sql maxage c="$VLABEL-$1" out err="$TMP/$1.err" v
  IFS='|' read -r image sql maxage <<<"$(probe "$svc")"
  [ -n "$sql" ] || { log "  no probe registered for $svc"; return 1; }
  case "$engine" in
    mysql)
      docker run -d --name "$c" --label "$VLABEL" --network none \
        -e MYSQL_ALLOW_EMPTY_PASSWORD=1 -e MARIADB_ALLOW_EMPTY_ROOT_PASSWORD=1 "$image" >/dev/null || return 1
      wait_log "$c" "port: 3306" && until_ok docker exec "$c" mysql -uroot -e 'SELECT 1' || return 1
      # Load and probe in ONE session: a dump that FLUSHes PRIVILEGES swaps in prod's
      # root password, which only an already-authenticated session survives.
      out=$( { cat "$payload"; printf '\n%s;\n' "$sql"; } | docker exec -i "$c" mysql -uroot -N 2>"$err") \
        || { log "  load failed:"; tail -5 "$err" | sed 's/^/    /'; return 1; }
      ;;
    pg)
      docker run -d --name "$c" --label "$VLABEL" --network none -e POSTGRES_HOST_AUTH_METHOD=trust "$image" >/dev/null || return 1
      wait_log "$c" "init process complete" && until_ok docker exec "$c" pg_isready -U postgres -q || return 1
      out=$( { cat "$payload"; printf '\n%s;\n' "$sql"; } | docker exec -i "$c" psql -X -q -t -A -U postgres -d postgres 2>"$err")
      # pg_dumpall always re-creates the superuser the fresh image already has.
      if grep 'ERROR' "$err" | grep -vq 'role "postgres" already exists'; then
        log "  load failed:"; grep ERROR "$err" | head -5 | sed 's/^/    /'; return 1
      fi
      ;;
    sqlite)
      out=$(docker run --rm --network none --user root -v "$payload":/db "$SQLITE_IMAGE" \
              sqlite3 /db "PRAGMA integrity_check; $sql;" 2>"$err")
      [ "$(head -1 <<<"$out")" = ok ] || { log "  integrity_check failed:"; head -5 <<<"$out" | sed 's/^/    /'; cat "$err"; return 1; }
      ;;
    tar)
      mkdir -p "$TMP/$svc" && tar xf "$payload" -C "$TMP/$svc" || return 1
      docker run -d --name "$c" --label "$VLABEL" --network none --user root -v "$TMP/$svc/data":/var/lib/mysql \
        "$image" --user=root --skip-grant-tables --skip-networking >/dev/null || return 1
      wait_log "$c" "ready for connections" || return 1
      if docker logs "$c" 2>&1 | grep -qiE "crash recovery|not shut ?down normally"; then
        log "  cold copy needed crash recovery: it was taken from a running server"; return 1
      fi
      out=$(docker exec "$c" mysql -uroot -N -e "$sql" 2>"$err") || { cat "$err"; return 1; }
      ;;
  esac
  docker rm -fv "$c" >/dev/null 2>&1
  v=$(tail -1 <<<"$out" | tr -d '[:space:]'); v=${v%%.*}
  [[ "$v" =~ ^[0-9]+$ ]] && [ "$v" -gt 0 ] || { log "  probe returned '$v', not a record time: empty or missing table"; return 1; }
  V_LAST[$svc]=$v
  log "  newest record $(date -u -d "@$v" +%FT%TZ 2>/dev/null || echo "$v")"
  if [ -n "$maxage" ] && [ $(( $(date +%s) - v )) -gt $(( maxage * 3600 )) ]; then
    log "  newest record is older than ${maxage}h: the backup is of a database nobody is writing"; return 1
  fi
  V_OK[$svc]=1
}

write_verify_metrics(){
  local prom="$TEXTFILE_DIR/db_restore_test.prom" tmp s prev last now
  tmp="$prom.$$"; now=$(date +%s)
  prev=$(awk '/^homelab_db_restore_test_last_success_timestamp_seconds /{print $2}' "$prom" 2>/dev/null | tail -1)
  last=$now; [ "$1" -eq 0 ] || last="${prev:-0}"
  {
    echo "# HELP homelab_db_restore_test_ok 1 if the latest backup loaded into a throwaway instance and passed its probe"
    echo "# TYPE homelab_db_restore_test_ok gauge"
    for s in $ALL; do echo "homelab_db_restore_test_ok{service=\"$s\"} ${V_OK[$s]:-0}"; done
    echo "# HELP homelab_db_restore_test_last_record_timestamp_seconds Newest record found in the restored backup"
    echo "# TYPE homelab_db_restore_test_last_record_timestamp_seconds gauge"
    for s in $ALL; do [ -n "${V_LAST[$s]:-}" ] && echo "homelab_db_restore_test_last_record_timestamp_seconds{service=\"$s\"} ${V_LAST[$s]}"; done
    echo "# HELP homelab_db_restore_test_last_success_timestamp_seconds Unix time of the last run where every backup restored"
    echo "# TYPE homelab_db_restore_test_last_success_timestamp_seconds gauge"
    echo "homelab_db_restore_test_last_success_timestamp_seconds $last"
    echo "# HELP homelab_db_restore_test_duration_seconds Wall-clock duration of the last run"
    echo "# TYPE homelab_db_restore_test_duration_seconds gauge"
    echo "homelab_db_restore_test_duration_seconds $(( now - START ))"
  } > "$tmp" && mv -f "$tmp" "$prom"
}

restore_one(){
  local svc="$1" m engine obj container target uri gen payload base
  m=$(meta "$svc"); [ -n "$m" ] || { log "unknown service '$svc' (known: $ALL)"; return 1; }
  IFS='|' read -r engine obj container target <<<"$m"
  uri="gs://$GCS_BUCKET/$svc/$obj"
  log "=== $svc (engine=$engine) ==="
  if [ "$MODE" != verify ]; then
    echo "  available versions (pass the #generation as POINT_IN_TIME):"
    gcs "gcloud storage ls --all-versions --long $uri" 2>/dev/null | sed 's/^/    /' || echo "    (cannot list)"
  fi
  gen=""; [ -n "$POINT_IN_TIME" ] && gen="#$POINT_IN_TIME"
  # download the chosen version to temp (read-only; never touches live data)
  if ! gcs "gcloud storage cp ${uri}${gen} /t/$obj" >/dev/null 2>&1 || ! [ -s "$TMP/$obj" ]; then
    log "  cannot fetch ${uri}${gen}; skipping"; return 1
  fi
  gunzip -f "$TMP/$obj"; payload="$TMP/${obj%.gz}"; base=$(basename "$target")
  log "  source: ${uri}${gen:+ @gen $POINT_IN_TIME} ($(wc -c <"$payload")B uncompressed)"
  if [ "$MODE" = verify ]; then verify_one "$svc" "$engine" "$payload"; local r=$?; rm -rf "$payload" "${TMP:?}/$svc"; return $r; fi
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

if [ "$MODE" = verify ]; then
  START=$(date +%s); trap 'sweep; rm -rf "$TMP"' EXIT; sweep
  log "VERIFY: SERVICE=$SERVICE  POINT_IN_TIME=${POINT_IN_TIME:-latest}. Throwaway containers only; no live database is touched."
  rc=0
  if [ "$SERVICE" = all ]; then
    for s in $ALL; do restore_one "$s" || { rc=1; log "  FAIL: $s"; }; done
    write_verify_metrics "$rc"
  else
    restore_one "$SERVICE" || { rc=1; log "  FAIL: $SERVICE"; }
  fi
  log "verify done: $([ "$rc" = 0 ] && echo 'every backup restored' || echo 'FAILURES above')"
  exit $rc
fi

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
