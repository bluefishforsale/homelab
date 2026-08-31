#!/usr/bin/env bash
# kometa-metrics.sh - publish Kometa run outcomes to the node_exporter textfile
# collector.
#
# Kometa has no metrics endpoint, its stdout is silent, and its container
# healthcheck only tests that a config file exists, so a job that has not
# succeeded since 2025-11-12 reported "healthy" the entire time. The only
# evidence a run happened at all is meta.log, which rotates one file per run:
# meta.log is the newest, meta.log.1 the one before it.
#
# Success is defined as a run that reached its "Finished ... Run" summary with
# no CRITICAL lines. A run that aborts on a config or connection error still
# prints a summary, so "it finished" alone is not enough.
set -uo pipefail

: "${TEXTFILE_DIR:=/data01/services/node-exporter/text_files}"
: "${LOG_DIR:=/data01/services/plex-meta-manager/config/logs}"

prom="$TEXTFILE_DIR/kometa.prom"
tmp="$prom.$$"
mkdir -p "$TEXTFILE_DIR" || { echo "cannot write $TEXTFILE_DIR"; exit 2; }

# "Finished: 05:00:42 2026-08-30" -> epoch, in the host's local zone, which is
# what Kometa writes.
finished_epoch() {
  local f="$1" line
  line=$(grep -ho 'Finished: [0-9:]\+ [0-9-]\+' "$f" 2>/dev/null | tail -1) || return 1
  [ -n "$line" ] || return 1
  local t d
  t=$(echo "$line" | awk '{print $2}')
  d=$(echo "$line" | awk '{print $3}')
  date -d "$d $t" +%s 2>/dev/null
}

run_time_seconds() {
  local f="$1" rt
  rt=$(grep -ho 'Run Time: [0-9]\+:[0-9]\+:[0-9]\+' "$f" 2>/dev/null | tail -1 | awk '{print $3}')
  [ -n "$rt" ] || { echo 0; return; }
  echo "$rt" | awk -F: '{print ($1*3600)+($2*60)+$3}'
}

criticals() { grep -c '\[CRITICAL\]' "$1" 2>/dev/null || echo 0; }

last_run_ts=0
last_run_failed=1
last_run_duration=0
last_run_criticals=0
last_success_ts=0
version="unknown"

# Newest first: meta.log, then meta.log.1, .2, ... as they age.
mapfile -t logs < <(ls -1 "$LOG_DIR"/meta.log "$LOG_DIR"/meta.log.[0-9]* 2>/dev/null)

for f in "${logs[@]}"; do
  [ -f "$f" ] || continue
  ts=$(finished_epoch "$f") || continue
  [ -n "$ts" ] || continue
  c=$(criticals "$f")

  if [ "$last_run_ts" -eq 0 ]; then
    last_run_ts=$ts
    last_run_criticals=$c
    last_run_duration=$(run_time_seconds "$f")
    [ "$c" -eq 0 ] && last_run_failed=0 || last_run_failed=1
    v=$(grep -ho 'Version: [0-9.]\+' "$f" 2>/dev/null | head -1 | awk '{print $2}')
    [ -n "$v" ] && version="$v"
  fi

  if [ "$c" -eq 0 ] && [ "$last_success_ts" -eq 0 ]; then
    last_success_ts=$ts
  fi
done

{
  echo "# HELP kometa_last_run_timestamp_seconds Unix time the most recent run finished, 0 if never"
  echo "# TYPE kometa_last_run_timestamp_seconds gauge"
  echo "kometa_last_run_timestamp_seconds $last_run_ts"

  echo "# HELP kometa_last_success_timestamp_seconds Unix time of the most recent run with no CRITICAL lines"
  echo "# TYPE kometa_last_success_timestamp_seconds gauge"
  echo "kometa_last_success_timestamp_seconds $last_success_ts"

  echo "# HELP kometa_last_run_failed 1 if the most recent run logged a CRITICAL"
  echo "# TYPE kometa_last_run_failed gauge"
  echo "kometa_last_run_failed $last_run_failed"

  echo "# HELP kometa_last_run_criticals CRITICAL lines in the most recent run"
  echo "# TYPE kometa_last_run_criticals gauge"
  echo "kometa_last_run_criticals $last_run_criticals"

  # A run that "succeeds" in zero seconds did nothing. The 8-month outage
  # looked exactly like that: Run Time 0:00:00, every night.
  echo "# HELP kometa_last_run_duration_seconds Wall-clock duration of the most recent run"
  echo "# TYPE kometa_last_run_duration_seconds gauge"
  echo "kometa_last_run_duration_seconds $last_run_duration"

  echo "# HELP kometa_runs_logged Run logs currently on disk"
  echo "# TYPE kometa_runs_logged gauge"
  echo "kometa_runs_logged ${#logs[@]}"

  echo "# HELP kometa_build_info Kometa version from the most recent run"
  echo "# TYPE kometa_build_info gauge"
  echo "kometa_build_info{version=\"$version\"} 1"

  echo "# HELP kometa_metrics_last_success_timestamp_seconds Unix time of this collection"
  echo "# TYPE kometa_metrics_last_success_timestamp_seconds gauge"
  echo "kometa_metrics_last_success_timestamp_seconds $(date +%s)"
} > "$tmp" && mv -f "$tmp" "$prom"

echo "[kometa-metrics] last_run=$last_run_ts failed=$last_run_failed criticals=$last_run_criticals duration=${last_run_duration}s version=$version"
