#!/usr/bin/env bash
# mem0-metrics.sh - publish mem0 statistics to the node_exporter textfile
# collector, the same way db-backup.sh publishes homelab_db_backup_*.
#
# mem0 ships no /metrics endpoint, but it already records every request into
# request_logs (method, path, status, latency_ms, auth_type, 30-day retention),
# so query counts and real latency come out of the app database rather than out
# of an exporter we would have to write into the service.
#
# Everything runs through docker: ocean has no host psql, and the SQLite audit
# trail is owned by a container UID.
set -uo pipefail

: "${TEXTFILE_DIR:=/data01/services/node-exporter/text_files}"
: "${PG_CONTAINER:=mem0-postgres}"
: "${API_CONTAINER:=mem0}"
: "${EMBED_MODEL:=nomic-embed-homelab}"
# Requests are counted over a window rather than as a monotonic counter:
# request_logs is pruned on retention, and a counter that shrinks makes rate()
# lie. A gauge over a fixed window is honest about what it is.
: "${WINDOW_MIN:=5}"
# Latency needs a wider window or a quiet homelab leaves every quantile empty.
: "${LAT_WINDOW_MIN:=60}"

START=$(date +%s)
prom="$TEXTFILE_DIR/mem0.prom"
tmp="$prom.$$"
mkdir -p "$TEXTFILE_DIR" || { echo "cannot write $TEXTFILE_DIR"; exit 2; }

psql_q() { docker exec "$PG_CONTAINER" psql -U postgres -d "$1" -tAc "$2" 2>/dev/null | tr -d ' '; }
num() { case "$1" in ''|*[!0-9.-]*) echo 0 ;; *) echo "$1" ;; esac; }

# --- store shape -----------------------------------------------------------
memories=$(num "$(psql_q postgres "select count(*) from mem0_local")")
users=$(num "$(psql_q postgres "select count(distinct payload->>'user_id') from mem0_local")")
vec_bytes=$(num "$(psql_q postgres "select pg_total_relation_size('mem0_local')")")
db_vectors=$(num "$(psql_q postgres "select pg_database_size('postgres')")")
db_app=$(num "$(psql_q postgres "select pg_database_size('mem0_app')")")

# Age of the most recent memory. This is the one that would have caught the
# capture hook silently writing nowhere for six weeks: the store stays up, the
# probe stays green, and only the freshness of its newest row says otherwise.
newest_age=$(num "$(psql_q postgres \
  "select coalesce(round(extract(epoch from now() - max((payload->>'created_at')::timestamptz))), -1) from mem0_local")")

# --- audit trail (SQLite inside the API container) -------------------------
history_rows=$(docker exec "$API_CONTAINER" python -c "
import sqlite3
try:
    c = sqlite3.connect('/app/history/history.db')
    print(c.execute('select count(*) from history').fetchone()[0])
except Exception:
    print(0)
" 2>/dev/null | tr -d ' ')
history_rows=$(num "$history_rows")

# --- traffic + latency from request_logs -----------------------------------
# /memories/<uuid> collapses to /memories/:id so one deleted memory cannot mint
# a new time series forever.
norm="case when path ~ '^/memories/[0-9a-f-]{36}' then '/memories/:id' else path end"

reqs=$(docker exec "$PG_CONTAINER" psql -U postgres -d mem0_app -tAF, -c "
  select $norm as p, status_code, count(*)
    from request_logs
   where created_at > now() - interval '$WINDOW_MIN minutes'
   group by 1, 2" 2>/dev/null)

# Paths come from a 24h window but the figures from LAT_WINDOW, so a path that
# has gone quiet reports 0 samples instead of vanishing. A panel that empties
# out looks broken; a zero bar reads as "nothing happened", which is the truth.
# Quantiles are emitted only when the window actually holds requests.
lat=$(docker exec "$PG_CONTAINER" psql -U postgres -d mem0_app -tAF, -c "
  with w as (select created_at > now() - interval '$LAT_WINDOW_MIN minutes' as recent,
                    $norm as p, latency_ms
               from request_logs
              where created_at > now() - interval '24 hours')
  select p,
         coalesce(round(percentile_cont(0.5)  within group (order by latency_ms)
                        filter (where recent)::numeric, 1), -1),
         coalesce(round(percentile_cont(0.95) within group (order by latency_ms)
                        filter (where recent)::numeric, 1), -1),
         coalesce(round(max(latency_ms) filter (where recent)::numeric, 1), -1),
         count(*) filter (where recent)
    from w group by 1" 2>/dev/null)

# --- live embedder probe ---------------------------------------------------
# The embedder is the component that has actually misbehaved (thread
# oversubscription, then CPU pinning), and it fails by getting slow rather than
# by going down, which no up/down probe can see. Unique prompt each run so a
# prompt-cache hit cannot flatter the number.
embed_secs=$(docker exec "$API_CONTAINER" python -c "
import json, time, urllib.request
p = 'metrics probe $(date +%s)'
t = time.time()
try:
    r = urllib.request.Request('http://ollama:11434/api/embeddings',
        data=json.dumps({'model': '$EMBED_MODEL', 'prompt': p}).encode(),
        headers={'Content-Type': 'application/json'})
    d = json.load(urllib.request.urlopen(r, timeout=120))
    print(f'{time.time()-t:.3f}' if d.get('embedding') else '-1')
except Exception:
    print('-1')
" 2>/dev/null | tr -d ' ')
embed_secs=$(num "$embed_secs")

END=$(date +%s)

{
  echo "# HELP mem0_memories_total Rows in the mem0 vector store"
  echo "# TYPE mem0_memories_total gauge"
  echo "mem0_memories_total $memories"

  echo "# HELP mem0_users_total Distinct user_ids in the vector store"
  echo "# TYPE mem0_users_total gauge"
  echo "mem0_users_total $users"

  echo "# HELP mem0_history_rows_total Rows in the SQLite memory-change audit trail"
  echo "# TYPE mem0_history_rows_total gauge"
  echo "mem0_history_rows_total $history_rows"

  echo "# HELP mem0_vector_table_bytes On-disk size of the vector table including indexes"
  echo "# TYPE mem0_vector_table_bytes gauge"
  echo "mem0_vector_table_bytes $vec_bytes"

  echo "# HELP mem0_database_bytes On-disk size of each mem0 database"
  echo "# TYPE mem0_database_bytes gauge"
  echo "mem0_database_bytes{database=\"vectors\"} $db_vectors"
  echo "mem0_database_bytes{database=\"app\"} $db_app"

  echo "# HELP mem0_newest_memory_age_seconds Age of the most recent memory, -1 if empty"
  echo "# TYPE mem0_newest_memory_age_seconds gauge"
  echo "mem0_newest_memory_age_seconds $newest_age"

  echo "# HELP mem0_requests_recent Requests in the last ${WINDOW_MIN}m by path and status"
  echo "# TYPE mem0_requests_recent gauge"
  if [ -n "$reqs" ]; then
    printf '%s\n' "$reqs" | while IFS=, read -r p code n; do
      [ -n "$p" ] || continue
      echo "mem0_requests_recent{path=\"$p\",status=\"$code\"} $(num "$n")"
    done
  fi

  echo "# HELP mem0_request_latency_ms Request latency over the last ${LAT_WINDOW_MIN}m"
  echo "# TYPE mem0_request_latency_ms gauge"
  echo "# HELP mem0_request_samples Requests behind the latency figures"
  echo "# TYPE mem0_request_samples gauge"
  if [ -n "$lat" ]; then
    printf '%s\n' "$lat" | while IFS=, read -r p p50 p95 mx n; do
      [ -n "$p" ] || continue
      # Sample count always; quantiles only when there is something behind
      # them. -1 is the SQL sentinel for "no requests in the window".
      echo "mem0_request_samples{path=\"$p\"} $(num "$n")"
      [ "$(num "$p50")" = "-1" ] && continue
      echo "mem0_request_latency_ms{path=\"$p\",quantile=\"0.5\"} $(num "$p50")"
      echo "mem0_request_latency_ms{path=\"$p\",quantile=\"0.95\"} $(num "$p95")"
      echo "mem0_request_latency_ms{path=\"$p\",quantile=\"max\"} $(num "$mx")"
    done
  fi

  echo "# HELP mem0_embed_probe_seconds Time to embed a unique short prompt, -1 on failure"
  echo "# TYPE mem0_embed_probe_seconds gauge"
  echo "mem0_embed_probe_seconds $embed_secs"

  echo "# HELP mem0_metrics_duration_seconds Wall-clock duration of this collection"
  echo "# TYPE mem0_metrics_duration_seconds gauge"
  echo "mem0_metrics_duration_seconds $((END-START))"

  echo "# HELP mem0_metrics_last_success_timestamp_seconds Unix time of this collection"
  echo "# TYPE mem0_metrics_last_success_timestamp_seconds gauge"
  echo "mem0_metrics_last_success_timestamp_seconds $END"
} > "$tmp" && mv -f "$tmp" "$prom"

echo "[mem0-metrics] memories=$memories history=$history_rows embed=${embed_secs}s in $((END-START))s"
