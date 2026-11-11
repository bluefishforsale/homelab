#!/usr/bin/env bash
# plex-transcode-metrics.sh - publish per-device transcode behaviour to the
# node_exporter textfile collector, from Tautulli history.
#
# Why this exists: a Plex client can be perfectly "up" while transcoding every
# single thing it plays. One Roku transcoded 164 of 164 sessions over five
# months and nothing alerted, because nothing in Prometheus could express it:
#
#   plex_video_transcode_sessions_count   labelled by username and state only,
#                                         no player, platform or product
#   tautulli_stream_count_transcode       one fleet-wide gauge, no breakdown
#
# Both are instantaneous session gauges, so even per-device labels would not
# give a ratio over time. The ratio lives only in Tautulli history, which
# nothing scraped. This fills that gap and nothing else: PlexDown and
# PlexPathDegraded already cover availability.
#
# READ-ONLY: one Tautulli get_history call. It never changes a session, a
# setting, or the library.
#
# The window is rolling on purpose. An all-time ratio would keep alerting
# forever on a device whose owner has since fixed their settings, so a fixed
# client has to be able to age out of the alert.
#
# Only transcode_decision is available here. A history row carries no
# video_decision, audio_decision, quality_profile or bitrate (all null), so
# splitting "transcodes video" from "transcodes only audio" would need one
# extra HTTP call per session, roughly 500 per 30 days. That distinction is
# what separates a misconfigured client from an incapable one, so it stays a
# hand-run question for `scripts/plex-playback.py why`.
#
# Usage: plex-transcode-metrics.sh        (writes $TEXTFILE_DIR/plex_transcode.prom)
set -uo pipefail

: "${TEXTFILE_DIR:=/data01/services/node-exporter/text_files}"
: "${TAUTULLI_URL:=http://localhost:8905}"
: "${TAUTULLI_CFG:=/data01/services/tautulli/config/config.ini}"
: "${WINDOW_DAYS:=30}"
# Ceiling on rows pulled in one call. Roughly 500 rows per 30 days today, so
# this is ~10x headroom; plex_transcode_history_truncated goes to 1 if the
# window ever outgrows it, rather than silently reporting a partial ratio.
: "${MAX_ROWS:=5000}"

prom="$TEXTFILE_DIR/plex_transcode.prom"
tmp="$prom.$$"

for dep in jq curl date; do
  command -v "$dep" >/dev/null 2>&1 || { echo "[plex-transcode-metrics] missing $dep"; exit 2; }
done
mkdir -p "$TEXTFILE_DIR" || { echo "[plex-transcode-metrics] cannot write $TEXTFILE_DIR"; exit 2; }

key=$(grep -oP 'api_key\s*=\s*\K\w+' "$TAUTULLI_CFG" 2>/dev/null | head -1)
[ -n "$key" ] || { echo "[plex-transcode-metrics] no api_key in $TAUTULLI_CFG"; exit 2; }

after=$(date -u -d "-${WINDOW_DAYS} days" +%Y-%m-%d) || exit 2

# A failed call leaves the previous .prom untouched rather than publishing an
# empty one: node_textfile_mtime_seconds already makes the staleness visible,
# and zeroed ratios would read as "every client behaves fine".
resp=$(curl -fsS -m 45 -G "$TAUTULLI_URL/api/v2" \
         --data-urlencode "apikey=$key" \
         --data-urlencode "cmd=get_history" \
         --data-urlencode "after=$after" \
         --data-urlencode "length=$MAX_ROWS" 2>/dev/null) || {
  echo "[plex-transcode-metrics] Tautulli call failed, keeping previous $prom"; exit 1; }

echo "$resp" | jq -e '.response.data.data' >/dev/null 2>&1 || {
  echo "[plex-transcode-metrics] unexpected response shape, keeping previous $prom"; exit 1; }

filtered=$(echo "$resp" | jq -r '.response.data.recordsFiltered // 0')
considered=$(echo "$resp" | jq -r '.response.data.data | length')
truncated=0
[ "$filtered" -gt "$MAX_ROWS" ] 2>/dev/null && truncated=1

# Label values are escaped for the Prometheus text format: backslash first, then
# quote, then newline. Not theoretical, real player names here include
# `55" Sharp Roku TV` and `65" TCL Roku TV`, whose literal double quote would
# otherwise produce a .prom that node_exporter refuses to parse, taking every
# other metric in this file down with it.
series=$(echo "$resp" | jq -r '
  def esc: tostring
    | gsub("\\\\"; "\\\\\\\\")
    | gsub("\""; "\\\"")
    | gsub("\n"; "\\n");
  .response.data.data
  | map(select((.player // "") != ""))
  | group_by([(.player // "unknown"), (.platform // "unknown"),
              (.product // "unknown"), (.friendly_name // .user // "unknown")])
  | sort_by(-length)
  | map(
      .[0] as $r
      | ($r.player // "unknown" | esc) as $player
      | ($r.platform // "unknown" | esc) as $platform
      | ($r.product // "unknown" | esc) as $product
      | ($r.friendly_name // $r.user // "unknown" | esc) as $user
      | length as $sessions
      | (map(select(.transcode_decision == "transcode")) | length) as $transcodes
      | "player=\"\($player)\",platform=\"\($platform)\",product=\"\($product)\",user=\"\($user)\"" as $labels
      | "plex_device_sessions{\($labels)} \($sessions)",
        "plex_device_transcodes{\($labels)} \($transcodes)"
    )
  | join("\n")
') || { echo "[plex-transcode-metrics] jq aggregation failed, keeping previous $prom"; exit 1; }

{
  echo "# HELP plex_device_sessions Playback sessions per client in the rolling window"
  echo "# TYPE plex_device_sessions gauge"
  echo "# HELP plex_device_transcodes Sessions that transcoded, same labels. Ratio against plex_device_sessions is the signal."
  echo "# TYPE plex_device_transcodes gauge"
  [ -n "$series" ] && echo "$series"

  echo "# HELP plex_transcode_window_seconds Length of history the ratios cover"
  echo "# TYPE plex_transcode_window_seconds gauge"
  echo "plex_transcode_window_seconds $((WINDOW_DAYS * 86400))"

  echo "# HELP plex_transcode_sessions_considered Rows aggregated on this run"
  echo "# TYPE plex_transcode_sessions_considered gauge"
  echo "plex_transcode_sessions_considered $considered"

  # 1 means the window holds more sessions than one call returns, so every
  # ratio above is computed from a partial window and reads low.
  echo "# HELP plex_transcode_history_truncated 1 if the window exceeded MAX_ROWS, making the ratios partial"
  echo "# TYPE plex_transcode_history_truncated gauge"
  echo "plex_transcode_history_truncated $truncated"

  echo "# HELP plex_transcode_metrics_last_success_timestamp_seconds Unix time of this collection"
  echo "# TYPE plex_transcode_metrics_last_success_timestamp_seconds gauge"
  echo "plex_transcode_metrics_last_success_timestamp_seconds $(date +%s)"
} > "$tmp" && mv -f "$tmp" "$prom"

echo "[plex-transcode-metrics] window=${WINDOW_DAYS}d considered=$considered filtered=$filtered truncated=$truncated"
