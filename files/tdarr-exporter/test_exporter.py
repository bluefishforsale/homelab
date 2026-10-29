#!/usr/bin/env python3
"""Pins the job-history math: run `python3 files/tdarr-exporter/test_exporter.py` (no pytest).

Every per-node panel on the Tdarr dashboard is an aggregate of JobsJSONDB, and
the record shape has changed across Tdarr versions (nodeNames only exists from
3.x, node ids rotate when a node re-registers), so the edges live here.
"""
import importlib.util
import os
import sys
import types

HERE = os.path.dirname(os.path.abspath(__file__))
# CI has no prometheus_client, and the functions under test need none of it.
sys.modules["prometheus_client"] = types.SimpleNamespace(start_http_server=None)
sys.modules["prometheus_client.core"] = types.SimpleNamespace(GaugeMetricFamily=None, REGISTRY=None)
_spec = importlib.util.spec_from_file_location("exporter", os.path.join(HERE, "exporter.py"))
exporter = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(exporter)

LIVE = {"0a4XIvK": "harbor.home", "BxMHaIf": "ocean.home"}


def test_job_is_attributed_to_the_live_node_that_ran_it():
    job = {"nodeID": "0a4XIvK", "nodeNames": ["harbor.home", "ocean.home"]}
    assert exporter.node_of(job, LIVE) == "harbor.home"


def test_retired_node_id_falls_back_to_its_only_recorded_name():
    # Node ids rotate when a node re-registers; ocean has had dozens.
    assert exporter.node_of({"nodeID": "8lq5ivu", "nodeNames": ["ocean.home"]}, LIVE) == "ocean.home"


def test_retired_node_id_with_no_single_name_is_unknown_not_dropped():
    # Pre-3.x records carry no nodeNames, and a flow that hopped nodes names
    # both; guessing would credit one node with the other's work.
    assert exporter.node_of({"nodeID": "MLIZTIQul"}, LIVE) == "unknown"
    assert exporter.node_of({"nodeID": "x", "nodeNames": ["harbor.home", "ocean.home"]}, LIVE) == "unknown"



def test_tdarr_status_strings_fold_into_one_outcome_vocabulary():
    # Healthchecks say "Success"/"Error", transcodes prefix "Transcode", and a
    # job still running has an empty status.
    assert [exporter.outcome(s) for s in
            ("Success", "Transcode success", "Error", "Transcode error", "Not required", "")] == \
        ["success", "success", "error", "error", "not_required", "running"]


NOW_MS = 1_793_280_000_000
HOUR_MS = 3_600_000


def job(node="0a4XIvK", type_="transcode", status="Transcode success", end_ago_h=1, duration_s=60,
        start_gb=0.0, end_gb=0.0):
    end = NOW_MS - end_ago_h * HOUR_MS
    return {"nodeID": node, "job": {"type": type_}, "status": status,
            "start": end - duration_s * 1000, "end": end, "duration": duration_s * 1000,
            "fileSizeStartGB": start_gb, "fileSizeEndGB": end_gb}


def summarize(jobs, window_h=24):
    return exporter.summarize(jobs, LIVE, NOW_MS, window_h * 3600)


def test_counts_are_lifetime_per_node_type_and_outcome():
    s = summarize([job(), job(end_ago_h=1000), job(status="Transcode error"),
                   job(node="BxMHaIf", type_="healthcheck", status="Success")])
    assert s["counts"] == {("harbor.home", "transcode", "success"): 2,
                           ("harbor.home", "transcode", "error"): 1,
                           ("ocean.home", "healthcheck", "success"): 1}



def test_durations_cover_only_successful_jobs_that_finished_inside_the_window():
    # An error usually dies in seconds and a running job has no end, so either
    # would drag min/avg toward zero; old jobs would pin max forever.
    s = summarize([job(duration_s=60), job(duration_s=180), job(duration_s=5, status="Transcode error"),
                   job(duration_s=9999, end_ago_h=48),
                   {**job(duration_s=0), "status": "", "end": 0, "duration": 0}])
    assert s["durations"] == {("harbor.home", "transcode"): (60.0, 120.0, 180.0)}



def test_bytes_count_only_transcodes_that_wrote_a_new_file():
    # Tdarr's "GB" is GiB (file_size is MiB, /1024). A healthcheck or a "Not
    # required" job records its input size but no output, which would read as
    # 100% savings.
    gib = 1024 ** 3
    s = summarize([job(start_gb=4.0, end_gb=1.0), job(start_gb=2.0, end_gb=1.5, end_ago_h=1000),
                   job(status="Not required", start_gb=7.0, end_gb=7.0),
                   job(status="Transcode error", start_gb=9.0),
                   job(type_="healthcheck", status="Success", start_gb=16.0)])
    assert s["bytes"] == {"harbor.home": (6.0 * gib, 2.5 * gib)}


def test_successful_transcode_with_no_recorded_output_size_is_left_out_of_bytes():
    # 4 of 6,345 live "Transcode success" records carry fileSizeEndGB 0.
    s = summarize([job(start_gb=4.0, end_gb=1.0), job(start_gb=30.0, end_gb=0.0)])
    assert s["bytes"] == {"harbor.home": (4.0 * 1024 ** 3, 1.0 * 1024 ** 3)}



def test_worker_waiting_for_a_task_is_not_active_even_though_tdarr_says_not_idle():
    # Live get-nodes: a worker between files reports idle=false, startTime=0.
    nodes = {"0a4XIvK": {"nodeName": "harbor.home", "workers": {
        "dark-dodo": {"workerType": "transcodecpu", "idle": False, "startTime": 0, "fps": 0,
                      "percentage": 0.0},
        "red-owl": {"workerType": "transcodegpu", "idle": False, "startTime": 1793277686540,
                    "fps": 33, "percentage": 33.88}}}}
    assert sorted(exporter.workers(nodes)) == [
        ("harbor.home", "transcodecpu", "dark-dodo", False, 0, 0.0),
        ("harbor.home", "transcodegpu", "red-owl", True, 33, 33.88)]



def test_node_version_is_the_version_of_its_most_recent_job():
    # get-nodes carries no version; jobs record the version that ran them, and
    # records come back in no particular order.
    new = job(end_ago_h=1)
    new["job"]["version"] = "3.01.00"
    old = job(end_ago_h=500)
    old["job"]["version"] = "2.35.02"
    assert summarize([new, old])["versions"] == {"harbor.home": "3.01.00"}


if __name__ == "__main__":
    tests = [v for k, v in sorted(globals().items()) if k.startswith("test_")]
    for t in tests:
        t()
        print(f"ok  {t.__name__}")
    print(f"{len(tests)} passed")
