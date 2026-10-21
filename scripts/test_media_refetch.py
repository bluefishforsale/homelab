#!/usr/bin/env python3
"""Pins media-refetch's choices: run `python3 scripts/test_media_refetch.py` (no pytest).

These decide what gets deleted: a pack replaces a whole season, and an
acceptable release is grabbed and the old files go right after.
"""
import importlib.util
import os
import urllib.error

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("refetch", os.path.join(HERE, "media-refetch.py"))
refetch = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(refetch)

PACK_TERM = r"Contains these ignored terms: /\bS\d+\b(?!E\d+\b)/i"


def test_half_bad_season_comes_back_as_a_pack():
    assert refetch.season_mode(5, 10, airing=False) == "pack"


def test_mostly_good_season_searches_only_its_bad_episodes():
    # A pack would replace the 8 good episodes too.
    assert refetch.season_mode(2, 10, airing=False) == "episodes"


def test_airing_season_never_comes_back_as_a_pack():
    # A pack for a season still airing is usually missing episodes.
    assert refetch.season_mode(10, 10, airing=True) == "episodes"


def test_pack_turned_down_only_by_the_no_packs_profile_is_acceptable():
    rel = {"downloadAllowed": True, "rejections": ["Existing file meets cutoff: Bluray-1080p", PACK_TERM]}
    assert refetch.acceptable(rel, {PACK_TERM})


def test_pack_from_an_ignored_group_is_not_acceptable():
    # The Sopranos S06 packs carry the no-packs term; one also from a bad dual-audio group must stay out.
    rel = {"downloadAllowed": True,
           "rejections": [PACK_TERM, r"Contains these ignored terms: /\b(-alfaHD|-BAT)\b/i"]}
    assert not refetch.acceptable(rel, {PACK_TERM})


def test_no_packs_term_is_not_waved_through_for_a_single_episode():
    rel = {"downloadAllowed": True, "rejections": [PACK_TERM]}
    assert not refetch.acceptable(rel)


def test_no_packs_profile_is_found_whatever_its_case():
    # Sonarr's API returns the profile as "no season packs"; an exact match found
    # nothing, so every pack looked unacceptable and every season fell to episodes.
    profiles = [{"name": "no season packs", "ignored": [r"/\bS\d+\b(?!E\d+\b)/i"]}]
    assert refetch.pack_rejection(profiles) == {PACK_TERM}


def test_release_sonarr_cannot_download_is_not_acceptable():
    assert not refetch.acceptable({"downloadAllowed": False, "rejections": []})


def test_refetched_file_queued_but_not_bumped_is_bumped():
    # Behind the library backlog, a refetched file outlasts FLOW_TIMEOUT and its batch stalls.
    assert refetch.needs_bump({"TranscodeDecisionMaker": "Queued", "bumped": False})


def test_bumped_file_is_not_bumped_again():
    # The timer ticks every 5 minutes; a fresh stamp each tick would reorder the bumped files.
    assert not refetch.needs_bump({"TranscodeDecisionMaker": "Queued", "bumped": 1792459953286})


def test_file_tdarr_has_finished_or_failed_is_not_bumped():
    # Bumping does not requeue; an errored file waits out FLOW_TIMEOUT and goes stuck.
    for state in ("Transcode success", "Transcode error", "Not required"):
        assert not refetch.needs_bump({"TranscodeDecisionMaker": state, "bumped": False})


def test_file_tdarr_has_not_scanned_yet_is_left_for_the_next_tick():
    # Radarr imports before tdarr's watcher records the file; cruddb getById answers None.
    assert not refetch.needs_bump(None)


def test_download_still_in_the_queue_after_grab_timeout_is_not_given_up():
    # The 2026-10-20 batch queued 1.5 TB behind NZBGet's 18:00-02:00 pause; 37 items
    # went stuck while still downloading and stuck never restores the added date.
    grabbed = refetch.now() - refetch.GRAB_TIMEOUT * 2
    assert not refetch.gave_up(grabbed, downloading=True)


class FakeArr:
    """Stands in for media_clients: nothing queued, the grab POST refused."""

    def __init__(self, files):
        self.files, self.deleted = files, []

    def get(self, app, path, **kw):
        if path.endswith("/queue/details"):
            return []
        return {"hasFile": True, "movieFile": {"id": self.files[0]}}

    def delete(self, app, path):
        self.deleted.append(path)

    def post(self, app, path, data):
        raise urllib.error.HTTPError(path, 409, "Conflict", {}, None)


def test_refused_movie_grab_keeps_the_old_file():
    # DAN DA DAN S02 lost all 12 episodes on 2026-10-21: files deleted, then Sonarr
    # answered the grab with 409 and nothing downloaded.
    fake = FakeArr([7])
    refetch.mc, refetch.best_release = fake, lambda mid: {"title": "x", "size": 1e9, "indexer": "i",
                                                         "guid": "g", "indexerId": 1}
    item = {"movie_id": 1, "state": "planned", "title": "t", "log": []}
    try:
        refetch.step(item, yes=True)
    except urllib.error.HTTPError:
        pass
    assert fake.deleted == [] and item["state"] == "planned"


def test_refused_season_grab_keeps_the_old_episodes():
    fake = FakeArr([])
    refetch.mc = fake
    refetch.best_tv_release = lambda item: {"title": "x", "size": 1e9, "indexer": "i", "guid": "g", "indexerId": 1}
    refetch.tv_files = lambda sid, eps: {e: {"id": 100 + e} for e in eps}
    item = {"series_id": 1, "season": 2, "episode_ids": [1, 2], "pack": True, "state": "planned",
            "title": "t", "log": []}
    try:
        refetch.step_tv(item, True, {})
    except urllib.error.HTTPError:
        pass
    assert fake.deleted == [] and item["state"] == "planned"


def test_earlier_bump_is_transcoded_first():
    # tdarr takes the highest bumped value first. A timestamp made the newest import
    # jump the line: 12 movies bumped 2026-10-20 sat behind 279 TV files bumped after.
    t = refetch.dt.datetime(2026, 10, 20, 20, 0)
    assert refetch.bump_value(t) > refetch.bump_value(t + refetch.dt.timedelta(minutes=1))


if __name__ == "__main__":
    for name, fn in sorted(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok  ", name)
