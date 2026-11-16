#!/usr/bin/env python3
"""plex-client-metrics.py - per-client Plex playback metrics for the
node_exporter textfile collector, from Tautulli.

Why this exists: a Plex client can be perfectly "up" while transcoding every
single thing it plays. One Roku transcoded 164 of 164 sessions over five
months and nothing alerted, because nothing in Prometheus could express it.
plex-exporter and tautulli-exporter only publish instantaneous, server-wide
gauges with no player, platform, resolution or bandwidth breakdown. Tautulli
holds all of that, per session; this publishes it.

Two halves, one file (plex_client.prom):

  live     get_activity, every run. One series per active session with what
           the client is getting right now: bandwidth, delivered and source
           bitrate, transcode speed, throttling, and an info series carrying
           every decision and format label.

  history  get_history over a rolling window, plus get_stream_data for each
           session, which is the only place video/audio/subtitle decisions,
           resolutions, codecs, bitrates and the quality profile live (history
           rows carry only transcode_decision). Stream data for a finished
           session never changes, so it is cached by row_id and each refresh
           fetches only sessions it has not seen.

READ-ONLY against Tautulli: get_activity, get_history, get_stream_data. It
never changes a session, a client setting, or the library.

Cardinality is per user on purpose: the user count here is small, and "which
person is having a bad time" is the question this answers.

The window is rolling so a client whose owner fixes their settings ages back
out of the numbers instead of being judged on its whole history forever.

Usage: plex-client-metrics.py   (env: TEXTFILE_DIR TAUTULLI_URL TAUTULLI_CFG
                                  WINDOW_DAYS CACHE_FILE HISTORY_REFRESH_SECONDS)
"""

import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request

TEXTFILE_DIR = os.environ.get("TEXTFILE_DIR", "/data01/services/node-exporter/text_files")
TAUTULLI_URL = os.environ.get("TAUTULLI_URL", "http://localhost:8905").rstrip("/")
TAUTULLI_CFG = os.environ.get("TAUTULLI_CFG", "/data01/services/tautulli/config/config.ini")
WINDOW_DAYS = int(os.environ.get("WINDOW_DAYS", "30"))
CACHE_FILE = os.environ.get("CACHE_FILE", "/data01/services/plex/plex-client-metrics-cache.json")
# History moves slowly and costs one get_history call plus one get_stream_data
# per new session, so it refreshes less often than the live half.
HISTORY_REFRESH_SECONDS = int(os.environ.get("HISTORY_REFRESH_SECONDS", "600"))
# Ceiling on history rows per call. ~1000 rows per 30 days today, so ~5x
# headroom; plex_client_history_truncated says so if the window outgrows it.
MAX_ROWS = int(os.environ.get("MAX_ROWS", "5000"))
# Bound on stream_data fetches per refresh so a cold cache cannot run long;
# the rest are picked up on the next refresh.
MAX_FETCH = int(os.environ.get("MAX_FETCH", "2000"))
HTTP_TIMEOUT = 30

PROM = os.path.join(TEXTFILE_DIR, "plex_client.prom")
BASE = ("user", "player", "platform", "product", "location")


def log(msg):
    print(f"[plex-client-metrics] {msg}", flush=True)


def api_key():
    with open(TAUTULLI_CFG, encoding="utf-8", errors="replace") as f:
        for line in f:
            m = re.match(r"\s*api_key\s*=\s*(\w+)", line)
            if m:
                return m.group(1)
    raise RuntimeError(f"no api_key in {TAUTULLI_CFG}")


def call(key, cmd, **params):
    q = urllib.parse.urlencode({"apikey": key, "cmd": cmd, **params})
    with urllib.request.urlopen(f"{TAUTULLI_URL}/api/v2?{q}", timeout=HTTP_TIMEOUT) as r:
        body = json.load(r)
    resp = body.get("response") or {}
    if resp.get("result") != "success":
        raise RuntimeError(f"{cmd}: {resp.get('message') or 'not success'}")
    return resp.get("data")


def s(v, default="unknown"):
    """Label value: Tautulli returns '', None, 0 and strings interchangeably."""
    if v is None:
        return default
    v = str(v).strip()
    return v if v else default


def num(v):
    try:
        return float(v)
    except (TypeError, ValueError):
        return None


def esc(v):
    # Prometheus text format: backslash, then quote, then newline. Real player
    # names here include `65" TCL Roku TV`; an unescaped quote makes
    # node_exporter reject the whole file.
    return str(v).replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")


def resolution(full, height):
    """'1080p' / '4k' / '720p' / 'sd', or 'none' for audio."""
    full = s(full, "")
    if full:
        return full.lower()
    h = num(height)
    if not h:
        return "none"
    if h >= 1600:
        return "4k"
    if h >= 900:
        return "1080p"
    if h >= 650:
        return "720p"
    if h >= 480:
        return "480p"
    return "sd"


def hw(v):
    return "true" if str(v).strip() in ("1", "True", "true") else "false"


def base_labels(r):
    return {
        "user": s(r.get("friendly_name") or r.get("user")),
        "player": s(r.get("player")),
        "platform": s(r.get("platform")),
        "product": s(r.get("product")),
        "location": s(r.get("location")),
    }


# ---------------------------------------------------------------- history


def load_cache():
    try:
        with open(CACHE_FILE, encoding="utf-8") as f:
            c = json.load(f)
        if isinstance(c, dict) and isinstance(c.get("rows"), dict):
            return c
    except (OSError, ValueError):
        pass
    return {"refreshed": 0, "filtered": 0, "rows": {}}


def save_cache(c):
    tmp = f"{CACHE_FILE}.{os.getpid()}"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(c, f, separators=(",", ":"))
    os.replace(tmp, CACHE_FILE)


def compact(h, sd):
    """The fields the aggregates need, from a history row and its stream data."""
    return {
        "started": int(num(h.get("started")) or 0),
        "stopped": int(num(h.get("stopped")) or 0),
        **base_labels(h),
        "media_type": s(h.get("media_type")),
        "transcode_decision": s(h.get("transcode_decision")),
        "play_duration": num(h.get("play_duration")) or 0,
        "paused": num(h.get("paused_counter")) or 0,
        "percent_complete": num(h.get("percent_complete")),
        "relayed": hw(h.get("relayed")),
        "secure": hw(h.get("secure")),
        "video_decision": s(sd.get("video_decision") or sd.get("stream_video_decision"), "none"),
        "audio_decision": s(sd.get("audio_decision") or sd.get("stream_audio_decision"), "none"),
        "subtitle_decision": s(sd.get("stream_subtitle_decision"), "none"),
        "container_decision": s(sd.get("stream_container_decision"), "none"),
        "quality_profile": s(sd.get("quality_profile")),
        "source_resolution": resolution(sd.get("video_full_resolution"), sd.get("video_height")),
        "stream_resolution": resolution(sd.get("stream_video_full_resolution"), sd.get("stream_video_height")),
        "source_video_codec": s(sd.get("video_codec"), "none"),
        "stream_video_codec": s(sd.get("stream_video_codec"), "none"),
        "source_dynamic_range": s(sd.get("video_dynamic_range"), "none"),
        "stream_dynamic_range": s(sd.get("stream_video_dynamic_range"), "none"),
        "source_audio_codec": s(sd.get("audio_codec"), "none"),
        "stream_audio_codec": s(sd.get("stream_audio_codec"), "none"),
        "stream_audio_channels": s(sd.get("stream_audio_channels"), "none"),
        "stream_container": s(sd.get("stream_container"), "none"),
        "hw_decode": hw(sd.get("transcode_hw_decoding")),
        "hw_encode": hw(sd.get("transcode_hw_encoding")),
        "source_bitrate": num(sd.get("bitrate")),
        "stream_bitrate": num(sd.get("stream_bitrate")),
    }


def refresh_history(key, cache, now):
    """Pull the window, fetch stream data for unseen sessions, prune old ones.
    Returns the number of stream_data fetches that failed."""
    after = time.strftime("%Y-%m-%d", time.gmtime(now - WINDOW_DAYS * 86400))
    data = call(key, "get_history", after=after, length=MAX_ROWS, grouping=0)
    rows = (data or {}).get("data") or []
    cache["filtered"] = int(num((data or {}).get("recordsFiltered")) or 0)

    seen = set()
    errors = fetched = 0
    for h in rows:
        rid = h.get("row_id") or h.get("id")
        # Rows without an id are sessions still in progress; the live half
        # covers them, history picks them up once they finish.
        if not rid:
            continue
        rid = str(rid)
        seen.add(rid)
        if rid in cache["rows"]:
            continue
        if fetched >= MAX_FETCH:
            continue
        fetched += 1
        try:
            sd = call(key, "get_stream_data", row_id=rid) or {}
        except Exception as e:  # one bad row must not sink the refresh
            errors += 1
            log(f"get_stream_data row {rid}: {e}")
            continue
        cache["rows"][rid] = compact(h, sd)

    # Drop sessions that left the window (or were deleted from Tautulli).
    cache["rows"] = {k: v for k, v in cache["rows"].items() if k in seen}
    cache["refreshed"] = now
    log(f"history: {len(rows)} rows, {fetched} new fetched, {errors} errors, {len(cache['rows'])} cached")
    return errors


def pct(values, q):
    v = sorted(values)
    if not v:
        return None
    i = min(len(v) - 1, max(0, int(round(q * (len(v) - 1)))))
    return v[i]


def history_series(cache):
    """Aggregate cached sessions into {metric: {labels_tuple: value}}."""
    out = {}

    def add(metric, labels, value=1.0):
        key = tuple(labels.items())
        m = out.setdefault(metric, {})
        m[key] = m.get(key, 0.0) + value

    per_client = {}
    for r in cache["rows"].values():
        b = {k: r[k] for k in BASE}
        add("plex_client_sessions", {**b,
            "media_type": r["media_type"],
            "transcode_decision": r["transcode_decision"],
            "video_decision": r["video_decision"],
            "audio_decision": r["audio_decision"],
            "subtitle_decision": r["subtitle_decision"]})
        add("plex_client_resolution_sessions", {**b,
            "source_resolution": r["source_resolution"],
            "stream_resolution": r["stream_resolution"]})
        add("plex_client_quality_profile_sessions", {**b, "quality_profile": r["quality_profile"]})
        add("plex_client_format_sessions", {**b,
            "source_video_codec": r["source_video_codec"],
            "stream_video_codec": r["stream_video_codec"],
            "source_dynamic_range": r["source_dynamic_range"],
            "stream_dynamic_range": r["stream_dynamic_range"],
            "source_audio_codec": r["source_audio_codec"],
            "stream_audio_codec": r["stream_audio_codec"],
            "stream_audio_channels": r["stream_audio_channels"],
            "stream_container": r["stream_container"],
            "container_decision": r["container_decision"]})
        if r["video_decision"] == "transcode":
            add("plex_client_video_transcode_hw_sessions", {**b,
                "hw_decode": r["hw_decode"], "hw_encode": r["hw_encode"]})
        if r["relayed"] == "true":
            add("plex_client_relayed_sessions", b)
        add("plex_client_watch_seconds", b, r["play_duration"])
        add("plex_client_paused_seconds", b, r["paused"])

        c = per_client.setdefault(tuple(b.items()), {"stream": [], "source": [], "complete": [], "last": 0})
        if r["stream_bitrate"]:
            c["stream"].append(r["stream_bitrate"])
        if r["source_bitrate"]:
            c["source"].append(r["source_bitrate"])
        if r["percent_complete"] is not None:
            c["complete"].append(r["percent_complete"])
        c["last"] = max(c["last"], r["stopped"] or r["started"])

    for key, c in per_client.items():
        b = dict(key)
        for stat, q in (("p10", 0.10), ("p50", 0.50), ("p90", 0.90), ("max", 1.0)):
            v = pct(c["stream"], q)
            if v is not None:
                out.setdefault("plex_client_stream_bitrate_kbps", {})[tuple({**b, "stat": stat}.items())] = v
        if c["stream"]:
            out.setdefault("plex_client_stream_bitrate_kbps", {})[tuple({**b, "stat": "avg"}.items())] = sum(c["stream"]) / len(c["stream"])
        if c["source"]:
            out.setdefault("plex_client_source_bitrate_kbps", {})[tuple({**b, "stat": "avg"}.items())] = sum(c["source"]) / len(c["source"])
        if c["complete"]:
            out.setdefault("plex_client_percent_complete_avg", {})[key] = sum(c["complete"]) / len(c["complete"])
        if c["last"]:
            out.setdefault("plex_client_last_played_timestamp_seconds", {})[key] = c["last"]
    return out


# ---------------------------------------------------------------- live


def live_series(key):
    data = call(key, "get_activity") or {}
    out = {}
    for x in data.get("sessions") or []:
        b = base_labels(x)
        # session_key keeps two streams from one user on one player apart; a
        # duplicate label set would make node_exporter drop the whole file.
        b["session"] = s(x.get("session_key") or x.get("session_id"))
        info = {**b,
                "state": s(x.get("state")),
                "media_type": s(x.get("media_type")),
                "transcode_decision": s(x.get("transcode_decision")),
                "video_decision": s(x.get("video_decision") or x.get("stream_video_decision"), "none"),
                "audio_decision": s(x.get("audio_decision") or x.get("stream_audio_decision"), "none"),
                "subtitle_decision": s(x.get("stream_subtitle_decision") or x.get("subtitle_decision"), "none"),
                "container_decision": s(x.get("stream_container_decision") or x.get("container_decision"), "none"),
                "quality_profile": s(x.get("quality_profile")),
                "source_resolution": resolution(x.get("video_full_resolution"), x.get("video_height")),
                "stream_resolution": resolution(x.get("stream_video_full_resolution"), x.get("stream_video_height")),
                "source_video_codec": s(x.get("video_codec"), "none"),
                "stream_video_codec": s(x.get("stream_video_codec"), "none"),
                "source_dynamic_range": s(x.get("video_dynamic_range"), "none"),
                "stream_dynamic_range": s(x.get("stream_video_dynamic_range"), "none"),
                "stream_audio_codec": s(x.get("stream_audio_codec"), "none"),
                "hw_decode": hw(x.get("transcode_hw_decoding")),
                "hw_encode": hw(x.get("transcode_hw_encoding")),
                "relayed": hw(x.get("relayed")),
                "secure": hw(x.get("secure")),
                "title": s(x.get("full_title"))}
        out.setdefault("plex_client_live_session_info", {})[tuple(info.items())] = 1
        bk = tuple(b.items())
        for metric, field in (("plex_client_live_bandwidth_kbps", "bandwidth"),
                              ("plex_client_live_stream_bitrate_kbps", "stream_bitrate"),
                              ("plex_client_live_source_bitrate_kbps", "bitrate"),
                              ("plex_client_live_progress_percent", "progress_percent")):
            v = num(x.get(field))
            if v is not None:
                out.setdefault(metric, {})[bk] = v
        speed = num(x.get("transcode_speed"))
        if speed is not None:
            out.setdefault("plex_client_live_transcode_speed", {})[bk] = speed
        out.setdefault("plex_client_live_throttled", {})[bk] = 1 if str(x.get("throttled")) == "1" or x.get("transcode_throttled") in (1, "1") else 0
    return out


# ---------------------------------------------------------------- output

HELP = {
    "plex_client_sessions": "Sessions in the window by playback decision (video/audio/subtitle/overall)",
    "plex_client_resolution_sessions": "Sessions in the window by source and delivered video resolution",
    "plex_client_quality_profile_sessions": "Sessions in the window by the quality profile the client requested",
    "plex_client_format_sessions": "Sessions in the window by source and delivered codecs, dynamic range and container",
    "plex_client_video_transcode_hw_sessions": "Video transcodes in the window by hardware decode/encode use",
    "plex_client_relayed_sessions": "Sessions in the window that went through the Plex relay (bandwidth-capped)",
    "plex_client_watch_seconds": "Seconds played in the window",
    "plex_client_paused_seconds": "Seconds paused in the window",
    "plex_client_stream_bitrate_kbps": "Delivered stream bitrate per session in the window, by stat",
    "plex_client_source_bitrate_kbps": "Source file bitrate per session in the window, by stat",
    "plex_client_percent_complete_avg": "Average percent of each item watched in the window",
    "plex_client_last_played_timestamp_seconds": "Unix time this client last stopped a session",
    "plex_client_live_session_info": "One series per active session carrying its decision and format labels",
    "plex_client_live_bandwidth_kbps": "Bandwidth Plex reserves for the active session",
    "plex_client_live_stream_bitrate_kbps": "Bitrate being delivered to the active session",
    "plex_client_live_source_bitrate_kbps": "Bitrate of the source file of the active session",
    "plex_client_live_progress_percent": "Progress through the item of the active session",
    "plex_client_live_transcode_speed": "Transcoder speed for the active session; below 1 the client will buffer",
    "plex_client_live_throttled": "1 if the transcode for the active session is throttled (ahead of playback)",
}


def fmt(v):
    # Not %g: it rounds Unix timestamps to six significant digits.
    v = float(v)
    return str(int(v)) if v.is_integer() else f"{v:.3f}"


def render(series):
    lines = []
    for metric in sorted(series):
        lines.append(f"# HELP {metric} {HELP.get(metric, metric)}")
        lines.append(f"# TYPE {metric} gauge")
        for labels, v in sorted(series[metric].items()):
            ls = ",".join(f'{k}="{esc(val)}"' for k, val in labels)
            lines.append(f"{metric}{{{ls}}} {fmt(v)}" if ls else f"{metric} {fmt(v)}")
    return lines


def main():
    now = int(time.time())
    try:
        key = api_key()
    except (OSError, RuntimeError) as e:
        log(str(e))
        return 2

    series = {}
    status = {}

    try:
        series.update(live_series(key))
        status["plex_client_live_ok"] = 1
    except Exception as e:
        log(f"live: {e}")
        status["plex_client_live_ok"] = 0

    cache = load_cache()
    errors = 0
    history_ok = 1
    if now - int(cache.get("refreshed") or 0) >= HISTORY_REFRESH_SECONDS:
        try:
            errors = refresh_history(key, cache, now)
            save_cache(cache)
        except Exception as e:
            # Keep publishing from the cache: a finished session's numbers do
            # not go stale, and _history_ok / _refreshed say it is behind.
            log(f"history: {e}")
            history_ok = 0
    series.update(history_series(cache))

    status.update({
        "plex_client_history_ok": history_ok,
        "plex_client_history_refreshed_timestamp_seconds": int(cache.get("refreshed") or 0),
        "plex_client_history_sessions_cached": len(cache["rows"]),
        "plex_client_history_truncated": 1 if int(cache.get("filtered") or 0) > MAX_ROWS else 0,
        "plex_client_history_stream_data_errors": errors,
        "plex_client_window_seconds": WINDOW_DAYS * 86400,
        "plex_client_metrics_last_run_timestamp_seconds": now,
    })
    for k, v in status.items():
        series[k] = {(): v}

    os.makedirs(TEXTFILE_DIR, exist_ok=True)
    tmp = f"{PROM}.{os.getpid()}"
    with open(tmp, "w", encoding="utf-8") as f:
        f.write("\n".join(render(series)) + "\n")
    os.replace(tmp, PROM)
    live = len(series.get("plex_client_live_session_info", {}))
    log(f"wrote {PROM}: live_sessions={live} cached_sessions={len(cache['rows'])} "
        f"live_ok={status['plex_client_live_ok']} history_ok={history_ok}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
