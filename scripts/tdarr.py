#!/usr/bin/env python3
"""Drive Tdarr from the shell, so flow edits and test runs never need the web UI.

The web UI is the only sanctioned way to edit a flow, which makes flows
un-reviewable and un-revertable. This exports them as JSON you can diff and
commit, pushes them back, and drives the queue.

  tdarr.py status                       # queue/error/success counts + live workers
  tdarr.py errors [-n 20]               # failing files WITH the ffmpeg reason
  tdarr.py survey                       # codec/bitrate/audio audit of the library
  tdarr.py libraries                    # libraries and the flow each one uses
  tdarr.py flows                        # flow ids and names
  tdarr.py flow-get <id> [-o f.json]    # export a flow
  tdarr.py flow-put <f.json>            # push a flow back (create or update)
  tdarr.py library-flow <lib> <flowId>  # point a library at a flow
  tdarr.py requeue --errors             # re-queue everything that errored
  tdarr.py requeue --path <substr>      # re-queue files whose path contains substr
  tdarr.py probe <path>                 # ffprobe a file through the node container
  tdarr.py nodes                        # pause state, schedule, worker limits, live workers
  tdarr.py pause|resume [node]          # stop/start a node taking new work (all nodes if omitted)
  tdarr.py workers <type> <n> [node]    # set a worker limit, e.g. transcodegpu 1
  tdarr.py schedule on|off [node]       # hourly worker schedule (it overwrites limits on the hour)
  tdarr.py queue [-n 20]                # next files in the order the queue will take them
  tdarr.py drive [--newer-than-days 7]  # paused node: fill free slots with recent arrivals only

Env: TDARR_URL (default http://ocean.home:8265).
"""
import argparse
import collections
import json
import os
import subprocess
import sys
import time
import urllib.request

URL = os.environ.get("TDARR_URL", "http://ocean.home:8265").rstrip("/")
# Tables Tdarr keys its queues by. Named here because "table3" at a call site is
# unreadable and the numbering is not guessable.
TABLES = {"queued": "table1", "success": "table2", "error": "table3",
          "healthy": "table5", "health_error": "table6"}
# Every table holding real files, for whole-library sweeps.
FILE_TABLES = ("table1", "table2", "table3")


def api(path, payload=None, timeout=120):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(f"{URL}{path}", data=data,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        body = r.read()
    # The mutating endpoints (update-node, alter-worker-limit) answer a bare "OK".
    return json.loads(body) if body.startswith((b"{", b"[")) else body.decode()


def crud(collection, mode, **kw):
    return api("/api/v2/cruddb", {"data": {"collection": collection, "mode": mode, **kw}})


def rows(table, page_size=500, limit=None):
    """Yield file records from a status table, paging until exhausted."""
    start = 0
    while True:
        d = api("/api/v2/client/status-tables",
                {"data": {"start": start, "pageSize": page_size, "filters": [],
                          "sorts": [], "opts": {"table": table}}})
        got = d.get("array", [])
        if not got:
            return
        for r in got:
            yield r
            if limit is not None:
                limit -= 1
                if limit <= 0:
                    return
        start += len(got)
        if start >= d.get("totalCount", 0):
            return


def node_exec(argv):
    """Run a command inside the tdarr-node container (it has ffmpeg/ffprobe)."""
    return subprocess.run(["ssh", "-o", "BatchMode=yes", "ocean.home",
                           "sudo -n docker exec tdarr-node " + " ".join(argv)],
                          capture_output=True, text=True, timeout=120)


def nodes(name=None):
    got = [n for n in api("/api/v2/get-nodes", timeout=20).values()
           if name in (None, n["_id"], n["nodeName"])]
    if not got:
        sys.exit(f"no such node: {name}")
    return got


def update_node(n, **updates):
    api("/api/v2/update-node", {"data": {"nodeID": n["_id"], "nodeUpdates": updates}})


def lang_of(stream):
    return ((stream.get("tags") or {}).get("language") or "und").lower()


def library_of(path):
    if path.startswith("/media/tv"):
        return "tv"
    if path.startswith("/media/movies"):
        return "movies"
    return "other"


# --------------------------------------------------------------------------
def cmd_status(a):
    s = crud("StatisticsJSONDB", "getById", docID="statistics")
    print(f"files {s.get('totalFileCount')}   transcodes {s.get('totalTranscodeCount')}"
          f"   saved {s.get('sizeDiff', 0)/1024:.2f} TB")
    for name, t in TABLES.items():
        print(f"  {name:14} {s.get(t + 'Count', 0)}")
    try:
        nodes = api("/api/v2/get-nodes", timeout=20)
    except Exception as e:                      # node list is best-effort
        print(f"  (workers unavailable: {e})")
        return
    for n in (nodes or {}).values():
        for w in (n.get("workers") or {}).values():
            print(f"  worker {w.get('workerType')} {w.get('status')} "
                  f"{w.get('percentage')}% {(w.get('file') or '')[-60:]}")


def cmd_errors(a):
    """Errors plus the reason, which the file record does NOT carry: it lives in
    the job report on the server, so we go and read it."""
    for r in rows(TABLES["error"], limit=a.limit):
        f = r.get("file", "")
        print(f"\n{f}")
        print(f"  codec={r.get('video_codec_name')} res={r.get('video_resolution')} "
              f"size={(r.get('file_size') or 0)/1024:.2f}GB bitrate={r.get('bit_rate')}")
        if a.reason:
            fp = r.get("footprintId")
            out = subprocess.run(
                ["ssh", "-o", "BatchMode=yes", "ocean.home",
                 "sudo -n docker exec tdarr sh -c "
                 f"'ls -t /app/server/Tdarr/DB2/JobReports/{fp}/*transcode*.txt 2>/dev/null | head -1'"],
                capture_output=True, text=True, timeout=60).stdout.strip()
            if out:
                grep = subprocess.run(
                    ["ssh", "-o", "BatchMode=yes", "ocean.home",
                     "sudo -n docker exec tdarr sh -c "
                     f"'grep -iE \"Invalid|failed|Conversion|error code\" \"{out}\" | tail -3'"],
                    capture_output=True, text=True, timeout=60).stdout.strip()
                for line in grep.splitlines():
                    print(f"    {line[:200]}")


def cmd_survey(a):
    codec = collections.Counter()
    size = collections.Counter()
    over = collections.Counter()
    aud = collections.Counter()
    subs = collections.Counter()
    caps = {"tv": 8_000_000, "movies": 16_000_000, "other": 16_000_000}
    total = 0
    for t in FILE_TABLES:
        for r in rows(t):
            total += 1
            c = (r.get("video_codec_name") or "?").lower()
            codec[c] += 1
            size[c] += (r.get("file_size") or 0)
            lib = library_of(r.get("file", ""))
            if (r.get("bit_rate") or 0) > caps[lib]:
                over[lib] += 1
            streams = ((r.get("ffProbeData") or {}).get("streams")) or []
            au = [s for s in streams if s.get("codec_type") == "audio"]
            sb = [s for s in streams if s.get("codec_type") == "subtitle"]
            subs["has" if sb else "none"] += 1
            if not au:
                aud["no audio"] += 1
            else:
                langs = [lang_of(s) for s in au]
                eng = [i for i, L in enumerate(langs) if L.startswith("en")]
                aud["english first" if eng and eng[0] == 0 else
                    "no english" if not eng else "english NOT first"] += 1
    print(f"files {total}")
    print("\ncodec                count       size")
    for c, n in codec.most_common():
        print(f"  {c:16} {n:6}   {size[c]/1024/1024:7.2f} TB")
    print("\nover target bitrate (tv>8Mb, movies>16Mb)")
    for k, v in over.most_common():
        print(f"  {k:16} {v:6}")
    print("\naudio ordering")
    for k, v in aud.most_common():
        print(f"  {k:16} {v:6}")
    print("\nsubtitles")
    for k, v in subs.most_common():
        print(f"  {k:16} {v:6}")


def cmd_libraries(a):
    for x in crud("LibrarySettingsJSONDB", "getAll"):
        print(f"{x.get('_id')}  name={x.get('name')!r:16} folder={x.get('folder')!r:22} "
              f"flow={x.get('flowId')}")


def cmd_flows(a):
    for x in crud("FlowsJSONDB", "getAll"):
        print(f"{x.get('_id'):16} {x.get('name')!r:44} plugins={len(x.get('flowPlugins') or [])}")


def cmd_flow_get(a):
    got = [x for x in crud("FlowsJSONDB", "getAll") if x.get("_id") == a.flow_id]
    if not got:
        sys.exit(f"no such flow: {a.flow_id}")
    text = json.dumps(got[0], indent=2)
    if a.out:
        with open(a.out, "w") as f:
            f.write(text + "\n")
        print(f"wrote {a.out}")
    else:
        print(text)


def cmd_flow_put(a):
    with open(a.file) as f:
        flow = json.load(f)
    fid = flow.get("_id")
    if not fid:
        sys.exit("flow JSON has no _id")
    existing = {x.get("_id") for x in crud("FlowsJSONDB", "getAll")}
    mode = "update" if fid in existing else "insert"
    crud("FlowsJSONDB", mode, docID=fid, obj=flow)
    print(f"{mode}d flow {fid} ({flow.get('name')!r})")


def cmd_library_flow(a):
    libs = crud("LibrarySettingsJSONDB", "getAll")
    match = [x for x in libs if a.library in (x.get("_id"), x.get("name"))]
    if not match:
        sys.exit(f"no such library: {a.library} (have: "
                 f"{[x.get('name') for x in libs]})")
    lib = match[0]
    lib["flowId"] = a.flow_id
    crud("LibrarySettingsJSONDB", "update", docID=lib["_id"], obj=lib)
    print(f"library {lib.get('name')!r} -> flow {a.flow_id}")


def cmd_requeue(a):
    if not a.errors and not a.path:
        sys.exit("give --errors or --path")
    targets = []
    tables = [TABLES["error"]] if a.errors else list(FILE_TABLES)
    for t in tables:
        for r in rows(t):
            f = r.get("file", "")
            if a.path and a.path not in f:
                continue
            targets.append(r)
    if not targets:
        print("nothing matched")
        return
    print(f"{len(targets)} file(s) to re-queue")
    if not a.yes:
        for r in targets[:10]:
            print(f"  {r.get('file')}")
        if len(targets) > 10:
            print(f"  ... and {len(targets)-10} more")
        print("re-run with --yes to actually queue")
        return
    # NOTE: /api/v2/update-file-tdarr is gone in tdarr 3.00 (404); the file
    # record is updated through cruddb like every other collection.
    for r in targets:
        crud("FileJSONDB", "update", docID=r["_id"],
             obj={"TranscodeDecisionMaker": "Queued", "lastTranscodeDate": 0})
    print(f"re-queued {len(targets)}")


def cmd_probe(a):
    r = node_exec(["ffprobe", "-v", "error", "-show_entries",
                   "stream=index,codec_type,codec_name,channels,channel_layout,"
                   "width,height,bit_rate,pix_fmt,color_space:stream_tags=language,title",
                   "-of", "default=noprint_wrappers=1", f"'{a.path}'"])
    print(r.stdout or r.stderr)


def cmd_nodes(a):
    for n in nodes(a.node):
        lim = {k: v for k, v in n["workerLimits"].items() if k != "_id"}
        print(f"{n['nodeName']} ({n['_id']})  paused={n['nodePaused']}  "
              f"schedule={'on' if n['scheduleEnabled'] else 'off'}  limits={lim}")
        for w in n["workers"].values():
            print(f"  worker {w.get('workerType')} {w.get('percentage')}% "
                  f"{(w.get('file') or '')[-70:]}")


def cmd_pause(a):
    for n in nodes(a.node):
        update_node(n, nodePaused=a.paused)
        print(f"{n['nodeName']} {'paused' if a.paused else 'resumed'}")


def cmd_workers(a):
    for n in nodes(a.node):
        # NOTE: with the schedule on, the server rewrites workerLimits from the
        # hourly table on the hour, so a manual limit silently reverts.
        if n["scheduleEnabled"]:
            sys.exit(f"{n['nodeName']}: schedule is on and would reset this on the hour; "
                     f"run `tdarr.py schedule off` first")
        delta = a.count - n["workerLimits"][a.type]
        for _ in range(abs(delta)):
            api("/api/v2/alter-worker-limit",
                {"data": {"nodeID": n["_id"], "workerType": a.type,
                          "process": "increase" if delta > 0 else "decrease"}})
        print(f"{n['nodeName']} {a.type} {n['workerLimits'][a.type]} -> {a.count}")


def cmd_schedule(a):
    for n in nodes(a.node):
        update_node(n, scheduleEnabled=a.state == "on")
        print(f"{n['nodeName']} schedule {a.state}")


def cmd_queue(a):
    order = crud("SettingsGlobalJSONDB", "getById", docID="globalsettings").get("queueSortType")
    queued = list(rows(TABLES["queued"]))
    if order in ("sortSizeLargest", "sortSizeSmallest"):
        queued.sort(key=lambda r: r.get("file_size") or 0, reverse=order == "sortSizeLargest")
    else:
        print(f"(queue sort {order!r} not modelled; showing table order)")
    print(f"{len(queued)} queued, sort {order}")
    for r in queued[:a.limit]:
        print(f"  {(r.get('file_size') or 0)/1024:6.1f} GB  {r.get('file')}")


def transcoding(node):
    return [w for w in node["workers"].values() if (w.get("workerType") or "").startswith("transcode")]


def cmd_drive(a):
    """Let a paused node take recent arrivals, leaving the backlog alone.

    With the queue newest first, resuming hands the workers the newest files;
    pausing once they have started stops the node collecting anything after.
    Fills every free GPU slot the node's (scheduled) limit allows, but never
    more slots than there are recent files, or a spare worker would pull an old
    backlog file. A node someone left running is not touched."""
    for n in nodes(a.node):
        if not n["nodePaused"]:
            print(f"{n['nodeName']}: running, not driving it")
            continue
        limit = n["workerLimits"].get("transcodegpu", 0) + n["workerLimits"].get("transcodecpu", 0)
        running = len(transcoding(n))
        if running >= limit:
            print(f"{n['nodeName']}: {running}/{limit} transcoding")
            continue
        now_ms = time.time() * 1000
        recent = sum(1 for r in rows(TABLES["queued"])
                     if (now_ms - (r.get("createdAt") or 0)) / 86_400_000 <= a.newer_than_days)
        if not recent:
            print(f"no queued file is under {a.newer_than_days:g} days old; leaving the backlog paused")
            continue
        want = running + min(limit - running, recent)
        busy = []
        update_node(n, nodePaused=False)
        try:
            deadline = time.time() + 120
            while time.time() < deadline:
                time.sleep(5)
                busy = transcoding(nodes(n["_id"])[0])
                # A worker exists a few seconds before it is given a file; wait
                # for the files so the log says what started.
                if len(busy) >= want and all(w.get("file") for w in busy):
                    break
        finally:
            update_node(n, nodePaused=True)
        print(f"{n['nodeName']}: {len(busy)}/{limit} transcoding "
              f"{[(w.get('file') or '')[-70:] for w in busy]}")


def main():
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)

    sub.add_parser("status").set_defaults(fn=cmd_status)
    sub.add_parser("survey").set_defaults(fn=cmd_survey)
    sub.add_parser("libraries").set_defaults(fn=cmd_libraries)
    sub.add_parser("flows").set_defaults(fn=cmd_flows)

    e = sub.add_parser("errors")
    e.add_argument("-n", "--limit", type=int, default=10)
    e.add_argument("--no-reason", dest="reason", action="store_false",
                   help="skip fetching job reports (much faster)")
    e.set_defaults(fn=cmd_errors, reason=True)

    g = sub.add_parser("flow-get")
    g.add_argument("flow_id")
    g.add_argument("-o", "--out")
    g.set_defaults(fn=cmd_flow_get)

    u = sub.add_parser("flow-put")
    u.add_argument("file")
    u.set_defaults(fn=cmd_flow_put)

    lf = sub.add_parser("library-flow")
    lf.add_argument("library")
    lf.add_argument("flow_id")
    lf.set_defaults(fn=cmd_library_flow)

    rq = sub.add_parser("requeue")
    rq.add_argument("--errors", action="store_true")
    rq.add_argument("--path")
    rq.add_argument("--yes", action="store_true")
    rq.set_defaults(fn=cmd_requeue)

    pr = sub.add_parser("probe")
    pr.add_argument("path")
    pr.set_defaults(fn=cmd_probe)

    nd = sub.add_parser("nodes")
    nd.add_argument("node", nargs="?")
    nd.set_defaults(fn=cmd_nodes)

    for name, paused in (("pause", True), ("resume", False)):
        ps = sub.add_parser(name)
        ps.add_argument("node", nargs="?")
        ps.set_defaults(fn=cmd_pause, paused=paused)

    wk = sub.add_parser("workers")
    wk.add_argument("type", choices=("transcodegpu", "transcodecpu", "healthcheckgpu",
                                     "healthcheckcpu"))
    wk.add_argument("count", type=int)
    wk.add_argument("node", nargs="?")
    wk.set_defaults(fn=cmd_workers)

    sc = sub.add_parser("schedule")
    sc.add_argument("state", choices=("on", "off"))
    sc.add_argument("node", nargs="?")
    sc.set_defaults(fn=cmd_schedule)

    dr = sub.add_parser("drive")
    dr.add_argument("--newer-than-days", type=float, default=7)
    dr.add_argument("node", nargs="?")
    dr.set_defaults(fn=cmd_drive)

    q = sub.add_parser("queue")
    q.add_argument("-n", "--limit", type=int, default=20)
    q.set_defaults(fn=cmd_queue)

    a = p.parse_args()
    a.fn(a)


if __name__ == "__main__":
    main()
