#!/usr/bin/env python3
"""Pins the plex-client-metrics math: run
`python3 files/ocean-plex/test_plex_client_metrics.py` (no pytest).

Field names and units here come from a live Tautulli (2026-11-15):
view_offset is milliseconds, transcode_max_offset_available is seconds, and
direct-play sessions carry '' for every transcode field.
"""
import importlib.util
import json
import os
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("pcm", os.path.join(HERE, "plex-client-metrics.py"))
pcm = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(pcm)

TRANSCODE = {
    "friendly_name": "alice", "player": 'Den 65" TCL Roku TV', "platform": "Roku",
    "product": "Plex for Roku", "location": "wan", "session_key": "41",
    "state": "playing", "media_type": "episode", "transcode_decision": "transcode",
    "video_decision": "transcode", "audio_decision": "copy", "stream_subtitle_decision": "burn",
    "container_decision": "transcode", "quality_profile": "Original",
    "video_full_resolution": "1080p", "stream_video_full_resolution": "1080p",
    "video_codec": "hevc", "stream_video_codec": "h264",
    "transcode_hw_requested": 1, "transcode_hw_full_pipeline": 0,
    "transcode_hw_decoding": 1, "transcode_hw_encoding": 0,
    "transcode_speed": "0.8", "transcode_throttled": 0, "throttled": "0",
    "transcode_key": "/transcode/sessions/abc",
    "view_offset": "3556000", "transcode_max_offset_available": 3598,
    "bandwidth": "14784", "stream_bitrate": "14080", "bitrate": "9974", "progress_percent": "99",
    "full_title": "Show - Ep",
}
DIRECT = {
    "friendly_name": "bob", "player": "Living Room TV", "platform": "tvOS",
    "product": "Plex for Apple TV", "location": "lan", "session_key": "42",
    "transcode_decision": "direct play", "video_decision": "direct play",
    "transcode_hw_requested": 0, "transcode_hw_full_pipeline": 0,
    "transcode_speed": "", "transcode_key": "", "view_offset": "5097221",
    "transcode_max_offset_available": 0,
}


def live(sessions):
    pcm.call = lambda key, cmd, **p: {"sessions": sessions}
    return pcm.live_series("k")


def only(series, metric):
    (labels, value), = series[metric].items()
    return dict(labels), value


def test_buffer_is_transcoder_offset_seconds_minus_viewer_offset_ms():
    labels, v = only(live([TRANSCODE]), "plex_client_live_transcode_buffer_seconds")
    assert v == 42.0, v
    assert labels["player"] == 'Den 65" TCL Roku TV'


def test_direct_play_has_no_buffer_or_speed_series():
    s = live([DIRECT])
    assert "plex_client_live_transcode_buffer_seconds" not in s
    assert "plex_client_live_transcode_speed" not in s


def test_info_carries_hw_requested_and_full_pipeline_so_cpu_fallback_is_visible():
    labels, _ = only(live([TRANSCODE]), "plex_client_live_session_info")
    assert labels["hw_requested"] == "true"
    assert labels["hw_full_pipeline"] == "false"
    assert labels["hw_encode"] == "false"


def test_two_sessions_on_one_player_stay_distinct():
    s = live([TRANSCODE, {**TRANSCODE, "session_key": "43"}])
    assert len(s["plex_client_live_session_info"]) == 2


def test_sessions_without_a_key_still_get_distinct_label_sets():
    # A duplicate label set makes node_exporter reject the whole file.
    nokey = {k: v for k, v in TRANSCODE.items() if k != "session_key"}
    s = live([nokey, nokey])
    assert len(s["plex_client_live_session_info"]) == 2
    assert len(s["plex_client_live_transcode_speed"]) == 2


def test_history_and_live_read_subtitle_decision_the_same_way():
    # Only subtitle_decision set: live and history must agree, since cause()
    # keys on it first.
    sd = {"subtitle_decision": "burn", "video_decision": "transcode"}
    hist = pcm.compact({"player": "p"}, sd)
    labels, _ = only(live([{**DIRECT, **sd, "stream_subtitle_decision": ""}]), "plex_client_live_session_info")
    assert hist["subtitle_decision"] == labels["subtitle_decision"] == "burn"


def row(**kw):
    base = pcm.compact(
        {"started": 100, "stopped": 200, "friendly_name": "alice", "player": "Roku Premere",
         "platform": "Roku", "product": "Plex for Roku", "location": "wan",
         "media_type": "movie", "transcode_decision": "transcode", "play_duration": 600},
        {"video_decision": "transcode", "audio_decision": "copy", "stream_subtitle_decision": "burn",
         "quality_profile": "4 Mbps 720p", "video_codec": "hevc", "stream_video_codec": "h264",
         "video_full_resolution": "1080p", "stream_video_full_resolution": "720p",
         "video_dynamic_range": "HDR", "transcode_hw_decoding": 1, "transcode_hw_encoding": 1,
         "bitrate": 9000, "stream_bitrate": 4000})
    base.update(kw)
    return base


def test_cause_metric_crosses_quality_subtitle_and_codec_for_video_transcodes_only():
    cache = {"rows": {"1": row(), "2": row(video_decision="direct play")}}
    s = pcm.history_series(cache)
    labels, v = only(s, "plex_client_video_transcode_cause_sessions")
    assert v == 1
    assert (labels["quality_profile"], labels["subtitle_decision"],
            labels["source_video_codec"], labels["stream_video_codec"],
            labels["source_resolution"], labels["stream_resolution"]) == \
        ("4 Mbps 720p", "burn", "hevc", "h264", "1080p", "720p")


def test_cause_is_the_first_reason_that_forces_a_video_transcode():
    # Precedence: a burn forces a transcode at any quality, a client cap
    # forces one on any codec, then codec, then resolution.
    cases = [
        (row(), "subtitle_burn"),
        (row(subtitle_decision="none"), "quality_cap"),
        (row(subtitle_decision="none", quality_profile="Original"), "codec"),
        (row(subtitle_decision="none", quality_profile="Original", stream_video_codec="hevc"), "resolution"),
        (row(subtitle_decision="none", quality_profile="Original", stream_video_codec="hevc",
             stream_resolution="1080p"), "other"),
        (row(subtitle_decision="none", quality_profile="unknown", stream_video_codec="hevc",
             stream_resolution="1080p"), "other"),
    ]
    for r, want in cases:
        assert pcm.cause(r) == want, (want, pcm.cause(r))
    labels, _ = only(pcm.history_series({"rows": {"1": row()}}), "plex_client_video_transcode_cause_sessions")
    assert labels["cause"] == "subtitle_burn"


def test_video_transcode_seconds_sums_play_duration_of_video_transcodes():
    cache = {"rows": {"1": row(), "2": row(), "3": row(video_decision="direct play")}}
    _, v = only(pcm.history_series(cache), "plex_client_video_transcode_seconds")
    assert v == 1200


def test_cache_from_another_schema_is_discarded_not_crashed_on():
    with tempfile.TemporaryDirectory() as d:
        pcm.CACHE_FILE = os.path.join(d, "c.json")
        with open(pcm.CACHE_FILE, "w") as f:
            json.dump({"refreshed": 5, "rows": {"1": {"player": "old"}}}, f)
        c = pcm.load_cache()
        assert c["rows"] == {} and c["refreshed"] == 0
        pcm.save_cache(c)
        assert pcm.load_cache()["version"] == pcm.CACHE_VERSION


def test_rendered_quote_in_player_name_is_escaped():
    out = "\n".join(pcm.render(live([TRANSCODE])))
    assert 'player="Den 65\\" TCL Roku TV"' in out


if __name__ == "__main__":
    tests = [(n, f) for n, f in sorted(globals().items()) if n.startswith("test_")]
    failed = 0
    for name, fn in tests:
        try:
            fn()
            print(f"ok   {name}")
        except Exception as e:  # report every failure, not just the first
            failed += 1
            print(f"FAIL {name}: {type(e).__name__}: {e}")
    print(f"{len(tests) - failed}/{len(tests)} passed")
    sys.exit(1 if failed else 0)
