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

# A failed query must NOT become a number. The previous version ran every result
# through num(), which turns "" into 0, so an unreachable Postgres published
# mem0_memories_total 0 as a PRESENT series -- indistinguishable from a real
# empty store, and invisible to absent(). Worse,
# mem0_newest_memory_age_seconds (the metric that exists to catch a silently
# failing capture hook) read 0, meaning "newest memory arrived a moment ago":
# the freshness detector reported perfect health exactly when the store was
# gone. Now an unmeasurable value returns non-zero and the series is OMITTED,
# so absent() fires and the dashboard shows a gap rather than a comfortable
# zero. The -1 sentinels stay: those mean "measured, and genuinely empty".
qnum() {  # $1 = database, $2 = sql -> value on stdout, non-zero if unmeasurable
  local v
  v=$(psql_q "$1" "$2") || return 1
  case "$v" in ''|*[!0-9.-]*) return 1 ;; esac
  printf '%s' "$v"
}

# Emit a gauge only when the value was actually measured. An empty value means
# the probe failed, and publishing nothing is the honest answer.
gauge() {  # $1 = metric, $2 = help, $3 = value ('' to skip)
  [ -n "$3" ] || return 0
  printf '# HELP %s %s\n# TYPE %s gauge\n%s %s\n' "$1" "$2" "$1" "$1" "$3"
}

# DEGRADED must be assigned in THIS shell, not inside a command substitution,
# or the subshell swallows it -- the same class of bug being fixed here.
DEGRADED=0

# --- store shape -----------------------------------------------------------
memories=$(qnum postgres "select count(*) from mem0_local") || { memories=""; DEGRADED=1; }
users=$(qnum postgres "select count(distinct payload->>'user_id') from mem0_local") || { users=""; DEGRADED=1; }
vec_bytes=$(qnum postgres "select pg_total_relation_size('mem0_local')") || { vec_bytes=""; DEGRADED=1; }
db_vectors=$(qnum postgres "select pg_database_size('postgres')") || { db_vectors=""; DEGRADED=1; }
db_app=$(qnum postgres "select pg_database_size('mem0_app')") || { db_app=""; DEGRADED=1; }

# Age of the most recent memory. This is the one that would have caught the
# capture hook silently writing nowhere for six weeks: the store stays up, the
# probe stays green, and only the freshness of its newest row says otherwise.
newest_age=$(qnum postgres \
  "select coalesce(round(extract(epoch from now() - max((payload->>'created_at')::timestamptz))), -1) from mem0_local") \
  || { newest_age=""; DEGRADED=1; }

# --- audit trail (SQLite inside the API container) -------------------------
history_rows=$(docker exec "$API_CONTAINER" python -c "
import sqlite3
try:
    c = sqlite3.connect('/app/history/history.db')
    print(c.execute('select count(*) from history').fetchone()[0])
except Exception:
    print(0)
" 2>/dev/null | tr -d ' ')
case "$history_rows" in
  ''|*[!0-9.-]*) history_rows=""; DEGRADED=1 ;;
esac

# --- traffic + latency from request_logs -----------------------------------
# /memories/<uuid> collapses to /memories/:id so one deleted memory cannot mint
# a new time series forever.
norm="case when path ~ '^/memories/[0-9a-f-]{36}' then '/memories/:id' else path end"

# Path/status pairs come from a 24h window but the counts from WINDOW_MIN, so a
# pair that has gone quiet reports 0 instead of vanishing — the same trick the
# latency block below uses, and for the same reason: a panel that empties out
# looks broken, while a zero reads as "nothing happened", which is the truth.
# Without this the traffic panel is blank on any idle stretch over WINDOW_MIN.
reqs=$(docker exec "$PG_CONTAINER" psql -U postgres -d mem0_app -tAF, -c "
  with w as (select created_at > now() - interval '$WINDOW_MIN minutes' as recent,
                    $norm as p, status_code
               from request_logs
              where created_at > now() - interval '24 hours')
  select p, status_code, count(*) filter (where recent)
    from w group by 1, 2" 2>/dev/null)

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
# The probe prints -1 when it ran and failed, which is a measurement worth
# publishing. An empty value means it could not run at all (container gone) --
# num() used to turn that into 0, i.e. "embedded instantly", the most reassuring
# possible number for a dead service.
case "$embed_secs" in
  ''|*[!0-9.-]*) embed_secs=""; DEGRADED=1 ;;
esac

# --- live extraction-LLM probe ---------------------------------------------
# The embedder probe above was written when both halves ran locally. Extraction
# now leaves the network, and that dependency has already failed once: the
# OpenAI account ran out of credit and every add returned 502 for 21 hours.
# Nothing noticed, because embeddings are local, so search stayed fast, /docs
# stayed 200 and the container healthcheck stayed green the whole time.
#
# Mem0WritesFailing catches that, but only once writes are being attempted and
# rejected. If nobody is working there are no writes, no 5xx and no alert, so
# the outage is discovered by sitting down to work. This probe is independent
# of usage.
#
# It must be a real completion. /v1/models returned 200 throughout that outage:
# the key was valid, the quota was not. Only generating a token surfaces
# insufficient_quota.
#
# Config is read from the mem0 container's own environment, so the probe tests
# exactly what mem0 uses, with mem0's credentials, from mem0's network
# position, and no secret has to be handled here.
#
# Rate-limited because this one costs money. Measured against the live
# provider: 8 prompt + 1 completion tokens, 1.4s. At 1/min that is $0.08/month
# against an extraction bill of ~$0.80/month, a tenth of the thing it watches.
# Every 5 minutes is $0.016/month and still finds an outage long before the
# 15-minute alert window closes.
: "${LLM_PROBE_INTERVAL_SEC:=300}"
llm_stamp="${TEXTFILE_DIR}/.mem0-llm-probe-stamp"
llm_cache="${TEXTFILE_DIR}/.mem0-llm-probe-value"
now=$(date +%s)
last=$(cat "$llm_stamp" 2>/dev/null || echo 0)
case "$last" in ''|*[!0-9]*) last=0 ;; esac

if [ $((now - last)) -ge "$LLM_PROBE_INTERVAL_SEC" ]; then
  llm_secs=$(docker exec "$API_CONTAINER" python -c "
import json, os, time, urllib.request
base = (os.environ.get('MEM0_LLM_BASE_URL') or os.environ.get('OPENAI_BASE_URL') or '').rstrip('/')
key = os.environ.get('MEM0_LLM_API_KEY') or os.environ.get('OPENAI_API_KEY') or ''
model = os.environ.get('MEM0_DEFAULT_LLM_MODEL') or ''
if not base or not model:
    print('-1'); raise SystemExit
body = json.dumps({'model': model,
                   'messages': [{'role': 'user', 'content': 'ping'}],
                   'max_tokens': 1, 'temperature': 0}).encode()
t = time.time()
try:
    r = urllib.request.Request(base + '/chat/completions', data=body,
        headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ' + key})
    d = json.load(urllib.request.urlopen(r, timeout=60))
    print(f'{time.time()-t:.3f}' if d.get('choices') else '-1')
except Exception:
    print('-1')
" 2>/dev/null | tr -d ' ')
  case "$llm_secs" in
    ''|*[!0-9.-]*) llm_secs="" ;;
    *) printf '%s' "$now" > "$llm_stamp"; printf '%s' "$llm_secs" > "$llm_cache" ;;
  esac
else
  # Republish the last real measurement between probes. Omitting the series on
  # in-between runs would make it flap in and out of existence and break
  # absent() for anything alerting on it.
  llm_secs=$(cat "$llm_cache" 2>/dev/null)
  case "$llm_secs" in ''|*[!0-9.-]*) llm_secs="" ;; esac
fi
# Unlike the embedder, an unmeasurable LLM probe does NOT set DEGRADED: the
# provider being unreachable is a real finding about the provider, not a broken
# collector, and it must not suppress mem0_metrics_last_success_timestamp.

END=$(date +%s)

{
  # One number to alert on: 0 means at least one probe could not measure, so
  # the gaps below are failure rather than genuine absence.
  echo "# HELP mem0_collector_up 1 when every probe in this run returned a real value"
  echo "# TYPE mem0_collector_up gauge"
  echo "mem0_collector_up $((1 - DEGRADED))"

  gauge mem0_memories_total "Rows in the mem0 vector store" "$memories"
  gauge mem0_users_total "Distinct user_ids in the vector store" "$users"
  gauge mem0_history_rows_total "Rows in the SQLite memory-change audit trail" "$history_rows"
  gauge mem0_vector_table_bytes "On-disk size of the vector table including indexes" "$vec_bytes"

  if [ -n "$db_vectors" ] || [ -n "$db_app" ]; then
    echo "# HELP mem0_database_bytes On-disk size of each mem0 database"
    echo "# TYPE mem0_database_bytes gauge"
    [ -n "$db_vectors" ] && echo "mem0_database_bytes{database=\"vectors\"} $db_vectors"
    [ -n "$db_app" ] && echo "mem0_database_bytes{database=\"app\"} $db_app"
  fi

  # Absent when unmeasurable, -1 when the store is genuinely empty. Never 0,
  # which used to read as "a memory arrived a moment ago" while Postgres was
  # unreachable.
  gauge mem0_newest_memory_age_seconds \
    "Age of the most recent memory, -1 if empty, absent if unmeasurable" "$newest_age"

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

  gauge mem0_embed_probe_seconds \
    "Time to embed a unique short prompt, -1 if the probe ran and failed, absent if it could not run" \
    "$embed_secs"

  gauge mem0_llm_probe_seconds \
    "Time for a 1-token completion from the configured extraction provider, -1 if the probe ran and failed, absent if it could not run" \
    "$llm_secs"

  echo "# HELP mem0_metrics_duration_seconds Wall-clock duration of this collection"
  echo "# TYPE mem0_metrics_duration_seconds gauge"
  echo "mem0_metrics_duration_seconds $((END-START))"

  # Only on a run where everything was measurable. It is named last_SUCCESS,
  # and stamping it after a failed collection is how a freshness alert gets
  # told the data is current when nothing was collected at all.
  if [ "$DEGRADED" -eq 0 ]; then
    echo "# HELP mem0_metrics_last_success_timestamp_seconds Unix time of the last fully successful collection"
    echo "# TYPE mem0_metrics_last_success_timestamp_seconds gauge"
    echo "mem0_metrics_last_success_timestamp_seconds $END"
  fi
} > "$tmp" && mv -f "$tmp" "$prom"

echo "[mem0-metrics] memories=$memories history=$history_rows embed=${embed_secs}s llm=${llm_secs:-skip}s in $((END-START))s"
