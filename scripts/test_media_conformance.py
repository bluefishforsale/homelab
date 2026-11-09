#!/usr/bin/env python3
"""Pins media-conformance.reasons(): run `python3 scripts/test_media_conformance.py` (no pytest).

A wrong reason moves a file between buckets, and the refetch bucket deletes the
file and downloads it again.
"""
import importlib.util
import os

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("conformance", os.path.join(HERE, "media-conformance.py"))
conformance = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(conformance)


def video(w, h):
    return {"codec_type": "video", "codec_name": "hevc", "width": w, "height": h}


def test_second_video_at_another_size_is_not_double_muxed():
    # Chamber of Secrets Extended: a 480p picture-in-picture commentary video.
    why = conformance.reasons([video(1920, 1080), video(720, 480)])
    assert "double-muxed" not in why, why


def test_second_video_at_the_same_size_is_double_muxed():
    why = conformance.reasons([video(1920, 1080), video(1920, 1080)])
    assert "double-muxed" in why, why


def audio(lang, title="", default=False):
    return {"codec_type": "audio", "codec_name": "ac3", "channels": 6, "tags": {"language": lang, "title": title},
            "disposition": {"default": int(default)}}


def test_dialogue_track_from_another_language_when_english_audio_exists():
    # Kiss of the Spider Woman: a German release whose default track was German,
    # so the dialogue boost was built from it beside an English AC3 5.1.
    why = conformance.reasons([video(1920, 1080), audio("ger", "Stereo (dialogue boost)", default=True),
                               audio("ger", "AC3 5.1"), audio("eng", "AC3 5.1")])
    assert "dialogue-not-english" in why, why


def cover(w, h):
    return {**video(w, h), "tags": {"FILENAME": "cover.jpg", "MIMETYPE": "image/jpeg"}}


def test_cover_encoded_as_video_is_flagged_and_not_counted_as_video():
    # Blade Runner 2049: the old flow encoded two cover images as HEVC video
    # at 30000 fps, so ffmpeg reports time=N/A and tdarr's stall detector
    # cancels every job on the file. Two covers at one size are not a double mux.
    why = conformance.reasons([video(1920, 1080), cover(640, 360), cover(640, 360)])
    assert "cover-as-video" in why and "double-muxed" not in why, why


def test_2160p_is_refetched_because_the_library_tops_out_at_1080p():
    # Batman (1989) 2160p remux: already HEVC, so tdarr's audio-only branch
    # copied ~60 GB of video into gaming's 40G cache and took down its neighbours.
    why = conformance.reasons([video(3840, 2160)])
    assert "over-1080p" in why and conformance.bucket(why) == "refetch", why


def test_1080p_hdr_is_refetched_because_the_library_is_sdr_only():
    # Ballerina WEBDL-1080p in PQ: right size, wrong colour.
    why = conformance.reasons([{**video(1920, 1080), "color_transfer": "smpte2084"}])
    assert "hdr" in why and conformance.bucket(why) == "refetch", why


def test_dolby_vision_without_a_pq_transfer_tag_is_still_hdr():
    # DV profile 5 carries its own colour space, so ffprobe reports no PQ transfer.
    dv = {**video(1920, 1080), "side_data_list": [{"side_data_type": "DOVI configuration record"}]}
    assert "hdr" in conformance.reasons([dv])


if __name__ == "__main__":
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            fn()
            print(f"ok  {name}")
