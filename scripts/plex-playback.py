#!/usr/bin/env python3
"""What actually plays on Plex, and what it costs — from Tautulli history.

Exists so "which audio format should live on disk" is answered from measurement
instead of from folklore about what devices support. Encoding decisions are
expensive to undo (22 TB of re-encoding), so they should be driven by what the
real clients did, not by a spec sheet.

  plex-playback.py summary            # direct play vs transcode, headline
  plex-playback.py clients            # platform / product / player breakdown
  plex-playback.py audio              # source audio codec+channels -> decision
  plex-playback.py video              # source video codec+resolution -> decision
  plex-playback.py why                # transcode reasons, ranked
  plex-playback.py languages          # audio/subtitle languages actually played
  plex-playback.py users              # who watches, and how much they transcode

  -n/--limit   history rows to pull            (default 1000)
  -s/--sample  rows to fetch stream detail for (default 200; 0 = all)

`summary`, `clients` and `users` read history only, which is one fast call.
`audio`, `video` and `why` need per-session detail, which is one call each, so
they work on a sample. Raise --sample when you want more confidence.

Env: TAUTULLI_URL (default http://ocean.home:8905), TAUTULLI_APIKEY. Without a
key it reads one off ocean over ssh, the same way media_clients.py does.
"""
import argparse
import collections
import json
import os
import re
import subprocess
import sys
import urllib.parse
import urllib.request

URL = os.environ.get("TAUTULLI_URL", "http://ocean.home:8905").rstrip("/")
CFG = "/data01/services/tautulli/config/config.ini"
UNKNOWN = "?"


def api_key():
    k = os.environ.get("TAUTULLI_APIKEY")
    if k:
        return k
    out = subprocess.run(
        ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "ocean.home",
         f"sudo -n grep -oP 'api_key\\s*=\\s*\\K\\w+' {CFG} | head -1"],
        capture_output=True, text=True, timeout=45)
    k = out.stdout.strip()
    if not k:
        sys.exit("no Tautulli API key: set TAUTULLI_APIKEY, or allow ssh to ocean.home")
    return k


KEY = None


def call(cmd, **params):
    q = urllib.parse.urlencode({"apikey": KEY, "cmd": cmd, **params})
    with urllib.request.urlopen(f"{URL}/api/v2?{q}", timeout=60) as f:
        return json.load(f)["response"]["data"]


def history(limit):
    d = call("get_history", length=limit)
    return d["data"] if isinstance(d, dict) else d


def stream_rows(rows, sample):
    """Per-session stream detail. One API call each, so this is the slow path."""
    take = rows if sample == 0 else rows[:sample]
    for r in take:
        rid = r.get("id") or r.get("row_id")
        if rid is None:
            continue
        try:
            yield r, call("get_stream_data", row_id=rid)
        except Exception:
            continue


def table(title, counter, width=34):
    print(f"\n-- {title} --")
    total = sum(counter.values()) or 1
    for k, v in counter.most_common():
        label = k if isinstance(k, str) else "  ".join(str(x) for x in k)
        print(f"   {label[:width]:{width}} {v:5}  {100*v/total:5.1f}%")


# --------------------------------------------------------------------------
def cmd_summary(a):
    rows = history(a.limit)
    print(f"sessions: {len(rows)}")
    table("transcode decision", collections.Counter(r.get("transcode_decision") or UNKNOWN
                                                    for r in rows))
    table("media type", collections.Counter(r.get("media_type") or UNKNOWN for r in rows))


def cmd_clients(a):
    rows = history(a.limit)
    print(f"sessions: {len(rows)}")
    for field in ("platform", "product", "player"):
        table(field, collections.Counter(r.get(field) or UNKNOWN for r in rows))


def cmd_users(a):
    rows = history(a.limit)
    per = collections.Counter(r.get("friendly_name") or r.get("user") or UNKNOWN for r in rows)
    tc = collections.Counter((r.get("friendly_name") or r.get("user") or UNKNOWN)
                             for r in rows if r.get("transcode_decision") == "transcode")
    print(f"sessions: {len(rows)}\n")
    print(f"   {'user':24} {'plays':>6} {'transcoded':>11}")
    for u, n in per.most_common(20):
        print(f"   {u[:24]:24} {n:6} {tc.get(u,0):11}")


# NOTE: Tautulli has no `original_*` fields. The SOURCE is `audio_codec` /
# `video_codec`; `stream_*` is what was actually delivered to the client. Reading
# a non-existent `original_audio_codec` silently yields "?" for every row, which
# looks like missing data rather than a wrong field name.
def _decision_matrix(a, kind):
    rows = history(a.limit)
    seen = collections.Counter()
    n = 0
    for _, d in stream_rows(rows, a.sample):
        n += 1
        if kind == "audio":
            codec = (d.get("audio_codec") or UNKNOWN).lower()
            extra = str(d.get("audio_channels") or UNKNOWN) + "ch"
            dec = (d.get("audio_decision") or UNKNOWN).lower()
        else:
            codec = (d.get("video_codec") or UNKNOWN).lower()
            extra = str(d.get("video_full_resolution") or UNKNOWN)
            dec = (d.get("video_decision") or UNKNOWN).lower()
        seen[(codec, extra, dec)] += 1
    print(f"sampled sessions: {n}")
    print(f"\n-- source {kind} -> decision --")
    for (c, e, dec), v in seen.most_common(24):
        print(f"   {c:10} {e:>7}  {dec:14} {v:5}")
    # Direct-play rate per codec is the number that decides an encoding target.
    per = collections.defaultdict(lambda: [0, 0])
    for (c, _e, dec), v in seen.items():
        per[c][1] += v
        if dec in ("direct play", "copy"):
            per[c][0] += v
    print(f"\n-- direct-play rate by {kind} codec --")
    for c, (ok, tot) in sorted(per.items(), key=lambda kv: -kv[1][1]):
        print(f"   {c:10} {ok:4}/{tot:<4} {100*ok/tot:5.1f}%")


def cmd_audio(a):
    _decision_matrix(a, "audio")


def cmd_video(a):
    _decision_matrix(a, "video")


def cmd_why(a):
    rows = history(a.limit)
    reasons = collections.Counter()
    players = collections.Counter()
    n = 0
    for r, d in stream_rows(rows, a.sample):
        n += 1
        hit = False
        if (d.get("audio_decision") or "").lower() == "transcode":
            reasons[f"audio: {(d.get('audio_codec') or UNKNOWN).lower()} "
                    f"{d.get('audio_channels') or UNKNOWN}ch -> "
                    f"{(d.get('stream_audio_codec') or UNKNOWN).lower()}"] += 1
            hit = True
        if (d.get("video_decision") or "").lower() == "transcode":
            reasons[f"video: {(d.get('video_codec') or UNKNOWN).lower()} "
                    f"{d.get('video_full_resolution') or UNKNOWN}"] += 1
            hit = True
        if (d.get("stream_subtitle_decision") or "").lower() in ("transcode", "burn"):
            reasons[f"subtitle burn: {(d.get('subtitle_codec') or UNKNOWN).lower()}"] += 1
            hit = True
        if hit:
            players[r.get("player") or UNKNOWN] += 1
    print(f"sampled sessions: {n}")
    table("transcode causes", reasons, width=44)
    table("players transcoding", players)


def cmd_languages(a):
    """Which audio/subtitle languages actually get played, and whether subtitle
    use forces a burn-in (which forces a full video transcode)."""
    rows = history(a.limit)
    audio = collections.Counter()
    subs = collections.Counter()
    sub_dec = collections.Counter()
    n = 0
    for _, d in stream_rows(rows, a.sample):
        n += 1
        audio[(d.get("audio_language") or "none").lower()] += 1
        s = (d.get("subtitle_language") or "").lower()
        subs[s if s else "no subtitles used"] += 1
        if s:
            sub_dec[(d.get("stream_subtitle_decision") or UNKNOWN).lower()] += 1
    print(f"sampled sessions: {n}")
    table("audio language played", audio)
    table("subtitle language used", subs)
    if sub_dec:
        table("subtitle handling (burn = full video transcode)", sub_dec, width=40)


def main():
    global KEY
    # Shared as a parent so the flags work on either side of the subcommand;
    # `audio -s 500` is what anyone actually types.
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("-n", "--limit", type=int, default=1000)
    common.add_argument("-s", "--sample", type=int, default=200)

    p = argparse.ArgumentParser(description=__doc__, parents=[common],
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)
    for name, fn in (("summary", cmd_summary), ("clients", cmd_clients), ("users", cmd_users),
                     ("audio", cmd_audio), ("video", cmd_video), ("why", cmd_why),
                     ("languages", cmd_languages)):
        sub.add_parser(name, parents=[common]).set_defaults(fn=fn)
    a = p.parse_args()
    KEY = api_key()
    a.fn(a)


if __name__ == "__main__":
    main()
