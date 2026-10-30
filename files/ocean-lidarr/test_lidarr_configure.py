#!/usr/bin/env python3
"""Pins the quality-profile shape: run `python3 files/ocean-lidarr/test_lidarr_configure.py` (no pytest).

Lidarr's profile schema nests qualities in groups (Lossless, Trash Quality) and
rejects a profile that lists a quality twice, so the flattening is where a bad
PUT would come from.
"""
import importlib.util
import os

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("configure", os.path.join(HERE, "lidarr-configure.py"))
configure = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(configure)


def q(i, name):
    return {"quality": {"id": i, "name": name}, "items": [], "allowed": False}


SCHEMA = [
    q(0, "Unknown"),
    {"name": "Lossless", "id": 1000, "allowed": True,
     "items": [q(6, "FLAC"), q(7, "ALAC")]},
    q(1, "MP3-192"), q(3, "MP3-256"), q(2, "MP3-VBR-V0"), q(4, "MP3-320"),
]


def test_grouped_qualities_are_flattened_so_each_appears_once():
    names = [i["quality"]["name"] for i in configure.profile_items(SCHEMA)]
    assert sorted(names) == sorted(["Unknown", "FLAC", "ALAC", "MP3-192", "MP3-256", "MP3-VBR-V0", "MP3-320"])


def test_mp3_320_is_most_preferred_and_lossless_is_not_allowed():
    items = configure.profile_items(SCHEMA)
    allowed = [i["quality"]["name"] for i in items if i["allowed"]]
    assert allowed == ["MP3-256", "MP3-VBR-V0", "MP3-320"]
    assert items[-1]["quality"]["name"] == "MP3-320"


def test_profile_built_here_reads_back_as_converged():
    items = configure.profile_items(SCHEMA)
    assert configure.profile_matches({"items": items, "cutoff": 4, "upgradeAllowed": True})


def test_allowed_lossless_group_is_not_converged():
    # A stock profile with the Lossless group allowed must be rewritten, even
    # when its flat MP3 items happen to match.
    items = configure.profile_items(SCHEMA)
    items.append({"name": "Lossless", "allowed": True, "items": [q(6, "FLAC")]})
    assert not configure.profile_matches({"items": items, "cutoff": 4, "upgradeAllowed": True})


def test_missing_quality_name_fails_loudly_instead_of_dropping_it():
    try:
        configure.profile_items([q(4, "MP3-320")])
    except SystemExit as e:
        assert "MP3-256" in str(e)
    else:
        raise AssertionError("expected SystemExit")


if __name__ == "__main__":
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok", name)
