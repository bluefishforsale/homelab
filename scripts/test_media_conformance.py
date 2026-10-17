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


if __name__ == "__main__":
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            fn()
            print(f"ok  {name}")
