#!/usr/bin/env python3
"""Repair library files the tdarr HEVC flow muxed with every stream twice.

Until #551 that flow's custom arguments added their own `-map` on top of the
per-stream maps tdarr already emits, so ~1,185 files carry two identical video
and audio tracks. Plex then transcodes HEVC a client could direct play, and an
Apple TV HD refused some of them outright. This drops the duplicates with a
lossless stream-copy remux; nothing is re-encoded.

DRY-RUN BY DEFAULT. Nothing on disk changes without --yes.

  media-dedupe-streams.py --path "Strange New Worlds/Season 3/" --limit 1
  media-dedupe-streams.py --path "Strange New Worlds/Season 3/" --limit 1 --yes --keep-orig
  media-dedupe-streams.py --yes                     # every affected file

Two streams are duplicates only when codec, shape and tags match. Audio must
also be byte-identical over its first packets, so a commentary track is never
dropped. The duplicated video is not a copy: the flow encoded it twice, and the
quality options it scoped to `:v:0` left the second encode on NVENC defaults
(about 3-4x the bitrate). Video therefore pairs on an identical timestamp and
keyframe timeline, and the larger encode is the one kept. Each remux is verified
(stream list, duration, size) before it replaces the original, which keeps its
owner and mode.

Runs from the laptop or agentbox: every file operation goes over ssh to ocean,
through the tdarr-node container that already holds ffmpeg and /data01/complete.
"""
import argparse
import json
import os
import shlex
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import media_clients as mc  # noqa: E402

HOST = os.environ.get("MEDIA_HOST", "ocean.home")
PLEX_DB = ("/config/Library/Application Support/Plex Media Server/"
           "Plug-in Support/Databases/com.plexapp.plugins.library.db")
# Plex paths -> host paths -> tdarr-node paths, for the same file.
PLEX_TO_HOST = {"/tv/": "/data01/complete/tv/", "/movies/": "/data01/complete/movies/"}
HOST_ROOT, NODE_ROOT = "/data01/complete/", "/media/"
# A stream is a duplicate only if all of these match an earlier stream.
SIGNATURE = ("codec_type", "codec_name", "profile", "width", "height", "pix_fmt",
             "channels", "sample_rate")
SIGNATURE_TAGS = ("title", "language", "BPS", "DURATION", "ENCODER")
# The muxer rewrites DURATION, and BPS is cleared below, so the remux is checked
# against the source without them.
VERIFY_TAGS = ("title", "language", "ENCODER")
# mkvmerge statistics copied from the original Blu-ray remux. They describe the
# AVC source, not the HEVC re-encode, and Plex trusts BPS: S03E02 advertised
# 30.7 Mbps and Plex sized its transcode target from that.
STALE_TAGS = ("BPS", "NUMBER_OF_BYTES", "NUMBER_OF_FRAMES", "_STATISTICS_TAGS",
              "_STATISTICS_WRITING_APP", "_STATISTICS_WRITING_DATE_UTC")
PACKETS_COMPARED = 64


def ocean(cmd, check=True):
    out = subprocess.run(["ssh", "-o", "BatchMode=yes", HOST, cmd],
                         capture_output=True, text=True)
    if check and out.returncode != 0:
        raise RuntimeError(f"ocean: {cmd[:160]}\n{out.stderr.strip()}")
    return out


def node(argv, check=True):
    return ocean("sudo -n docker exec tdarr-node " + " ".join(shlex.quote(a) for a in argv), check)


def affected(path_filter):
    """(rating_key, plex_path) for every part Plex sees with >1 video stream."""
    like = "%" + path_filter.replace("'", "''") + "%"
    sql = f"""
      SELECT mi.metadata_item_id, mp.file FROM media_parts mp
      JOIN media_items mi ON mi.id = mp.media_item_id
      WHERE mp.media_item_id IN (SELECT media_item_id FROM media_streams
                                 WHERE stream_type_id = 1 GROUP BY media_item_id HAVING COUNT(*) > 1)
        AND mp.file LIKE '{like}'
      ORDER BY mp.file;"""
    cmd = ("sudo -n docker exec -i plex '/usr/lib/plexmediaserver/Plex SQLite' -readonly "
           + shlex.quote(PLEX_DB))
    out = subprocess.run(["ssh", "-o", "BatchMode=yes", HOST, cmd], input=sql,
                         capture_output=True, text=True, check=True).stdout
    rows = [line.split("|", 1) for line in out.splitlines() if "|" in line]
    return [(int(rk), f) for rk, f in rows]


def host_path(plex_path):
    for prefix, root in PLEX_TO_HOST.items():
        if plex_path.startswith(prefix):
            return root + plex_path[len(prefix):]
    raise ValueError(f"no host mapping for Plex path {plex_path!r}")


def node_path(path):
    return NODE_ROOT + path[len(HOST_ROOT):]


def probe(npath):
    out = node(["ffprobe", "-v", "error", "-show_entries",
                "format=duration,size:stream=index," + ",".join(SIGNATURE) + ":stream_tags",
                "-of", "json", npath]).stdout
    return json.loads(out)


def signature(s, tag_names=SIGNATURE_TAGS):
    tags = {k.upper(): v for k, v in (s.get("tags") or {}).items()}
    return tuple(s.get(k) for k in SIGNATURE) + tuple(tags.get(k.upper()) for k in tag_names)


def packets(npath, index, interval):
    out = node(["ffprobe", "-v", "error", "-select_streams", str(index), "-read_intervals",
                interval, "-show_entries", "packet=pts,size,flags", "-of", "csv=p=0",
                npath]).stdout
    return [line.split(",") for line in out.split()]


def same_content(npath, kind, a, b):
    """True when stream b carries the same content as earlier stream a."""
    pa, pb = (packets(npath, i, f"%+#{PACKETS_COMPARED}") for i in (a, b))
    if kind == "audio":
        return pa == pb
    timeline = lambda ps: [(p[0], p[2]) for p in ps]  # pts, keyframe flag
    return timeline(pa) == timeline(pb)


def sample_bytes(npath, index):
    """Bytes in a mid-file window: which of two encodes carries more detail."""
    return sum(int(p[1]) for p in packets(npath, index, "600%+#400"))


def plan(info, npath):
    """Indexes to keep, and {dropped index: the stream kept in its place}."""
    groups = []  # [[index, ...]] of mutually duplicate streams, in file order
    for s in info["streams"]:
        sig = signature(s)
        for g in groups:
            first = info["streams"][g[0]]
            if signature(first) == sig and same_content(npath, s["codec_type"], g[0], s["index"]):
                g.append(s["index"])
                break
        else:
            groups.append([s["index"]])
    keep, dropped = [], {}
    for g in groups:
        winner = g[0] if len(g) == 1 else max(g, key=lambda i: sample_bytes(npath, i))
        keep.append(winner)
        dropped.update({i: winner for i in g if i != winner})
    # Video first: the source interleaves v, a, v, a and Plex plays the first video.
    kind = {st["index"]: st["codec_type"] for st in info["streams"]}
    return sorted(keep, key=lambda i: (kind[i] != "video", i)), dropped


def verify(src, out, keep):
    """Raise unless `out` holds exactly the kept streams, same length, plausible size."""
    by_index = {s["index"]: s for s in src["streams"]}
    want = [signature(by_index[i], VERIFY_TAGS) for i in keep]
    got = [signature(s, VERIFY_TAGS) for s in out["streams"]]
    if got != want:
        raise RuntimeError(f"stream list mismatch: kept {len(want)}, remux has {len(got)}")
    d_src, d_out = float(src["format"]["duration"]), float(out["format"]["duration"])
    if abs(d_src - d_out) > 1.0:
        raise RuntimeError(f"duration {d_out:.1f}s vs source {d_src:.1f}s")
    s_src, s_out = int(src["format"]["size"]), int(out["format"]["size"])
    # Dropping duplicated video roughly halves the file; anything near the
    # original size or far below half means the remux did not do what we think.
    if not 0.3 * s_src < s_out < 0.95 * s_src:
        raise RuntimeError(f"size {s_out} is {s_out / s_src:.0%} of source {s_src}")


def repair(hpath, keep, src, keep_orig):
    d, base = os.path.split(hpath)
    tmp = os.path.join(d, f".{base}.dedupe.mkv")
    maps = [a for i in keep for a in ("-map", f"0:{i}")]
    clear = [a for t in STALE_TAGS for a in ("-metadata:s", f"{t}=")]
    node(["ffmpeg", "-v", "error", "-nostdin", "-y", "-i", node_path(hpath), *maps,
          "-c", "copy", "-map_metadata", "0", "-map_chapters", "0", *clear, node_path(tmp)])
    try:
        verify(src, probe(node_path(tmp)), keep)
    except Exception:
        ocean(f"sudo -n rm -f {shlex.quote(tmp)}", check=False)
        raise
    q, qt = shlex.quote(hpath), shlex.quote(tmp)
    steps = [f"sudo -n chown --reference={q} {qt}", f"sudo -n chmod --reference={q} {qt}"]
    if keep_orig:
        steps.append(f"sudo -n mv -n {q} {shlex.quote(hpath + '.orig')}")
    steps.append(f"sudo -n mv -f {qt} {q}")
    ocean(" && ".join(steps))


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--path", default="", help="only Plex paths containing this substring")
    ap.add_argument("--limit", type=int, help="stop after N files")
    ap.add_argument("--yes", action="store_true", help="actually remux and replace")
    ap.add_argument("--keep-orig", action="store_true",
                    help="keep the original beside the repair as <file>.orig")
    a = ap.parse_args()

    items = affected(a.path)[:a.limit]
    print(f"{len(items)} affected file(s){'' if a.yes else '  [dry run]'}")
    fixed = failed = 0
    for rk, ppath in items:
        hpath = host_path(ppath)
        name = os.path.basename(hpath)
        try:
            src = probe(node_path(hpath))
            keep, dropped = plan(src, node_path(hpath))
            if not dropped:
                print(f"skip   {name}: no byte-identical duplicates")
                continue
            drop = ", ".join(f"{i}={src['streams'][i]['codec_name']} (keeping {f})"
                             for i, f in dropped.items())
            print(f"{'fix   ' if a.yes else 'would '} {name}: drop {drop}")
            if not a.yes:
                continue
            repair(hpath, keep, src, a.keep_orig)
            mc.put("plex", f"/library/metadata/{rk}/analyze")
            fixed += 1
        except Exception as e:  # one bad file must not stop a library-wide run
            failed += 1
            print(f"FAILED {name}: {e}", file=sys.stderr)
    if a.yes:
        print(f"fixed {fixed}, failed {failed}")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
