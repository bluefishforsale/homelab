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
# No test reaches a live host: a path lookup finds nothing and keeps the path it had.
refetch.ssh = lambda cmd, stdin=None: ""
# Some tests stub this out for the rest of the run; keep the real one.
BEST_RELEASE = refetch.best_release

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


BEST_TV_RELEASE = refetch.best_tv_release
BEST_RELEASE = refetch.best_release

EXISTING = "Existing file on disk has a equal or higher Custom Format score: 98"


def fake_movie_search(year, titles):
    class Fake:
        def get(self, app, path, **kw):
            if path == "/api/v3/release":
                return [{"title": t, "downloadAllowed": True, "rejections": [EXISTING], "releaseWeight": w}
                        for w, t in enumerate(titles)]
            if path == "/api/v3/blocklist/movie":
                return []
            return {"year": year}
    return Fake()


def test_release_of_another_years_film_is_not_grabbed():
    # Radarr maps a same-titled release onto the movie: The Island of Dr. Moreau
    # (1996) was replaced by the 1977 film on 2026-10-25, its old file deleted at grab.
    refetch.mc = fake_movie_search(1996, ["The.Island.Of.Dr.Moreau.1977.1080p.BluRay.REMUX.AVC.FLAC.2.0-EPSiLON",
                                          "The.Island.of.Dr.Moreau.1996.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-EPSiLON"])
    assert "1996" in BEST_RELEASE(1)["title"]


def test_release_a_year_off_the_movie_is_the_same_film():
    # Festival year against release year: Nine Days (2021) comes out as Nine.Days.2020.
    refetch.mc = fake_movie_search(2021, ["Nine.Days.2020.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-TRiToN"])
    assert BEST_RELEASE(1)


def test_extras_disc_is_never_the_season():
    # Star Trek: Voyager S06 and S07 lost all 52 episodes on 2026-11-03: an
    # EXTRAS.DVDRip pack won the pick, the old files went at grab, and the
    # featurettes imported nothing.
    class Fake:
        def get(self, app, path, **kw):
            if path == "/api/v3/releaseprofile":
                return []
            if path == "/api/v3/series/1":
                return {"title": "Star Trek: Voyager"}
            return [{"title": t, "seriesTitle": "Star Trek Voyager", "size": 9e9, "fullSeason": True, "downloadAllowed": True,
                     "rejections": [EXISTING], "releaseWeight": w}
                    for w, t in enumerate(["Star.Trek.Voyager.S06.EXTRAS.DVDRip.x264-MARS",
                                           "Star.Trek.Voyager.S06.NTSC.DVD.DD5.1.MPEG-2.REMUX-S0NNY"])]
    refetch.mc = Fake()
    item = {"pack": True, "series_id": 1, "season": 6, "episode_ids": [1]}
    assert "EXTRAS" not in BEST_TV_RELEASE(item)["title"]


def test_pack_of_another_show_sonarr_matched_by_id_is_not_grabbed():
    # Steven Universe S01 grabbed Universe.2021.S01 on 2026-11-06: the indexer
    # tagged the nature series with Steven Universe's TVDB id, Sonarr matched it by
    # id with no rejection, and the season's 49 old files went at grab.
    class Fake:
        def get(self, app, path, **kw):
            if path == "/api/v3/releaseprofile":
                return []
            if path == "/api/v3/series/1":
                return {"title": "Steven Universe", "alternateTitles": [{"title": "Steven Universe (2014)"}]}
            return [{"title": t, "seriesTitle": st, "size": 9e9, "fullSeason": True, "downloadAllowed": True,
                     "rejections": [EXISTING], "releaseWeight": w}
                    for w, (t, st) in enumerate([("Universe.2021.S01.1080p.BluRay.FLAC2.0.x264-NTb", "Universe 2021"),
                                                 ("Steven.Universe.S01.1080p.BluRay.x265-iVy", "Steven Universe")])]
    refetch.mc = Fake()
    item = {"pack": True, "series_id": 1, "season": 1, "episode_ids": list(range(49))}
    assert BEST_TV_RELEASE(item)["title"].startswith("Steven.Universe")


def test_anime_pack_named_for_its_season_is_the_same_show():
    # DAN DA DAN S02's pack came as "[DB] Dandadan 2nd Season" (2026-10-24).
    series = {"title": "DAN DA DAN", "alternateTitles": [{"title": "Dandadan"}]}
    assert refetch.same_show({"seriesTitle": "Dandadan 2nd Season"}, series)


def test_show_named_for_a_year_does_not_take_another_years_show():
    # Stripping years would leave 1883 and 1923 both empty, and so the same show.
    assert not refetch.same_show({"seriesTitle": "1923"}, {"title": "1883"})
    assert refetch.same_show({"seriesTitle": "1883"}, {"title": "1883"})


def test_ampersand_and_and_name_the_same_show():
    assert refetch.same_show({"seriesTitle": "Law and Order"}, {"title": "Law & Order"})


def test_pack_too_small_to_hold_the_season_is_not_grabbed():
    # Voyager S06 then picked the same 0.4 GB disc without EXTRAS in its name:
    # about 15 MB for each of 26 episodes.
    class Fake:
        def get(self, app, path, **kw):
            if path == "/api/v3/releaseprofile":
                return []
            if path == "/api/v3/series/1":
                return {"title": "Star Trek: Voyager"}
            return [{"title": t, "seriesTitle": "Star Trek Voyager", "size": s, "fullSeason": True, "downloadAllowed": True, "rejections": [EXISTING],
                     "releaseWeight": w}
                    for w, (t, s) in enumerate([("Star.Trek.Voyager.S06.iNTERNAL.DVDRip.x264-MARS", 0.4e9),
                                                ("Star.Trek.Voyager.S06.DVDRip.x264-OTHER", 9e9)])]
    refetch.mc = Fake()
    item = {"pack": True, "series_id": 1, "season": 6, "episode_ids": list(range(26))}
    assert BEST_TV_RELEASE(item)["title"].endswith("-OTHER")


def test_tv_pack_takes_sonarrs_pick_even_when_it_is_x265():
    # The library now wants TV as ~3.5 Mbps 10-bit HEVC, and Sonarr's "x265
    # 10-bit SDR" format ranks a pack like iVy's first. Refetch passing over
    # HEVC (#1356) would grab a remux instead and spend an NVENC encode to land
    # back at the same size with a second generation of loss.
    class Fake:
        def get(self, app, path, **kw):
            if path == "/api/v3/releaseprofile":
                return []
            if path == "/api/v3/series/1":
                return {"title": "Charmed"}
            return [{"title": t, "seriesTitle": "Charmed", "size": 20e9, "fullSeason": True, "downloadAllowed": True,
                     "rejections": [EXISTING], "releaseWeight": w}
                    for w, t in enumerate(["Charmed.1998.S04.1080p.BluRay.10bit.EAC3.2.0.x265-iVy",
                                           "Charmed.S04.1080p.BluRay.REMUX.AVC.FLAC.2.0-EPSiLON"])]
    refetch.mc = Fake()
    item = {"pack": True, "series_id": 1, "season": 4, "episode_ids": list(range(22))}
    assert BEST_TV_RELEASE(item)["title"].endswith("x265-iVy")


def test_hevc_release_is_still_taken_when_it_is_the_only_one():
    refetch.mc = fake_movie_search(2004, ["Film.2004.1080p.BluRay.x265-iVy"])
    assert BEST_RELEASE(1)


def test_anime_season_search_gets_time_for_every_episode():
    # Sonarr runs an anime season search as the pack query plus one search per
    # episode. DAN DA DAN S02 (12 episodes) took 656s, past a flat 600s, every tick.
    timeouts = []

    class Fake:
        def get(self, svc, path, timeout=None, **params):
            timeouts.append(timeout)
            return []

    refetch.mc = Fake()
    BEST_TV_RELEASE({"pack": True, "series_id": 1, "season": 2, "episode_ids": list(range(12))})
    assert timeouts[0] > 656, timeouts


def test_download_sonarr_cannot_import_is_blocklisted_not_waited_on():
    # DAN DA DAN S02 (2026-10-24): Sonarr took "[Moozzi2] Dan Da Dan S2-08" for a
    # pack, but it held one episode. Finished and stuck at importPending, it read
    # as still downloading, so the item would have waited forever (South Park
    # S13E06 had since 10-21).
    rows = [{"id": 500 + e, "episodeId": e, "downloadId": "D", "title": "[Moozzi2] Dan Da Dan S2-08",
             "status": "completed", "trackedDownloadState": "importPending", "trackedDownloadStatus": "warning"}
            for e in (1, 2)]
    deleted = []

    class Fake:
        def get(self, app, path, **kw):
            return rows if path.endswith("/queue/details") else []

        def delete(self, app, path, **params):
            deleted.append((path, params))

    refetch.mc = Fake()
    refetch.tv_files = lambda sid, eps: {}
    item = {"series_id": 1, "season": 2, "episode_ids": [1, 2], "pack": True, "state": "grabbed",
            "grabbed_at": refetch.now().isoformat(), "title": "t", "log": []}
    refetch.step_tv(item, True, {})
    # Without skipRedownload Sonarr searches too: on 2026-10-24 its own grab of the
    # same [DB] pack raced ours, and the next one queued single episodes beside our pack.
    assert deleted == [("/api/v3/queue/501", {"removeFromClient": "true", "blocklist": "true",
                                              "skipRedownload": "true"})], deleted
    assert item["state"] == "planned"


def test_movie_download_radarr_cannot_import_is_blocklisted_not_waited_on():
    # The TV path waited forever on a finished, import-blocked download (#866);
    # Radarr's queue reports the same shape.
    deleted = []

    class Fake:
        def get(self, app, path, **kw):
            if path.endswith("/queue/details"):
                return [{"id": 900, "downloadId": "D", "title": "x", "status": "completed",
                         "trackedDownloadState": "importPending", "trackedDownloadStatus": "warning"}]
            return {"hasFile": False} if "/movie/" in path else []

        def delete(self, app, path, **params):
            deleted.append((path, params))

    refetch.mc = Fake()
    item = {"movie_id": 1, "state": "grabbed", "grabbed_at": refetch.now().isoformat(), "title": "t", "log": []}
    refetch.step(item, yes=True)
    assert deleted == [("/api/v3/queue/900", {"removeFromClient": "true", "blocklist": "true",
                                              "skipRedownload": "true"})], deleted
    assert item["state"] == "planned"


def test_season_just_marked_failed_waits_for_sonarrs_own_redownload():
    # The Simpsons S11 and S12 (2026-10-30): marking a KONTRAST pack failed for
    # missing subtitles made Sonarr grab DSNP episodes while we grabbed an iVy
    # pack in the same tick. DSNP imported first, and 43 iVy rows sat blocked.
    searched = []
    refetch.mc = FakeArr([])
    refetch.best_tv_release = lambda item: searched.append(item) or None
    item = {"series_id": 1, "season": 2, "episode_ids": [1, 2], "pack": True, "state": "planned",
            "failed_at": refetch.now().isoformat(), "title": "t", "log": []}
    assert not refetch.step_tv(item, True, {})
    assert searched == [] and item["state"] == "planned"


def test_movie_just_marked_failed_waits_for_radarrs_own_redownload():
    searched = []
    refetch.mc = FakeArr([])
    refetch.best_release = lambda mid: searched.append(mid) or None
    item = {"movie_id": 1, "state": "planned", "failed_at": refetch.now().isoformat(), "title": "t", "log": []}
    assert not refetch.step(item, yes=True)
    assert searched == [] and item["state"] == "planned"


def test_season_marked_failed_long_ago_searches_again():
    refetch.mc = FakeArr([])
    refetch.best_tv_release = lambda item: None
    item = {"series_id": 1, "season": 2, "episode_ids": [1, 2], "pack": False, "state": "planned",
            "failed_at": (refetch.now() - refetch.REDOWNLOAD_GRACE * 2).isoformat(), "title": "t", "log": []}
    refetch.step_tv(item, True, {})
    assert item["state"] == "no-release"


def test_pack_sonarr_imported_only_part_of_searches_the_rest():
    # Attack on Titan S04 (2026-10-30): the SiQ pack imported 28 of 30 episodes and
    # Sonarr held the release as import-blocked for the other two. Still queued, it
    # read as downloading, so the item would have waited in grabbed forever.
    grabbed = refetch.now() - refetch.dt.timedelta(hours=1)
    rows = [{"id": 700 + e, "episodeId": e, "downloadId": "D", "title": "SiQ", "status": "completed",
             "trackedDownloadState": "importPending", "trackedDownloadStatus": "warning"} for e in (1, 2, 3)]
    deleted, posted = [], []

    class Fake:
        def get(self, app, path, **kw):
            return rows if path.endswith("/queue/details") else []

        def delete(self, app, path, **params):
            deleted.append((path, params))

        def post(self, app, path, data):
            posted.append(data)

    refetch.mc = Fake()
    refetch.tv_files = lambda sid, eps: {e: {"id": 100 + e, "dateAdded": refetch.now().isoformat(),
                                             "path": f"/tv/{e}.mkv"} for e in eps if e != 3}
    refetch.subs = lambda path: ["eng"]
    item = {"series_id": 1, "season": 4, "episode_ids": [1, 2, 3], "pack": True, "state": "grabbed",
            "grabbed_at": grabbed.isoformat(), "attempts": 0, "title": "t", "log": []}
    refetch.step_tv(item, True, {})
    # Not blocklisted: the release was good for the episodes it held.
    assert deleted == [("/api/v3/queue/701", {"removeFromClient": "true", "blocklist": "false"})], deleted
    assert posted == [{"name": "EpisodeSearch", "episodeIds": [3]}], posted
    assert item["state"] == "imported"


def test_earlier_bump_is_transcoded_first():
    # tdarr takes the highest bumped value first. A timestamp made the newest import
    # jump the line: 12 movies bumped 2026-10-20 sat behind 279 TV files bumped after.
    t = refetch.dt.datetime(2026, 10, 20, 20, 0)
    assert refetch.bump_value(t) > refetch.bump_value(t + refetch.dt.timedelta(minutes=1))


def test_movie_tdarr_is_still_transcoding_past_flow_timeout_keeps_waiting():
    # Friday (1995), bumped onto harbor's CPU workers at about 2% an hour, would have
    # gone stuck at 3 days with its transcode still running (#1087).
    f = "/media/movies/Friday (1995)/Friday.mkv"

    class Fake:
        def get(self, app, path, **kw):
            return {"n": {"workers": {"w": {"workerType": "transcodecpu", "file": f}}}}

    saved = refetch.probe, refetch.conformance.reasons, refetch.bump
    refetch.mc = Fake()
    refetch.probe = lambda file: {}
    refetch.conformance.reasons = lambda probed: ["video-not-hevc"]
    refetch.bump = lambda item, files, yes: False
    long_ago = (refetch.now() - refetch.FLOW_TIMEOUT * 2).isoformat()
    item = {"movie_id": 1, "state": "imported", "file": f, "title": "t", "log": [],
            "grabbed_at": long_ago, "imported_at": long_ago}
    try:
        refetch.step(item, yes=True)
    finally:
        refetch.probe, refetch.conformance.reasons, refetch.bump = saved
    assert item["state"] == "imported", item["log"]


def test_season_tdarr_is_still_transcoding_past_flow_timeout_keeps_waiting():
    files = ["/media/tv/Dollhouse/Season 1/E01.mkv", "/media/tv/Dollhouse/Season 1/E02.mkv"]

    class Fake:
        def get(self, app, path, **kw):
            return {"n": {"workers": {"w": {"workerType": "transcodegpu", "file": files[1]}}}}

    saved = refetch.probe, refetch.conformance.reasons, refetch.bump
    refetch.mc = Fake()
    refetch.probe = lambda file: file
    refetch.conformance.reasons = lambda probed: ["video-not-hevc"] if probed == files[1] else []
    refetch.bump = lambda item, files, yes: False
    item = {"series_id": 1, "season": 1, "episode_ids": [1, 2], "state": "imported", "files": files,
            "title": "t", "log": [], "imported_at": (refetch.now() - refetch.FLOW_TIMEOUT * 2).isoformat()}
    try:
        refetch.step_tv(item, True, {})
    finally:
        refetch.probe, refetch.conformance.reasons, refetch.bump = saved
    assert item["state"] == "imported", item["log"]


def test_season_tdarr_remuxed_to_mkv_is_probed_at_its_new_path():
    # DS9 S02 (2026-11-04): the flow turned 26 imported .mp4s into .mkvs and the
    # ledger kept probing the gone .mp4s, an ssh error every tick.
    mp4 = "/media/tv/DS9/Season 2/E01.mp4"
    mkv = "/media/tv/DS9/Season 2/E01.mkv"
    probed, posted = [], []

    class Fake:
        def get(self, app, path, **kw):
            return {}

        def post(self, app, path, data):
            posted.append(data)

    saved = refetch.ssh, refetch.probe, refetch.conformance.reasons, refetch.bump
    refetch.mc = Fake()
    refetch.ssh = lambda cmd, stdin=None: mkv + "\n" if mkv in cmd else ""
    refetch.probe = lambda file: probed.append(file)
    refetch.conformance.reasons = lambda probed: ["video-not-hevc"]
    refetch.bump = lambda item, files, yes: False
    item = {"series_id": 1, "season": 2, "episode_ids": [1], "state": "imported", "files": [mp4],
            "title": "t", "log": [], "imported_at": refetch.now().isoformat()}
    try:
        refetch.step_tv(item, True, {})
    finally:
        refetch.ssh, refetch.probe, refetch.conformance.reasons, refetch.bump = saved
    assert probed == [mkv] and item["files"] == [mkv], (probed, item["files"])
    assert {"name": "RescanSeries", "seriesId": 1} in posted, posted


def test_movie_tdarr_remuxed_to_mkv_is_probed_at_its_new_path():
    # Tenet (2020-11-10): the audio pass turned the imported BHDStudio .mp4 into
    # an .mkv before Radarr rescanned, so the grabbed check probed the gone .mp4
    # and failed with an ssh error every tick.
    mp4 = "/media/movies/Tenet (2020)/Tenet.mp4"
    mkv = "/media/movies/Tenet (2020)/Tenet.mkv"
    probed = []

    class Fake:
        def get(self, app, path, **kw):
            return {"hasFile": True, "path": "/movies/Tenet (2020)",
                    "movieFile": {"id": 5, "relativePath": "Tenet.mp4",
                                  "dateAdded": refetch.now().isoformat()}}

    saved = refetch.ssh, refetch.subs
    refetch.mc = Fake()
    refetch.ssh = lambda cmd, stdin=None: mkv + "\n" if mkv in cmd else ""
    refetch.subs = lambda file: probed.append(file) or ["eng"]
    item = {"movie_id": 1, "state": "grabbed", "attempts": 0, "title": "t", "log": [],
            "grabbed_at": (refetch.now() - refetch.dt.timedelta(hours=1)).isoformat()}
    try:
        refetch.step(item, yes=True)
    finally:
        refetch.ssh, refetch.subs = saved
    assert probed == [mkv] and item["file"] == mkv, (probed, item.get("file"))
    assert item["path"] == mkv.removeprefix("/media") and item["state"] == "imported"


def test_movie_search_gets_the_same_time_as_a_season_search():
    # Gladiator II, Django Unchained, Crimes of Grindelwald (2026-11-10): Radarr's
    # interactive search for a popular film ran past the 120s default, so they
    # "timed out" every tick and were never grabbed.
    seen = []

    class Fake:
        def get(self, app, path, timeout=120, **kw):
            seen.append((path, timeout))
            return [] if path.endswith(("/release", "/blocklist/movie")) else {"year": 2024}

    refetch.mc = Fake()
    BEST_RELEASE(1)
    assert ("/api/v3/release", refetch.SEARCH_TIMEOUT) in seen, seen


def test_hdr_source_encoded_to_untagged_sdr_is_refetched():
    # Squid Game S01E02: tdarr's encode dropped the PQ tags, so ffprobe sees a
    # plain 10-bit file and it plays washed out on an SDR screen. Only Sonarr's
    # media info, read before the encode, still says HDR10.
    path = "/media/tv/Squid Game/Season 1/Squid Game - S01E02 - Hell WEBDL-1080p.mkv"
    row = {"file": path, "library": "tv", "bucket": "ok", "reasons": []}
    [out] = refetch.with_stripped_hdr([row], {path})
    assert out["bucket"] == "refetch" and "hdr-stripped" in out["reasons"], out


def test_arr_hdr_paths_are_the_tdarr_paths_of_files_recorded_as_hdr():
    movies = [{"path": "/movies/Ballerina (2025)", "movieFile": {"relativePath": "Ballerina.mkv",
                                                                  "mediaInfo": {"videoDynamicRangeType": "PQ"}}},
              {"path": "/movies/Heat (1995)", "movieFile": {"relativePath": "Heat.mkv",
                                                             "mediaInfo": {"videoDynamicRangeType": ""}}},
              {"path": "/movies/Unreleased (2027)"}]
    episodes = [{"path": "/tv/Ozark/Season 4/Ozark - S04E01.mkv", "mediaInfo": {"videoDynamicRangeType": "HDR10"}},
                {"path": "/tv/Ozark/Season 1/Ozark - S01E01.mkv", "mediaInfo": {}}]
    assert refetch.arr_hdr_paths(movies, episodes) == {
        "/media/movies/Ballerina (2025)/Ballerina.mkv", "/media/tv/Ozark/Season 4/Ozark - S04E01.mkv"}


def test_movie_refetched_into_4k_is_planned_again():
    # Batman (1989) was refetched for subtitles on 11-08 and came back a 72 GB
    # 2160p DV remux. The ledger held it as already refetched, so the 1080p
    # cap never saw it again.
    assert not refetch.held_back(7, ["over-1080p", "hdr"], {7})
    assert not refetch.held_back(7, ["over-bitrate"], {7})


def test_movie_kept_without_subs_is_not_planned_again():
    # A refetch that found no English subtitles keeps the last download; planning
    # it again would loop on the same releases forever.
    assert refetch.held_back(7, ["no-english-subs"], {7})
    assert not refetch.held_back(8, ["no-english-subs"], {7})


def _movie(i, title, rel):
    return {"id": i, "title": title, "year": 2016, "path": f"/movies/{title} (2016)",
            "movieFile": {"relativePath": rel, "size": 2.6e9}}


def test_listed_movie_is_planned_though_it_conforms():
    # Arrival came out of tdarr's 8-bit squeeze with subtitles and the dialogue
    # track, so conformance calls it clean; the banding is only visible on screen.
    m = _movie(1, "Arrival", "Arrival.mkv")
    plex = {"/movies/Arrival (2016)/Arrival.mkv": (500, 1600000000, None)}
    todo = refetch.listed_movies([m], {1}, plex, {500: 2}, {})
    assert [(t["title"], t["reasons"], t["plays"], t["file"]) for t in todo] == [
        ("Arrival (2016)", ["listed"], 2, "/media/movies/Arrival (2016)/Arrival.mkv")]


def test_listed_movie_already_in_flight_is_not_planned_twice():
    m = _movie(1, "Arrival", "Arrival.mkv")
    plex = {"/movies/Arrival (2016)/Arrival.mkv": (500, 1600000000, None)}
    ledger = {"/media/movies/Arrival (2016)/Old.mkv": {"movie_id": 1, "state": "grabbed"}}
    assert refetch.listed_movies([m], {1}, plex, {}, ledger) == []


def test_listed_movie_already_refetched_from_a_list_is_not_planned_again():
    # 2026-11-11: the third 100 from squeezed-first.txt re-planned ~80 films the
    # first two hundred had already finished (Empire of the Sun, 12 Years a
    # Slave), mostly grabbing the very release they already had.
    m = _movie(1, "Arrival", "Arrival.mkv")
    plex = {"/movies/Arrival (2016)/Arrival.mkv": (500, 1600000000, None)}
    ledger = {"/media/movies/Arrival (2016)/Old.mkv": {"movie_id": 1, "state": "done", "reasons": ["listed"]}}
    assert refetch.listed_movies([m], {1}, plex, {}, ledger) == []


def test_listed_movie_refetched_for_another_reason_can_be_listed():
    # A REMUX refetched for its size in October is squeezed and banded now;
    # the list is how it gets its second pass.
    m = _movie(1, "Arrival", "Arrival.mkv")
    plex = {"/movies/Arrival (2016)/Arrival.mkv": (500, 1600000000, None)}
    ledger = {"/media/movies/Arrival (2016)/Old.mkv": {"movie_id": 1, "state": "done", "reasons": ["over-bitrate"]}}
    assert [t["movie_id"] for t in refetch.listed_movies([m], {1}, plex, {}, ledger)] == [1]


def test_listed_movie_missing_from_plex_is_skipped():
    # Without a Plex rating key the added date can't be put back afterwards.
    assert refetch.listed_movies([_movie(1, "Arrival", "Arrival.mkv")], {1}, {}, {}, {}) == []


def test_finished_tv_refetch_can_be_listed_again():
    # Charmed S03 came back as x264-BORDURE and finished, then the 1.2 Mbps flow
    # (#1370) squeezed it before the 10-bit target landed. Listing it must not be
    # blocked by the done ledger entry.
    ledger = {"tv:5:3": {"kind": "tv", "state": "done", "episode_ids": [11, 12]},
              "/media/tv/X/Season 1/a.mkv": {"kind": "tv", "state": "grabbed", "episode_ids": [21]}}
    eps, seasons = refetch.tv_held(ledger, relist=True)
    assert eps == {21} and seasons == set()


def test_tv_refetch_in_flight_or_done_holds_by_default():
    ledger = {"tv:5:3": {"kind": "tv", "state": "done", "episode_ids": [11, 12]}}
    eps, seasons = refetch.tv_held(ledger, relist=False)
    assert eps == {11, 12} and seasons == {"tv:5:3"}


class FakeRadarr:
    """One movie with an old file; records every write."""

    def __init__(self, new=False, queue=(), subs_ok=True):
        old = refetch.now() - refetch.dt.timedelta(days=400)
        self.file = {"id": 9 if new else 7, "relativePath": "new.mkv" if new else "old.mkv",
                     "dateAdded": (refetch.now() if new else old).isoformat()}
        self.queue, self.writes = list(queue), []

    def get(self, app, path, **kw):
        if path.endswith("/queue/details"):
            return self.queue
        if "/history/movie" in path:
            return [{"id": 3, "date": refetch.now().isoformat()}]
        return {"hasFile": True, "path": "/movies/Film (2016)", "movieFile": self.file}

    def delete(self, app, path, **kw):
        self.writes.append(("delete", path))

    def post(self, app, path, data):
        self.writes.append(("post", path, data))


def run_step(item, fake, subs=("eng",)):
    cmds = []
    saved = refetch.ssh, refetch.subs, refetch.mc, refetch.best_release
    refetch.mc, refetch.subs = fake, lambda file: list(subs)
    refetch.best_release = lambda mid: {"title": "x", "size": 1e9, "indexer": "i", "guid": "g", "indexerId": 1}
    refetch.ssh = lambda cmd, stdin=None: cmds.append(cmd) or ""
    try:
        refetch.step(item, yes=True)
    finally:
        refetch.ssh, refetch.subs, refetch.mc, refetch.best_release = saved
    return cmds


HOLD = refetch.HOLD + "/1/old.mkv"
OLD = "/data01/complete/movies/Film (2016)/old.mkv"


def grabbed_item(**kw):
    return {"movie_id": 1, "state": "grabbed", "attempts": 0, "title": "t", "log": [], "held": [[OLD, HOLD]],
            "grabbed_at": (refetch.now() - refetch.dt.timedelta(hours=1)).isoformat(), **kw}


def test_movie_grab_leaves_the_old_file_playable():
    # Free State of Jones (2026-11-10): the old file went at grab time, so the film
    # was gone from Plex for the whole download and both failed attempts.
    fake = FakeRadarr()
    item = {"movie_id": 1, "state": "planned", "title": "t", "log": []}
    cmds = run_step(item, fake)
    assert not [w for w in fake.writes if w[0] == "delete"], fake.writes
    assert item["state"] == "grabbed" and item["held"] == [[OLD, HOLD]]
    assert any("ln" in c and OLD in c and HOLD in c for c in cmds), cmds


def test_movie_import_refused_as_no_upgrade_swaps_the_old_file_out():
    # With the old file still there, Radarr refuses a release that is not an
    # upgrade on paper (same quality, the squeeze does not show in the name).
    q = {"status": "completed", "trackedDownloadStatus": "warning", "downloadId": "D", "outputPath": "/downloads/x",
         "title": "x", "statusMessages": [{"messages": ["Not an upgrade for existing movie file(s)"]}]}
    fake = FakeRadarr(queue=[q])
    item = grabbed_item()
    run_step(item, fake)
    assert ("delete", "/api/v3/moviefile/7") in fake.writes, fake.writes
    assert not [w for w in fake.writes if "/queue/" in w[1]], "must not blocklist the release"
    scan = [w for w in fake.writes if w[0] == "post" and w[1] == "/api/v3/command"]
    assert scan and scan[0][2]["name"] == "DownloadedMoviesScan" and scan[0][2]["downloadClientId"] == "D"
    assert item["state"] == "grabbed"


def test_movie_release_blocklisted_after_the_swap_puts_the_old_file_back():
    # The swap took the old file out, then Radarr still would not import: the
    # film must not sit missing until some later attempt lands.
    q = {"status": "completed", "trackedDownloadStatus": "warning", "downloadId": "D", "outputPath": "/downloads/x",
         "title": "x", "id": 5, "statusMessages": [{"messages": ["Unable to parse file"]}]}
    fake = FakeRadarr(queue=[q])
    fake.get_movie = fake.get
    fake.get = lambda app, path, **kw: ({"hasFile": False, "path": "/movies/Film (2016)"}
                                        if path == "/api/v3/movie/1" else fake.get_movie(app, path, **kw))
    item = grabbed_item()
    cmds = run_step(item, fake)
    assert any("ln" in c and c.index(HOLD) < c.index(OLD) for c in cmds if HOLD in c and OLD in c), cmds
    assert item["state"] == "planned"


def test_movie_new_file_that_passes_drops_the_held_copy():
    item = grabbed_item()
    cmds = run_step(item, FakeRadarr(new=True))
    assert item["state"] == "imported" and "held" not in item
    assert any("rm" in c and HOLD in c for c in cmds), cmds


def test_movie_new_file_without_english_puts_the_old_file_back():
    fake = FakeRadarr(new=True)
    item = grabbed_item()
    cmds = run_step(item, fake, subs=())
    assert ("delete", "/api/v3/moviefile/9") in fake.writes
    assert any("ln" in c and c.index(HOLD) < c.index(OLD) for c in cmds if HOLD in c and OLD in c), cmds
    assert ("post", "/api/v3/command", {"name": "RescanMovie", "movieIds": [1]}) in fake.writes, fake.writes
    assert item["state"] == "planned" and item["held"] == [[OLD, HOLD]]


def test_put_back_recreates_the_movie_folder_as_media():
    # A.I. (2026-11-11): Radarr removed the emptied folder, `sudo mkdir -p` made
    # it again as root, and every later delete Radarr tried there was
    # "Access to the path is denied" (HTTP 500), so the swap looped forever.
    cmds = []
    saved = refetch.ssh, refetch.mc
    refetch.ssh, refetch.mc = (lambda cmd, stdin=None: cmds.append(cmd) or ""), FakeRadarr()
    try:
        refetch.put_back({"movie_id": 1, "held": [[OLD, HOLD]]})
    finally:
        refetch.ssh, refetch.mc = saved
    assert not any("mkdir" in c for c in cmds), cmds
    assert any("install -d -o media -g media" in c for c in cmds), cmds


def test_movie_kept_after_the_last_attempt_drops_the_held_copy():
    item = grabbed_item(attempts=refetch.MAX_ATTEMPTS - 1)
    cmds = run_step(item, FakeRadarr(new=True), subs=())
    assert item["state"] == "imported" and item.get("no_subs") and "held" not in item
    assert any("rm" in c and HOLD in c for c in cmds), cmds


def test_movie_swap_waits_for_radarrs_scan_instead_of_blocklisting_it():
    # Slow West (2026-11-12): the run loop stepped again 7s after the swap, before
    # DownloadedMoviesScan ran, saw no file and the row still blocked, and
    # blocklisted the download it had just asked Radarr to import.
    q = {"status": "completed", "trackedDownloadStatus": "warning", "downloadId": "D", "outputPath": "/downloads/x",
         "title": "x", "id": 5, "statusMessages": [{"messages": ["Not an upgrade for existing movie file(s)"]}]}
    fake = FakeRadarr(queue=[q])
    item = grabbed_item()
    run_step(item, fake)
    fake.get_movie = fake.get
    fake.get = lambda app, path, **kw: ({"hasFile": False, "path": "/movies/Film (2016)"}
                                        if path == "/api/v3/movie/1" else fake.get_movie(app, path, **kw))
    cmds = run_step(item, fake)
    assert not [w for w in fake.writes if "/queue/" in w[1]], fake.writes
    assert not any(OLD in c for c in cmds), cmds
    assert item["state"] == "grabbed"


def test_movie_swap_radarr_still_will_not_import_later_is_blocklisted():
    q = {"status": "completed", "trackedDownloadStatus": "warning", "downloadId": "D", "outputPath": "/downloads/x",
         "title": "x", "id": 5, "statusMessages": [{"messages": ["Unable to parse file"]}]}
    fake = FakeRadarr(queue=[q])
    fake.get_movie = fake.get
    fake.get = lambda app, path, **kw: ({"hasFile": False, "path": "/movies/Film (2016)"}
                                        if path == "/api/v3/movie/1" else fake.get_movie(app, path, **kw))
    item = grabbed_item(swapped_at=(refetch.now() - refetch.IMPORT_GRACE).isoformat())
    run_step(item, fake)
    assert ("delete", "/api/v3/queue/5") in fake.writes, fake.writes
    assert item["state"] == "planned" and "swapped_at" not in item


def test_reposted_copy_of_a_blocklisted_release_is_not_grabbed():
    # Slow West (2026-11-11): BHDStudio was blocklisted for no English subtitle and
    # grabbed again 51 minutes later as another post of the same title. Radarr's
    # blocklist only matches the one post.
    class Fake:
        def get(self, app, path, **kw):
            if path == "/api/v3/release":
                return [{"title": "Film.2016.1080p-BHD", "releaseWeight": 1, "downloadAllowed": True, "rejections": []},
                        {"title": "Film.2016.1080p-OTHER", "releaseWeight": 2, "downloadAllowed": True,
                         "rejections": []}]
            if path == "/api/v3/blocklist/movie":
                return [{"sourceTitle": "Film.2016.1080p-BHD"}]
            return {"year": 2016}

    refetch.mc = Fake()
    assert BEST_RELEASE(1)["title"] == "Film.2016.1080p-OTHER"


if __name__ == "__main__":
    for name, fn in sorted(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok  ", name)
