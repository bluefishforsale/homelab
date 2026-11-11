#!/usr/bin/env python3
"""Pins x265-watch's alarm: run `python3 scripts/test_x265_watch.py` (no pytest).

A loud push is the signal to revisit the movie policy, so it must fire on a
real shift in vetted disc-sourced x265 releases and stay quiet otherwise.
"""
import importlib.util
import os

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("watch", os.path.join(HERE, "x265-watch.py"))
watch = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(watch)

RUNTIME = 155  # Dune (2021), minutes


def rel(title, mbps=8.4, source="bluray", resolution=1080, group="HONE", cfs=("HD Bluray Tier 03", "x265 (HD)")):
    return {"title": title, "size": int(mbps * 1e6 / 8 * RUNTIME * 60), "releaseGroup": group,
            "quality": {"quality": {"source": source, "resolution": resolution}},
            "customFormats": [{"name": n} for n in cfs]}


HONE = rel("Dune.(2021).(1080p.BluRay.x265.SDR.DD.5.1.English.-.Weasley.HONE)")


def test_vetted_disc_x265_near_10mbps_is_a_candidate():
    assert watch.candidate(HONE, RUNTIME)


def test_micro_encode_is_not_a_candidate():
    # GalaxyRG265 and SM737 run 2-2.5 Mbps: the grain-stripping kind the -10000 is for.
    assert not watch.candidate(rel("Dune.2021.1080p.BluRay.DDP5.1.x265.10bit-GalaxyRG265", mbps=2.0), RUNTIME)


def test_x265_from_a_web_source_is_not_a_candidate():
    # Re-encoding a WEB-DL is a second lossy pass; only disc-sourced encodes count.
    assert not watch.candidate(rel("Dune.2021.1080p.WEB-DL.x265.10bit-X", source="webdl"), RUNTIME)


def test_hdr_or_dv_1080p_is_not_a_candidate():
    # TayTO's 1080p DV/HDR10 x265 is Tier 01 but not SDR; Plex tone-maps it on most clients.
    tayto = rel("Dune.Part.One.2021.1080p.UHD.BluRay.DDP7.1.DV.HDR10.x265-TayTO", mbps=27.6,
                group="TayTO", cfs=("DV HDR10", "HD Bluray Tier 01", "x265 (HD)"))
    assert not watch.candidate(tayto, RUNTIME)
    assert not watch.candidate(rel("Dune.2021.1080p.UHD.BluRay.HLG.HDR10.X265-BiTOR", mbps=10), RUNTIME)


def test_x264_is_not_a_candidate():
    assert not watch.candidate(rel("Dune.2021.1080p.BluRay.x264-BHDStudio", group="BHDStudio"), RUNTIME)


def test_only_tiered_candidates_are_observed_per_group():
    untiered = rel("Dune.2021.1080p.BluRay.x265.10bit.Tigole", mbps=9.1, group=None, cfs=("x265 (HD)",))
    seen = watch.observe({"Dune (2021)": (RUNTIME, [HONE, untiered])})
    assert seen == {"HONE": {"tier": 3, "films": ["Dune (2021)"]}}


def test_first_sighting_of_a_vetted_group_is_loud():
    assert watch.changes({}, {"HONE": {"tier": 3, "films": ["Dune (2021)"]}})


def test_same_group_same_tier_is_quiet():
    seen = {"HONE": {"tier": 3, "films": ["Dune (2021)"]}}
    assert watch.changes(seen, seen) == []


def test_group_moving_up_a_tier_is_loud():
    assert watch.changes({"HONE": {"tier": 3, "films": ["A"]}}, {"HONE": {"tier": 2, "films": ["A"]}})


def test_top_tier_group_reaching_two_films_is_loud():
    before = {"X": {"tier": 1, "films": ["A"]}}
    after = {"X": {"tier": 1, "films": ["A", "B"]}}
    assert watch.changes(before, after)


def test_low_tier_group_spreading_is_quiet():
    # Tier 03 on more films is drift, not the wind changing.
    assert watch.changes({"X": {"tier": 3, "films": ["A"]}}, {"X": {"tier": 3, "films": ["A", "B"]}}) == []


def test_group_vanishing_from_a_search_is_quiet():
    # Indexer results come and go week to week; absence is not a signal.
    assert watch.changes({"HONE": {"tier": 3, "films": ["A"]}}, {}) == []


def test_trash_definition_change_is_loud():
    assert watch.trash_changes({"x265-hd": "a"}, {"x265-hd": "b"})


def test_trash_definition_unfetched_is_quiet():
    # A GitHub blip must not read as TRaSH dropping the rule.
    assert watch.trash_changes({"x265-hd": "a"}, {}) == []


def test_digest_goes_once_a_month():
    assert watch.digest_due({"digest_month": "2026-10"}, "2026-11")
    assert not watch.digest_due({"digest_month": "2026-11"}, "2026-11")


if __name__ == "__main__":
    tests = [(n, f) for n, f in sorted(globals().items()) if n.startswith("test_")]
    for name, fn in tests:
        fn()
        print("ok", name)
    print(f"{len(tests)} passed")
