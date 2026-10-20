#!/usr/bin/env python3
"""Pins nzbget-prune.targets(): run `python3 files/ocean-nzbget/test_nzbget_prune.py` (no pytest).

A wrong target is an rm -rf on data01, so every rule that keeps something is
pinned as hard as every rule that deletes.
"""
import importlib.util
import os

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("prune", os.path.join(HERE, "nzbget-prune.py"))
prune = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(prune)

DAY = 86400
NOW = 1_800_000_000


def test_orphan_intermediate_older_than_a_day_is_pruned():
    # 95 dirs, 560 GiB on 2026-10-19: history expires after KeepHistory=30 days
    # and nzbget never deletes a failed job's intermediate files with it.
    entries = [("intermediate", "Some.Movie.2024.1080p.#81234", NOW - 2 * DAY)]
    got = prune.targets(entries, queue_ids=set(), history={}, now=NOW)
    assert got == [("intermediate", "Some.Movie.2024.1080p.#81234", "orphan")], got


def test_intermediate_of_queued_job_is_never_pruned():
    # A paused or slow job can sit in the queue for weeks; its dir is the live download.
    entries = [("intermediate", "Black.Box.2026.1080p.Remux.#84131", NOW - 40 * DAY)]
    got = prune.targets(entries, queue_ids={84131}, history={}, now=NOW)
    assert got == [], got


if __name__ == "__main__":
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok", name)
