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


def test_foreign_forced_subtitles_are_dropped():
    # Blade Runner 2049 and Once Upon a Time in the West (2026-10-28): a German
    # forced track has a cue every half hour or so, ffmpeg's progress time waits
    # on it, and tdarr's stall detector cancels a job that is copying at 2.8x.
    # English forced subtitles are the ones anyone here reads; they stay.
    out = run([
        stream(0, "video", "hevc", width=1920, height=1080),
        stream(1, "audio", "ac3", channels=6, tags={"language": "eng"}),
        stream(2, "subtitle", "subrip", tags={"language": "ger", "title": "forced"}),
        stream(3, "subtitle", "hdmv_pgs_subtitle", tags={"language": "ger", "title": "Deutsch (PGS)"}),
        stream(4, "subtitle", "hdmv_pgs_subtitle", tags={"language": "ger"}, disposition={"forced": 1}),
        stream(5, "subtitle", "subrip", tags={"language": "eng", "title": "English Forced"}),
    ])
    assert [s["index"] for s in out if s["removed"]] == [2, 4], out


def test_subtitle_ffmpeg_cannot_identify_is_dropped():
    # The Mongoose (2026-10-29): a WebVTT track in matroska probes as codec
    # "unknown", the matroska muxer refuses it ("Subtitle codec 0 is not
    # supported") and the whole job fails before the first frame.
    out = run([
        stream(0, "video", "h264", width=1920, height=1080),
        stream(1, "audio", "aac", channels=2, tags={"language": "eng"}),
        stream(2, "subtitle", "unknown", tags={"language": "eng"}),
    ])
    assert [s["index"] for s in out if s["removed"]] == [2], out



def dialogue_source(streams):
    for s in run(streams):
        if "title=Stereo (dialogue boost)" in s["outputArgs"]:
            return s["mapArgs"][1], s["outputArgs"][s["outputArgs"].index("-metadata:s:{outputIndex}", s["outputArgs"].index("title=Stereo (dialogue boost)")) + 1]
    raise AssertionError("no dialogue track built")


def test_audio_description_does_not_become_the_dialogue_track():
    # Amelie (2026-11-11): French 5.1 plus two English stereo tracks, an audio
    # description and the director's commentary. English wins, so the narration
    # for blind viewers became the default track.
    assert dialogue_source([
        stream(0, "video", "hevc", width=1920, height=1080),
        stream(1, "audio", "ac3", channels=6, tags={"language": "fre"}, disposition={"default": 1}),
        stream(2, "audio", "ac3", channels=2, tags={"language": "eng", "title": "Audio.Description"}),
        stream(3, "audio", "ac3", channels=2, tags={"language": "eng", "title": "Director's.Commentary"}),
    ]) == ("0:1", "language=fre")


def test_audio_description_flagged_only_by_disposition_is_skipped():
    assert dialogue_source([
        stream(0, "video", "hevc", width=1920, height=1080),
        stream(1, "audio", "ac3", channels=6, tags={"language": "jpn"}, disposition={"default": 1}),
        stream(2, "audio", "aac", channels=2, tags={"language": "eng"}, disposition={"visual_impaired": 1}),
    ]) == ("0:1", "language=jpn")


def test_dialogue_track_built_from_a_side_track_is_rebuilt():
    # Files the old policy already did: an English dialogue track from the audio
    # description, next to the French original.
    out = run([
        stream(0, "video", "hevc", width=1920, height=1080),
        stream(1, "audio", "aac", channels=2, tags={"language": "eng", "title": "Stereo (dialogue boost)"}),
        stream(2, "audio", "ac3", channels=6, tags={"language": "fre"}),
        stream(3, "audio", "ac3", channels=2, tags={"language": "eng", "title": "Audio.Description"}),
    ])
    assert [s["index"] for s in out if s["removed"]] == [1], out
    assert any(s["mapArgs"] == ["-map", "0:2"] and "title=Stereo (dialogue boost)" in s["outputArgs"] for s in out)

if __name__ == "__main__":
    for name, fn in sorted(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok  ", name)
