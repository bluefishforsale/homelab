#!/usr/bin/env python3
"""Pins nzbget-prune.targets(): run `python3 files/nzbget/test_nzbget_prune.py` (no pytest).

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


def test_failed_job_files_pruned_after_7_days():
    # Gladiator II remux, FAILURE/PAR, 34 GiB held in intermediate. The history
    # entry stays (Sonarr/Radarr already reacted); only the files go.
    entries = [("intermediate", "Gladiator.II.2024.1080p.BluRay.REMUX.#84080", NOW - 8 * DAY)]
    got = prune.targets(entries, queue_ids=set(), history={84080: "FAILURE/PAR"}, now=NOW)
    assert got == [("intermediate", "Gladiator.II.2024.1080p.BluRay.REMUX.#84080", "failed")], got


def test_failed_job_younger_than_7_days_is_kept():
    # The week is the window for a manual "post-process again" from nzbget.
    entries = [("intermediate", "The.Madison.S01E06.1080p.BluRay.#84110", NOW - 1 * DAY)]
    got = prune.targets(entries, queue_ids=set(), history={84110: "FAILURE/UNPACK"}, now=NOW)
    assert got == [], got


def test_incoming_older_than_30_days_is_pruned():
    # 470 finished downloads Sonarr/Radarr never imported, back to 2024-07.
    # Past 30 days nothing is going to import them, so they go unchecked.
    entries = [("incoming/movies", "Dog.Man.2025.1080p.WEB.h264-ETHEL", NOW - 31 * DAY)]
    got = prune.targets(entries, queue_ids=set(), history={}, now=NOW)
    assert got == [("incoming/movies", "Dog.Man.2025.1080p.WEB.h264-ETHEL", "stale-incoming")], got


def test_incoming_younger_than_30_days_is_kept():
    # A download Sonarr hasn't imported yet (a blocked import waiting on a manual
    # match) has the month to get picked up.
    entries = [("incoming/tv", "One.Piece.S23E25.720p.WEB-DL.AAC2.0.H.264-SubsPlease", NOW - 10 * DAY)]
    got = prune.targets(entries, queue_ids=set(), history={}, now=NOW)
    assert got == [], got


def test_nzbget_unreachable_prunes_nothing():
    # With no queue to check against, every intermediate dir looks orphaned,
    # including the live download. A failed read must delete nothing at all.
    def down():
        raise OSError("connection refused")
    entries = [("intermediate", "Black.Box.2026.1080p.Remux.#84131", NOW - 40 * DAY),
               ("incoming/movies", "Dog.Man.2025.1080p.WEB.h264-ETHEL", NOW - 31 * DAY)]
    got = prune.plan(down, entries, now=NOW)
    assert got == [], got


def test_name_with_slash_or_leading_dot_is_never_a_target():
    # Every target becomes rm -rf <area>/<name>. A name that is not exactly one
    # path component ("..", "../services", "a/b", "") could reach outside the area.
    old = NOW - 400 * DAY
    entries = [("incoming/movies", n, old) for n in ("..", ".", "../services", "a/b", ".hidden", "")]
    entries += [("intermediate", "../x.#81234", old), ("intermediate", ".x.#81234", old)]
    got = prune.targets(entries, queue_ids=set(), history={}, now=NOW)
    assert got == [], got


def test_failed_job_pruned_at_any_age_under_disk_pressure():
    # Spartacus S03 remux, FAILURE/PAR, 130 GiB, a few hours old on 2026-10-30.
    # Two of those and the scratch SSD is past 85%; the retry window loses.
    entries = [("intermediate", "Spartacus.S03.PROPER.BluRay.1080p.REMUX.#85786", NOW - 2 * 3600)]
    got = prune.targets(entries, queue_ids=set(), history={85786: "FAILURE/PAR"}, now=NOW, pressure=True)
    assert got == [("intermediate", "Spartacus.S03.PROPER.BluRay.1080p.REMUX.#85786", "failed")], got


def test_queued_job_is_never_pruned_under_disk_pressure():
    # Pressure shortens retention; it never touches the live download.
    entries = [("intermediate", "The.Simpsons.S13.1080p.BluRay.REMUX.#85796", NOW - 3 * DAY)]
    got = prune.targets(entries, queue_ids={85796}, history={}, now=NOW, pressure=True)
    assert got == [], got


def test_orphan_younger_than_an_hour_is_kept_under_disk_pressure():
    # A job leaving the queue between the listgroups and history reads looks
    # orphaned for a moment; the hour covers that race.
    entries = [("intermediate", "Hackers.1995.Remastered.REMUX.#85801", NOW - 600)]
    got = prune.targets(entries, queue_ids=set(), history={}, now=NOW, pressure=True)
    assert got == [], got


def test_stale_tmp_older_than_a_day_is_pruned():
    # DirectWrite leaves tmp empty in normal running; anything a day old is
    # from a crash or a killed container.
    entries = [("tmp", "85786.out.tmp", NOW - 2 * DAY), ("tmp", "85805.out.tmp", NOW - 3600)]
    got = prune.targets(entries, queue_ids=set(), history={}, now=NOW)
    assert got == [("tmp", "85786.out.tmp", "stale-tmp")], got


def test_guard_pauses_at_85_percent():
    assert prune.guard_action(0.85, guard_paused=False) == "pause"


def test_guard_resumes_only_downloads_it_paused():
    # A pause from the web UI is the user's; the guard never undoes it.
    assert prune.guard_action(0.40, guard_paused=False) is None
    assert prune.guard_action(0.40, guard_paused=True) == "resume"


def test_guard_holds_pause_between_75_and_85_percent():
    # Without the gap, finishing one unpack flips it back on and the next
    # download pushes it over again, every five minutes.
    assert prune.guard_action(0.80, guard_paused=True) is None


if __name__ == "__main__":
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok", name)
