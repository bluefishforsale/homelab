#!/usr/bin/env python3
"""Tdarr exporter.

Tdarr has no metrics endpoint, and the per-node story (who transcoded what, how
fast, how much it saved) only exists in its job history, so this reads the
server's API on every scrape and serves it as Prometheus gauges.

NOTE: a job's start is when the server STAGED it for a node, not when a worker
picked it up, so tdarr_job_duration_seconds includes time spent waiting in the
node's staging slot (hours, for a transcode behind a long queue). Healthchecks
barely wait, so theirs is close to pure processing time. For transcodes use
tdarr_job_processing_seconds_*, which runs from the worker's start.
"""
import collections
import json
import os
import threading
import time
import urllib.request

from prometheus_client import start_http_server
from prometheus_client.core import REGISTRY, GaugeMetricFamily

URL = os.environ.get("TDARR_URL", "http://tdarr:8265").rstrip("/")
PORT = int(os.environ.get("TDARR_EXPORTER_PORT", "9196"))
# min/avg/max job time covers jobs that finished in this window; counts and
# bytes are lifetime.
WINDOW_HOURS = float(os.environ.get("TDARR_DURATION_WINDOW_HOURS", "24"))
TIMEOUT = float(os.environ.get("TDARR_TIMEOUT_SECONDS", "30"))
# Reading the pending queue pages through every file (about a minute for 11k),
# far too slow for a scrape, so a thread refreshes it this often.
QUEUE_REFRESH_SECONDS = float(os.environ.get("TDARR_QUEUE_REFRESH_SECONDS", "900"))
QUEUE_PAGE = 1000
# StatisticsJSONDB keys its queue counts by table number.
TABLE_STATES = {"table1Count": "pending", "table2Count": "success", "table3Count": "error",
                "table5Count": "health_ok", "table6Count": "health_error"}

# Tdarr's "GB" fields are GiB: FileJSONDB file_size is MiB and the UI divides by 1024.
GIB = 1024 ** 3


def node_of(job, live):
    # nodeID is the node that staged the job; nodeNames grows as the job is
    # requeued to another node, so its last entry is the node that ran it.
    names = job.get("nodeNames") or []
    if names:
        return names[-1]
    return live.get(job.get("nodeID"), "unknown")


def outcome(status):
    s = status.lower().removeprefix("transcode ").strip()
    return s.replace(" ", "_") if s else "running"


def workers(nodes):
    """(node, worker_type, worker_id, active, fps, percent) per worker."""
    return [(n["nodeName"], w["workerType"], wid, w.get("startTime", 0) > 0, w.get("fps", 0),
             w.get("percentage", 0.0))
            for n in nodes.values() for wid, w in (n.get("workers") or {}).items()]


def remember_starts(started, nodes, oldest_ms):
    """Record when each running job's worker began it, and forget stale ones."""
    for n in nodes.values():
        for w in (n.get("workers") or {}).values():
            if w.get("startTime", 0) > 0 and (w.get("job") or {}).get("jobId"):
                started.setdefault(w["job"]["jobId"], w["startTime"])
    for jid in [k for k, v in started.items() if v < oldest_ms]:
        del started[jid]


def summarize(jobs, live, now_ms, window_s, started=None):
    """started maps jobId to the startTime a worker reported for it."""
    started = started or {}
    counts = collections.Counter()
    seconds = collections.defaultdict(list)
    processing = collections.defaultdict(lambda: [0.0, 0])
    sizes = collections.defaultdict(lambda: [0.0, 0.0])
    latest = {}
    since_ms = now_ms - window_s * 1000
    for j in jobs:
        node, type_, result = node_of(j, live), j["job"]["type"], outcome(j.get("status", ""))
        counts[(node, type_, result)] += 1
        if "version" in j["job"] and j["start"] >= latest.get(node, (0, ""))[0]:
            latest[node] = (j["start"], j["job"]["version"])
        if result == "success" and j.get("end", 0) >= since_ms:
            seconds[(node, type_)].append(j["duration"] / 1000)
            if j.get("_id") in started:
                processing[(node, type_)][0] += (j["end"] - started[j["_id"]]) / 1000
                processing[(node, type_)][1] += 1
        if result == "success" and type_ == "transcode" and j["fileSizeEndGB"] > 0:
            sizes[node][0] += j["fileSizeStartGB"] * GIB
            sizes[node][1] += j["fileSizeEndGB"] * GIB
    return {"counts": dict(counts),
            "durations": {k: (min(v), sum(v) / len(v), max(v)) for k, v in seconds.items()},
            "processing": {k: tuple(v) for k, v in processing.items()},
            "bytes": {k: tuple(v) for k, v in sizes.items()},
            "versions": {k: v[1] for k, v in latest.items()}}


def queue_by_codec(rows):
    """(files, seconds of video) per video codec among pending files."""
    files, seconds = collections.Counter(), collections.Counter()
    for r in rows:
        codec = r.get("video_codec_name") or "unknown"
        files[codec] += 1
        seconds[codec] += r.get("duration") or 0
    return {c: (files[c], seconds[c]) for c in files}


def api(path, payload=None):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(f"{URL}{path}", data=data,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=TIMEOUT) as r:
        return json.load(r)


def crud(collection, mode, **kw):
    return api("/api/v2/cruddb", {"data": {"collection": collection, "mode": mode, **kw}})


def read_queue():
    rows, start = [], 0
    while True:
        page = api("/api/v2/client/status-tables",
                   {"data": {"start": start, "pageSize": QUEUE_PAGE, "filters": [], "sorts": [],
                             "opts": {"table": "table1"}}})["array"]
        rows += [{"video_codec_name": r.get("video_codec_name"), "duration": r.get("duration")} for r in page]
        if len(page) < QUEUE_PAGE:
            return rows
        start += QUEUE_PAGE


QUEUE = {}


def refresh_queue():
    while True:
        try:
            QUEUE.update(codecs=queue_by_codec(read_queue()), at=time.time())
        except Exception as e:  # keep the last good read; the timestamp shows its age
            print(f"[err] queue: {e}", flush=True)
        time.sleep(QUEUE_REFRESH_SECONDS)


def gauge(name, doc, labels=()):
    return GaugeMetricFamily(name, doc, labels=list(labels))


class TdarrCollector:
    def __init__(self):
        self.started = {}

    def collect(self):
        began = time.time()
        up = gauge("tdarr_up", "1 if every Tdarr API read in this scrape succeeded")
        try:
            status = api("/api/v2/status")
            nodes = api("/api/v2/get-nodes")
            stats = crud("StatisticsJSONDB", "getById", docID="statistics")
            staged = crud("StagedJSONDB", "getAll")
            jobs = crud("JobsJSONDB", "getAll")
        except Exception as e:  # server down or restarting; tdarr_up carries it
            print(f"[err] {e}", flush=True)
            up.add_metric([], 0)
            yield up
            return
        up.add_metric([], 1)
        yield up

        live = {nid: n["nodeName"] for nid, n in nodes.items()}
        now_ms = time.time() * 1000
        remember_starts(self.started, nodes, oldest_ms=now_ms - 2 * WINDOW_HOURS * 3_600_000)
        s = summarize(jobs, live, now_ms, WINDOW_HOURS * 3600, self.started)

        g = gauge("tdarr_server_info", "Tdarr server build", ["version", "engine"])
        g.add_metric([status.get("version", ""), status.get("serverEngine", "")], 1)
        yield g
        g = gauge("tdarr_server_uptime_seconds", "Tdarr server process uptime")
        g.add_metric([], status.get("uptime", 0))
        yield g

        info = gauge("tdarr_node_info", "Registered node; version is that of its newest job",
                     ["node", "node_id", "version", "engine"])
        uptime = gauge("tdarr_node_uptime_seconds", "Node process uptime", ["node"])
        paused = gauge("tdarr_node_paused", "1 if the node is paused (takes no new work)", ["node"])
        limit = gauge("tdarr_node_worker_limit", "Configured worker slots", ["node", "worker_type"])
        for nid, n in nodes.items():
            name = n["nodeName"]
            info.add_metric([name, nid, s["versions"].get(name, ""), n.get("nodeEngine", "")], 1)
            uptime.add_metric([name], ((n.get("resStats") or {}).get("process") or {}).get("uptime", 0))
            paused.add_metric([name], 1 if n.get("nodePaused") else 0)
            for wtype, v in (n.get("workerLimits") or {}).items():
                if wtype != "_id":
                    limit.add_metric([name, wtype], v)
        yield from (info, uptime, paused, limit)

        # Summed per node and type: worker ids are random names that churn.
        active = collections.Counter()
        fps = collections.Counter()
        for node, wtype, _, is_active, w_fps, _ in workers(nodes):
            active[(node, wtype)] += is_active
            fps[(node, wtype)] += w_fps if is_active else 0
        g = gauge("tdarr_workers_active", "Workers currently processing a file", ["node", "worker_type"])
        for k, v in active.items():
            g.add_metric(list(k), v)
        yield g
        g = gauge("tdarr_worker_fps", "Summed ffmpeg/HandBrake frames per second", ["node", "worker_type"])
        for k, v in fps.items():
            g.add_metric(list(k), v)
        yield g

        g = gauge("tdarr_files", "Library files by queue state (staged = waiting in a node slot)",
                  ["state"])
        for key, state in TABLE_STATES.items():
            g.add_metric([state], stats.get(key, 0))
        g.add_metric(["staged"], len(staged))
        yield g
        g = gauge("tdarr_library_files", "Files in all libraries")
        g.add_metric([], stats.get("totalFileCount", 0))
        yield g
        g = gauge("tdarr_space_saved_bytes", "Tdarr's own lifetime total of bytes saved")
        g.add_metric([], stats.get("sizeDiff", 0) * GIB)
        yield g

        g = gauge("tdarr_jobs", "Lifetime jobs by node, type and outcome (job history)",
                  ["node", "type", "outcome"])
        for k, v in s["counts"].items():
            g.add_metric(list(k), v)
        yield g
        g = gauge("tdarr_job_duration_seconds",
                  f"Staged-to-finished time of successful jobs in the last {WINDOW_HOURS:g}h",
                  ["node", "type", "stat"])
        for (node, type_), stats3 in s["durations"].items():
            for stat, v in zip(("min", "avg", "max"), stats3):
                g.add_metric([node, type_, stat], v)
        yield g
        g_sum = gauge("tdarr_job_processing_seconds_sum",
                      f"Worker-start-to-finished time summed over successful jobs in the last {WINDOW_HOURS:g}h",
                      ["node", "type"])
        g_count = gauge("tdarr_job_processing_count",
                        f"Successful jobs in the last {WINDOW_HOURS:g}h whose worker start this exporter saw",
                        ["node", "type"])
        for k, (total, n) in s["processing"].items():
            g_sum.add_metric(list(k), total)
            g_count.add_metric(list(k), n)
        yield from (g_sum, g_count)

        if "codecs" in QUEUE:
            g_files = gauge("tdarr_queue_files", "Pending (queued) files by video codec", ["codec"])
            g_secs = gauge("tdarr_queue_video_seconds", "Video length of pending files by video codec", ["codec"])
            for codec, (n, secs) in QUEUE["codecs"].items():
                g_files.add_metric([codec], n)
                g_secs.add_metric([codec], secs)
            g_at = gauge("tdarr_queue_read_timestamp_seconds", "When the pending queue was last read")
            g_at.add_metric([], QUEUE["at"])
            yield from (g_files, g_secs, g_at)

        g_in = gauge("tdarr_transcode_input_bytes", "Lifetime input size of successful transcodes", ["node"])
        g_out = gauge("tdarr_transcode_output_bytes", "Lifetime output size of successful transcodes", ["node"])
        for node, (b_in, b_out) in s["bytes"].items():
            g_in.add_metric([node], b_in)
            g_out.add_metric([node], b_out)
        yield from (g_in, g_out)

        g = gauge("tdarr_exporter_collect_seconds", "Time this scrape spent reading Tdarr")
        g.add_metric([], time.time() - began)
        yield g


def main():
    threading.Thread(target=refresh_queue, daemon=True).start()
    REGISTRY.register(TdarrCollector())
    start_http_server(PORT)
    print(f"tdarr-exporter on :{PORT}, tdarr={URL}", flush=True)
    while True:
        time.sleep(3600)


if __name__ == "__main__":
    main()
