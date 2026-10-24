#!/usr/bin/env python3
"""Pins the tdarr flow's audio policy: run `python3 scripts/test_audio_policy.py` (needs node, no pytest).

files/ocean-tdarr/conf/audio-policy.js builds the ffmpeg arguments for every
transcode; a bad filter fails a whole encode at its last frame.
"""
import json
import os
import subprocess

HERE = os.path.dirname(os.path.abspath(__file__))
POLICY = os.path.join(HERE, "..", "files", "ocean-tdarr", "conf", "audio-policy.js")

RUN = """
const policy = require(process.argv[1]);
const streams = JSON.parse(process.argv[2]);
const args = {jobLog: () => {}, inputFileObj: {}, variables: {ffmpegCommand: {streams}}};
policy(args).then((r) => console.log(JSON.stringify(r.variables.ffmpegCommand.streams)));
"""


def stream(index, kind, codec, **kw):
    s = {"index": index, "codec_type": kind, "codec_name": codec, "tags": {}, "disposition": {},
         "removed": False, "mapArgs": ["-map", f"0:{index}"], "inputArgs": [], "outputArgs": []}
    s.update(kw)
    return s


def run(streams):
    out = subprocess.run(["node", "-e", RUN, os.path.abspath(POLICY), json.dumps(streams)],
                         capture_output=True, text=True, check=True).stdout
    return json.loads(out)


def dialogue_filter(streams):
    for s in run(streams):
        args = s["outputArgs"]
        if "title=Stereo (dialogue boost)" in args:
            return args[args.index("-filter:{outputIndex}") + 1]
    raise AssertionError("no dialogue track built")


def test_dialogue_track_scrubs_nan_after_loudnorm():
    # Last Cab to Darwin: its DTS-HD track reconfigures at 2:01:38, loudnorm emits
    # NaN, and AAC fails the 68-minute encode at the last frame.
    f = dialogue_filter([
        stream(0, "video", "h264", width=1920, height=1080),
        stream(1, "audio", "dts", channels=6, channel_layout="5.1(side)", profile="DTS-HD MA",
               tags={"language": "eng"}),
    ])
    after = f.split("loudnorm=", 1)[1]
    assert "isnan(" in after and "isinf(" in after, f


def test_cover_dropped_when_stored_probe_lost_its_tags():
    # Bound (1996): tdarr's stored ffProbeData keeps codec_name for its 8 MJPEG
    # covers but drops tags, disposition and frame rate, so the mimetype match
    # missed them and hevc_nvenc stalled on time=N/A (2026-10-20, 10-23).
    out = run([
        stream(0, "video", "h264", width=1920, height=1080),
        stream(1, "audio", "ac3", channels=6, tags={"language": "eng"}),
        stream(7, "video", "mjpeg", width=1288, height=1600, tags={}, disposition=None),
        stream(9, "video", "mjpeg", width=0, height=0, tags={}, disposition=None),
    ])
    assert [s["index"] for s in out if s["removed"]] == [7, 9], out


if __name__ == "__main__":
    for name, fn in sorted(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok  ", name)
