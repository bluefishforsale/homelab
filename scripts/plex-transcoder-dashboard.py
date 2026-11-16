#!/usr/bin/env python3
"""Generate the Plex Transcoder Grafana dashboard JSON (stdout).

The JSON in files/grafana-compose/dashboards/plex-transcoder.json is this
script's output; change the panels here and regenerate, rather than editing
1900 lines of JSON by hand:

    python3 scripts/plex-transcoder-dashboard.py \\
        > files/grafana-compose/dashboards/plex-transcoder.json

Metrics come from files/ocean-plex/plex-client-metrics.py.
"""
import json

DS = {"type": "prometheus", "uid": "bf02tpxiz6m0wf"}
U = 'user=~"$user"'
CLIENT = "user, player, platform, product"
_id = 0
panels = []
y = 0


def nid():
    global _id
    _id += 1
    return _id


def tgt(expr, ref="A", legend=None, instant=False, table=False):
    t = {"datasource": DS, "editorMode": "code", "expr": expr, "refId": ref}
    if legend:
        t["legendFormat"] = legend
    if instant:
        t["instant"] = True
        t["range"] = False
    if table:
        t["format"] = "table"
    return t


def row(title):
    global y
    panels.append({"type": "row", "title": title, "collapsed": False, "id": nid(),
                   "gridPos": {"h": 1, "w": 24, "x": 0, "y": y}, "panels": []})
    y += 1


def panel(kind, title, x, w, h, targets, desc="", defaults=None, options=None, overrides=None,
          transformations=None):
    p = {"type": kind, "title": title, "id": nid(), "datasource": DS,
         "gridPos": {"h": h, "w": w, "x": x, "y": y}, "targets": targets,
         "fieldConfig": {"defaults": defaults or {}, "overrides": overrides or []},
         "options": options or {}}
    if desc:
        p["description"] = desc
    if transformations:
        p["transformations"] = transformations
    panels.append(p)
    return p


def stat(title, x, w, expr, desc, unit="short", steps=None, mappings=None, decimals=None):
    d = {"unit": unit, "color": {"mode": "thresholds"},
         "thresholds": {"mode": "absolute", "steps": steps or [{"color": "text", "value": None}]}}
    if mappings:
        d["mappings"] = mappings
    if decimals is not None:
        d["decimals"] = decimals
    panel("stat", title, x, w, 4, [tgt(expr, instant=True)], desc, d,
          {"reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
           "colorMode": "value", "graphMode": "none", "textMode": "auto", "justifyMode": "auto"})


def ts(title, x, w, h, targets, desc, unit="short", extra=None, overrides=None, stack=False):
    d = {"unit": unit, "color": {"mode": "palette-classic"},
         "custom": {"drawStyle": "line", "lineWidth": 2, "fillOpacity": 0, "showPoints": "never",
                    "spanNulls": False, "axisSoftMin": 0,
                    "stacking": {"mode": "normal" if stack else "none", "group": "A"}}}
    if extra:
        d.update(extra)
    panel("timeseries", title, x, w, h, targets, desc, d,
          {"legend": {"displayMode": "list", "placement": "bottom", "showLegend": True},
           "tooltip": {"mode": "multi", "sort": "desc"}}, overrides)


RED = "red"
OK = [{"color": "text", "value": None}]
# Status colors carry a word too (mappings), never color alone.
UPDOWN = [{"type": "value", "options": {"0": {"text": "FAILING", "color": RED},
                                        "1": {"text": "OK", "color": "green"}}}]

# ------------------------------------------------------------------ now
row("Now: the transcoder, live (Tautulli get_activity, 1-minute timer)")
stat("Sessions", 0, 4, f"count(plex_client_live_session_info{{{U}}}) or vector(0)",
     "Active Plex sessions.")
stat("Video transcodes", 4, 4,
     f'count(plex_client_live_session_info{{{U}, video_decision="transcode"}}) or vector(0)',
     "Active sessions whose video is being transcoded.")
stat("On CPU (fell back)", 8, 4,
     f'count(plex_client_live_session_info{{{U}, video_decision="transcode", hw_requested="true", hw_encode="false"}}) or vector(0)',
     "Video transcodes where Plex asked for hardware and is encoding in software. Above 0 for 10m raises PlexTranscodeCpuFallback.",
     steps=[{"color": "green", "value": None}, {"color": RED, "value": 1}],
     mappings=[{"type": "value", "options": {"0": {"text": "none"}}}])
stat("Least buffer ahead", 12, 4,
     f"min(plex_client_live_transcode_buffer_seconds{{{U}}})",
     "Smallest number of seconds any transcoder is ahead of its viewer. Under 15s at under 1x for 5m raises PlexTranscodeStarved. Empty when nothing is transcoding.",
     unit="s", steps=[{"color": RED, "value": None}, {"color": "orange", "value": 15}, {"color": "green", "value": 60}])
stat("Slowest transcode", 16, 4, f"min(plex_client_live_transcode_speed{{{U}}})",
     "Lowest transcoder speed across sessions (x real time). Not colored: below 1 is normal once the transcoder is far ahead (it throttles on purpose), and only matters when the buffer is also small.",
     unit="none", decimals=1)
stat("NVENC", 20, 4, 'max(DCGM_FI_DEV_ENC_UTIL{instance="ocean.home"})',
     "GPU encoder utilisation on ocean's 3090. Shared with Tdarr; near 100% is when Plex transcodes risk falling back to CPU.",
     unit="percent")
y += 4

live_cols = ["user", "player", "state", "media_type", "video_decision", "audio_decision",
             "subtitle_decision", "quality_profile", "source_resolution", "stream_resolution",
             "source_video_codec", "stream_video_codec", "hw_requested", "hw_encode", "location",
             "title", "Value #B", "Value #C", "Value #D"]
panel("table", "Live sessions", 0, 24, 7,
      [tgt(f"plex_client_live_session_info{{{U}}}", "A", instant=True, table=True),
       tgt(f"plex_client_live_transcode_speed{{{U}}}", "B", instant=True, table=True),
       tgt(f"plex_client_live_transcode_buffer_seconds{{{U}}}", "C", instant=True, table=True),
       tgt(f"plex_client_live_bandwidth_kbps{{{U}}}", "D", instant=True, table=True)],
      "One row per active session: what was asked for, what the server is doing about it, and how far ahead the transcoder is.",
      {"custom": {"align": "auto", "cellOptions": {"type": "auto"}}},
      {"showHeader": True, "cellHeight": "sm"},
      [{"matcher": {"id": "byName", "options": "Value #B"},
        "properties": [{"id": "displayName", "value": "speed"}, {"id": "unit", "value": "none"},
                       {"id": "decimals", "value": 1}]},
       {"matcher": {"id": "byName", "options": "Value #C"},
        "properties": [{"id": "displayName", "value": "buffer ahead"}, {"id": "unit", "value": "s"},
                       {"id": "decimals", "value": 0}]},
       {"matcher": {"id": "byName", "options": "Value #D"},
        "properties": [{"id": "displayName", "value": "bandwidth"}, {"id": "unit", "value": "Kbits"}]}],
      [{"id": "merge", "options": {}},
       {"id": "organize", "options": {
           "excludeByName": {"Time": True, "Value #A": True, "__name__": True, "instance": True, "job": True,
                             "session": True, "platform": True, "product": True, "container_decision": True,
                             "source_dynamic_range": True, "stream_dynamic_range": True,
                             "stream_audio_codec": True, "hw_decode": True, "hw_full_pipeline": True,
                             "relayed": True, "secure": True, "transcode_decision": True},
           "indexByName": {c: i for i, c in enumerate(live_cols)}}}])
y += 7

ts("Transcode speed", 0, 6, 8,
   [tgt(f"plex_client_live_transcode_speed{{{U}}}", legend="{{user}} · {{player}}")],
   "x real time per transcoding session. The dashed line is 1x.",
   unit="none",
   extra={"thresholds": {"mode": "absolute", "steps": [{"color": "transparent", "value": None},
                                                         {"color": "orange", "value": 1}]},
          "custom": {"drawStyle": "line", "lineWidth": 2, "fillOpacity": 0, "showPoints": "never",
                     "axisSoftMin": 0, "thresholdsStyle": {"mode": "dashed"},
                     "stacking": {"mode": "none", "group": "A"}}})
ts("Transcoder buffer ahead of viewer", 6, 6, 8,
   [tgt(f"plex_client_live_transcode_buffer_seconds{{{U}}}", legend="{{user}} · {{player}}")],
   "Seconds transcoded beyond where the viewer is. Falling toward 0 means a stall is coming; the dashed line is the 15s alert threshold.",
   unit="s",
   extra={"thresholds": {"mode": "absolute", "steps": [{"color": "transparent", "value": None},
                                                         {"color": "orange", "value": 15}]},
          "custom": {"drawStyle": "line", "lineWidth": 2, "fillOpacity": 0, "showPoints": "never",
                     "axisSoftMin": 0, "thresholdsStyle": {"mode": "dashed"},
                     "stacking": {"mode": "none", "group": "A"}}})
ts("Sessions by what the server does", 12, 6, 8,
   [tgt(f'count(plex_client_live_session_info{{{U}, video_decision="transcode", hw_encode="true"}}) or vector(0)', "A", "video transcode, GPU"),
    tgt(f'count(plex_client_live_session_info{{{U}, video_decision="transcode", hw_encode="false"}}) or vector(0)', "B", "video transcode, CPU"),
    tgt(f'count(plex_client_live_session_info{{{U}, video_decision!="transcode", transcode_decision!="direct play"}}) or vector(0)', "C", "direct stream (audio or container changed)"),
    tgt(f'count(plex_client_live_session_info{{{U}, transcode_decision="direct play"}}) or vector(0)', "D", "direct play")],
   "Concurrent sessions, stacked by how the server delivers them. 'video transcode, CPU' is every software encode, whether or not hardware was requested; the 'On CPU (fell back)' stat counts only those where it was.",
   stack=True,
   extra={"custom": {"drawStyle": "line", "lineWidth": 2, "fillOpacity": 30, "showPoints": "never",
                     "axisSoftMin": 0, "stacking": {"mode": "normal", "group": "A"}}})
GPU = 'instance="ocean.home"'
ts("GPU: encoder, decoder, memory", 18, 6, 8,
   [tgt(f"max(DCGM_FI_DEV_ENC_UTIL{{{GPU}}})", "A", "NVENC"),
    tgt(f"max(DCGM_FI_DEV_DEC_UTIL{{{GPU}}})", "B", "NVDEC"),
    tgt(f"max(DCGM_FI_DEV_FB_USED{{{GPU}}} / (DCGM_FI_DEV_FB_USED{{{GPU}}} + DCGM_FI_DEV_FB_FREE{{{GPU}}}) * 100)", "C", "VRAM used")],
   "ocean's 3090, shared by Plex, Tdarr and llama.cpp. NVENC pinned near 100% or VRAM near full is when a Plex transcode can fall back to CPU. One axis: percent.",
   unit="percent", extra={"max": 100})
y += 8

# ------------------------------------------------------------------ clients
row("Clients: rolling 30 days (Tautulli history, refreshed every 10 minutes)")
sess = f"sum by ({CLIENT}) (plex_client_sessions{{{U}}})"
vt = f'sum by ({CLIENT}) (plex_client_sessions{{{U}, video_decision="transcode"}})'
panel("table", "Clients by video transcode share", 0, 14, 11,
      [tgt(sess, "A", instant=True, table=True),
       tgt(f"({vt} or 0 * {sess})", "B", instant=True, table=True),
       tgt(f"({vt} or 0 * {sess}) / {sess}", "C", instant=True, table=True),
       tgt(f"sum by ({CLIENT}) (plex_client_video_transcode_seconds{{{U}}})", "D", instant=True, table=True),
       tgt(f"max by ({CLIENT}) (plex_client_last_played_timestamp_seconds{{{U}}}) * 1000", "E", instant=True, table=True)],
      "Every client seen in the window. A share near 100% is a client that cannot direct play, usually a quality setting or subtitle choice on that client; the cause table to the right says which.",
      {"custom": {"align": "auto", "cellOptions": {"type": "auto"}}},
      {"showHeader": True, "cellHeight": "sm", "sortBy": [{"displayName": "video transcoded", "desc": True}]},
      [{"matcher": {"id": "byName", "options": "Value #A"},
        "properties": [{"id": "displayName", "value": "sessions"}]},
       {"matcher": {"id": "byName", "options": "Value #B"},
        "properties": [{"id": "displayName", "value": "video transcodes"}]},
       {"matcher": {"id": "byName", "options": "Value #C"},
        "properties": [{"id": "displayName", "value": "video transcoded"}, {"id": "unit", "value": "percentunit"},
                       {"id": "min", "value": 0}, {"id": "max", "value": 1},
                       {"id": "color", "value": {"mode": "continuous-blues"}},
                       {"id": "custom.cellOptions", "value": {"type": "gauge", "mode": "basic", "valueDisplayMode": "text"}}]},
       {"matcher": {"id": "byName", "options": "Value #D"},
        "properties": [{"id": "displayName", "value": "time transcoding"}, {"id": "unit", "value": "s"}]},
       {"matcher": {"id": "byName", "options": "Value #E"},
        "properties": [{"id": "displayName", "value": "last played"}, {"id": "unit", "value": "dateTimeFromNow"}]}],
      [{"id": "merge", "options": {}},
       {"id": "organize", "options": {"excludeByName": {"Time": True},
                                      "indexByName": {"user": 0, "player": 1, "platform": 2, "product": 3}}}])

panel("bargauge", "Why video transcodes", 14, 10, 5,
      [tgt(f"sum by (cause) (plex_client_video_transcode_cause_sessions{{{U}}})", legend="{{cause}}", instant=True)],
      "Video transcodes in the window by the first reason that forces one: subtitle_burn (image subtitles burned in), quality_cap (the session's quality was capped below Original, by the client's setting or Plex's remote auto quality; every such transcode delivered below the source bitrate), codec (client cannot decode the source codec), resolution (downscale), other (none of these: a remote bitrate limit or a container the client cannot take).",
      {"unit": "short", "min": 0, "color": {"mode": "fixed", "fixedColor": "blue"}},
      {"orientation": "horizontal", "displayMode": "basic", "showUnfilled": True, "valueMode": "text",
       "namePlacement": "left", "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False}},
      transformations=[{"id": "sortBy", "options": {"sort": [{"field": "Value", "desc": True}]}}])
y_cause = y
panel("table", "Transcode causes by client", 14, 10, 6,
      [tgt(f"sum by (user, player, cause, quality_profile, subtitle_decision, source_video_codec, stream_video_codec, source_resolution, stream_resolution) (plex_client_video_transcode_cause_sessions{{{U}}})",
           "A", instant=True, table=True)],
      "The facts behind each cause: which quality profile, subtitle decision, codec and resolution change.",
      {"custom": {"align": "auto", "cellOptions": {"type": "auto"}}},
      {"showHeader": True, "cellHeight": "sm", "sortBy": [{"displayName": "transcodes", "desc": True}]},
      [{"matcher": {"id": "byName", "options": "Value"}, "properties": [{"id": "displayName", "value": "transcodes"}]}],
      [{"id": "organize", "options": {"excludeByName": {"Time": True},
                                      "indexByName": {"user": 0, "player": 1, "cause": 2, "Value": 3}}}])
panels[-1]["gridPos"]["y"] = y_cause + 5
y += 11

ts("Video transcode share, whole server, 30-day rolling", 0, 8, 7,
   [tgt(f'(sum(plex_client_sessions{{{U}, video_decision="transcode"}}) or 0 * sum(plex_client_sessions{{{U}}})) / sum(plex_client_sessions{{{U}}})', legend="30-day video transcode share")],
   "Share of sessions in the rolling window whose video was transcoded. Falls as clients are fixed and their old sessions age out.",
   unit="percentunit", extra={"min": 0, "max": 1})
panel("bargauge", "Quality profiles clients ask for", 8, 8, 7,
      [tgt(f"sum by (quality_profile) (plex_client_quality_profile_sessions{{{U}}})", legend="{{quality_profile}}", instant=True)],
      "Sessions by the quality the client requested. Anything other than Original forces a transcode for most files.",
      {"unit": "short", "min": 0, "color": {"mode": "fixed", "fixedColor": "blue"}},
      {"orientation": "horizontal", "displayMode": "basic", "showUnfilled": True, "valueMode": "text",
       "namePlacement": "left", "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False}},
      transformations=[{"id": "sortBy", "options": {"sort": [{"field": "Value", "desc": True}]}}])
panel("bargauge", "Video transcodes by encoder", 16, 8, 7,
      [tgt(f'sum(plex_client_video_transcode_hw_sessions{{{U}, hw_encode="true"}}) or vector(0)', "A", "GPU (NVENC)", instant=True),
       tgt(f'sum(plex_client_video_transcode_hw_sessions{{{U}, hw_encode="false"}}) or vector(0)', "B", "CPU", instant=True)],
      "Finished video transcodes in the window by whether the GPU encoded them.",
      {"unit": "short", "min": 0, "color": {"mode": "fixed", "fixedColor": "blue"}},
      {"orientation": "horizontal", "displayMode": "basic", "showUnfilled": True, "valueMode": "text",
       "namePlacement": "left", "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False}})
y += 7

# ------------------------------------------------------------------ health
row("Collector health (plex-client-metrics on ocean)")
stat("Live calls", 0, 4, "plex_client_live_ok", "Last get_activity call succeeded.", mappings=UPDOWN, steps=OK)
stat("History calls", 4, 4, "plex_client_history_ok", "Last get_history refresh succeeded.", mappings=UPDOWN, steps=OK)
stat("Since last run", 8, 4, "time() - plex_client_metrics_last_run_timestamp_seconds",
     "The timer runs every minute. Over 5 minutes for 10 raises PlexClientMetricsStale.",
     unit="s", steps=[{"color": "text", "value": None}, {"color": RED, "value": 300}])
stat("Sessions cached", 12, 4, "plex_client_history_sessions_cached", "Finished sessions in the window with stream data cached.")
stat("Stream data errors", 16, 4, "plex_client_history_stream_data_errors",
     "get_stream_data calls that failed at the last history refresh.",
     steps=[{"color": "text", "value": None}, {"color": "orange", "value": 1}])
stat("History truncated", 20, 4, "plex_client_history_truncated",
     "1 if the window holds more sessions than MAX_ROWS, so the oldest are missing.",
     mappings=[{"type": "value", "options": {"0": {"text": "no"}, "1": {"text": "YES", "color": "orange"}}}], steps=OK)
y += 4

# History and live both, so a user whose only sessions are live can be picked.
USERS = 'label_values({__name__=~"plex_client_sessions|plex_client_live_session_info"}, user)'

dash = {
    "annotations": {"list": []},
    "description": "Plex transcoder: what is transcoding now and whether it keeps up, and which clients transcode and why. From plex-client-metrics (files/ocean-plex/plex-client-metrics.py).",
    "editable": True, "graphTooltip": 1, "id": None, "links": [], "refresh": "1m",
    "schemaVersion": 39, "tags": ["plex", "media", "transcode"],
    "templating": {"list": [{
        "name": "user", "label": "User", "type": "query", "datasource": DS,
        "query": {"query": USERS, "refId": "user"},
        "definition": USERS,
        "refresh": 2, "includeAll": True, "multi": True, "allValue": ".*",
        "current": {"selected": True, "text": ["All"], "value": ["$__all"]}, "sort": 1}]},
    "time": {"from": "now-6h", "to": "now"}, "timezone": "",
    "title": "Plex Transcoder", "uid": "plex-transcoder", "version": 1,
    "panels": panels,
}
print(json.dumps(dash, indent=2))
