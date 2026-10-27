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


def test_cover_the_flow_calls_an_attachment_is_dropped():
    # Bound (1996): the flow hands its 8 MJPEG covers over as codec_type
    # "attachment", not "video", so a video-only match kept them and the job
    # stalled on time=N/A (2026-10-20, 10-23, 10-24). Fonts are attachments too
    # and the subtitles need them.
    out = run([
        stream(0, "video", "h264", width=1920, height=1080),
        stream(1, "audio", "ac3", channels=6, tags={"language": "eng"}),
        stream(7, "attachment", "mjpeg", tags={"filename": "Bound_BD_Front.jpg", "mimetype": "image/jpeg"}),
        stream(8, "attachment", "ttf", tags={"filename": "arial.ttf", "mimetype": "font/ttf"}),
    ])
    assert [s["index"] for s in out if s["removed"]] == [7], out


def test_mp4_text_subtitles_are_converted_for_matroska():
    # Tenzing (2026), Rosemead (2025): mp4 WEBDLs carry mov_text subtitles, which
    # matroska cannot hold, so copying them failed the mux ("Subtitle codec 94213
    # is not supported", 2026-10-24).
    out = run([
        stream(0, "video", "h264", width=1920, height=1080),
        stream(1, "audio", "eac3", channels=6, tags={"language": "eng"}),
        stream(2, "subtitle", "mov_text", tags={"language": "eng"}),
    ])
    sub = next(s for s in out if s["index"] == 2)
    assert not sub["removed"] and sub["outputArgs"][-2:] == ["-c:{outputIndex}", "srt"], sub


def test_mp4_cover_without_a_mimetype_is_dropped():
    # Tenzing (2026): an mp4 cover reaches the flow with no tags at all, only the
    # attached_pic disposition, so a mimetype match sent it to hevc_nvenc.
    out = run([
        stream(0, "video", "h264", width=1920, height=1080),
        stream(1, "audio", "eac3", channels=6, tags={"language": "eng"}),
        stream(44, "attachment", "mjpeg", tags=None, disposition={"attached_pic": 1}),
    ])
    assert [s["index"] for s in out if s["removed"]] == [44], out


def test_mp4_data_stream_is_dropped():
    # Tenzing (2026) carries a bin_data text track beside its subtitles; matroska
    # takes no data streams, so copying it fails the mux once mov_text is fixed.
    out = run([
        stream(0, "video", "h264", width=1920, height=1080),
        stream(1, "audio", "eac3", channels=6, tags={"language": "eng"}),
        stream(43, "data", "bin_data", tags={"handler_name": "SubtitleHandler"}),
    ])
    assert [s["index"] for s in out if s["removed"]] == [43], out


if __name__ == "__main__":
    for name, fn in sorted(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok  ", name)
