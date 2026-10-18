#!/usr/bin/env python3
"""Which library files meet the media standard, and what each one needs.

The standard is what the tdarr flow and its audio policy
(files/ocean-tdarr/conf/audio-policy.js) produce, plus an English subtitle
from the release itself:

  video      one HEVC stream, no stale mkvmerge BPS left from a re-encode; or the
             source codec, when the flow found HEVC came out larger and kept it
  audio      default "Stereo (dialogue boost)" track, English when the file has
             English audio; surround only AC3/EAC3;
             nothing lossless
  subtitles  at least one English (or untagged) subtitle stream

A file that fails only on video or audio is fixed in place by a tdarr pass. A
file that is double-muxed or has no English subtitle has to be re-downloaded:
the old flow stripped subtitles (-sn) and nothing local can put them back.

Reads the ffprobe tdarr already stores for every file, so a full sweep reads no
media. Read-only.

  media-conformance.py                    # summary per library and bucket
  media-conformance.py --list refetch     # paths needing a re-download
  media-conformance.py --json refetch     # same, with reasons, for media-refetch.py
"""
import argparse
import collections
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tdarr  # noqa: E402

DIALOGUE_TITLE = "Stereo (dialogue boost)"
LOSSLESS = {"truehd", "flac", "alac", "mlp"}
ENGLISH = {"eng", "en", "und", ""}
ENGLISH_AUDIO = {"eng", "en"}
# Stats mkvmerge writes; on a stream ffmpeg re-encoded they describe the source.
STALE = ("BPS", "NUMBER_OF_BYTES")


def tags(s):
    return {k.upper(): v for k, v in (s.get("tags") or {}).items()}


def is_lossless(s):
    c = s.get("codec_name") or ""
    return (c in LOSSLESS or c.startswith("pcm_")
            or (c == "dts" and any(p in (s.get("profile") or "") for p in ("MA", "HRA", "X"))))


def reasons(streams):
    """Every way a file misses the standard; empty when it conforms."""
    out = []
    video = [s for s in streams if s.get("codec_type") == "video"
             and not (s.get("disposition") or {}).get("attached_pic")]
    audio = [s for s in streams if s.get("codec_type") == "audio"]
    subs = [s for s in streams if s.get("codec_type") == "subtitle"]
    # NOTE: the old flow's double mux repeats the main video at its own size. A
    # second video at another size is part of the release (a PIP commentary, a
    # cover), and a re-download brings it straight back.
    if len({(s.get("width"), s.get("height")) for s in video}) < len(video):
        out.append("double-muxed")
    if not any((tags(s).get("LANGUAGE") or "").lower() in ENGLISH for s in subs):
        out.append("no-english-subs")
    # The flow keeps the source video when HEVC came out larger (a lean WEB-DL),
    # and still runs the audio policy; its dialogue track marks that decision.
    has_dialogue = any(tags(s).get("TITLE") == DIALOGUE_TITLE for s in audio)
    if video and video[0].get("codec_name") != "hevc" and not has_dialogue:
        out.append("video-not-hevc")
    if video and any(k in tags(video[0]) for k in STALE) and "Lavc" in tags(video[0]).get("ENCODER", ""):
        out.append("stale-bps")
    if audio:
        default = [s for s in audio if (s.get("disposition") or {}).get("default")]
        dialogue = [s for s in default if tags(s).get("TITLE") == DIALOGUE_TITLE]
        if not dialogue:
            out.append("no-dialogue-track")
        # English wins over a release's own default, even when that makes a dub
        # the dialogue track of a foreign film.
        lang = lambda s: (tags(s).get("LANGUAGE") or "").lower()
        if (dialogue and lang(dialogue[0]) not in ENGLISH_AUDIO
                and any(lang(s) in ENGLISH_AUDIO and "COMMENT" not in tags(s).get("TITLE", "").upper() for s in audio)):
            out.append("dialogue-not-english")
        if any(is_lossless(s) for s in audio):
            out.append("lossless-audio")
        if any((s.get("channels") or 0) > 2 and s.get("codec_name") not in ("ac3", "eac3") for s in audio):
            out.append("surround-not-ac3")
    return out


def bucket(why):
    if not why:
        return "ok"
    if "double-muxed" in why or "no-english-subs" in why:
        return "refetch"
    return "tdarr"


def sweep():
    for table in tdarr.FILE_TABLES:
        for r in tdarr.rows(table):
            lib = tdarr.library_of(r.get("file", ""))
            if lib == "other":
                continue
            why = reasons(((r.get("ffProbeData") or {}).get("streams")) or [])
            yield {"file": r["file"], "library": lib, "size_gb": round((r.get("file_size") or 0) / 1024, 2),
                   "bucket": bucket(why), "reasons": why}


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    g = ap.add_mutually_exclusive_group()
    g.add_argument("--list", choices=("ok", "tdarr", "refetch"), help="print the paths in one bucket")
    g.add_argument("--json", choices=("ok", "tdarr", "refetch"), help="one JSON object per file in a bucket")
    a = ap.parse_args()

    if a.list or a.json:
        for f in sweep():
            if f["bucket"] == (a.list or a.json):
                print(json.dumps(f) if a.json else f["file"])
        return

    count, size, why = collections.Counter(), collections.Counter(), collections.Counter()
    for f in sweep():
        count[(f["library"], f["bucket"])] += 1
        size[(f["library"], f["bucket"])] += f["size_gb"]
        for r in f["reasons"]:
            why[(f["library"], r)] += 1
    print(f"{'library':8} {'bucket':8} {'files':>7} {'TB':>6}")
    for lib in ("movies", "tv"):
        for b in ("ok", "tdarr", "refetch"):
            print(f"{lib:8} {b:8} {count[(lib, b)]:7} {size[(lib, b)] / 1024:6.1f}")
    print(f"\n{'reason':20} {'movies':>7} {'tv':>7}")
    for r in sorted({r for _, r in why}):
        print(f"{r:20} {why[('movies', r)]:7} {why[('tv', r)]:7}")


if __name__ == "__main__":
    main()
