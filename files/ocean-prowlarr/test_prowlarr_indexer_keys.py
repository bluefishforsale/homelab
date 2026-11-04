#!/usr/bin/env python3
"""Pins which indexers get a key pushed: run `python3 files/ocean-prowlarr/test_prowlarr_indexer_keys.py` (no pytest).

Vault slugs are matched to Prowlarr's display names, and a wrong match pushes
one indexer's key into another, which Prowlarr then disables on the failed test.
"""
import importlib.util
import json
import os

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("keys", os.path.join(HERE, "prowlarr-indexer-keys.py"))
keys = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(keys)

STORED = {1: ("Miatrix", "aaa"), 4: ("NzbPlanet", "old"), 5: ("NZBs.in", "ccc")}


def test_only_the_indexer_whose_key_differs_is_pushed():
    assert keys.stale(STORED, {"miatrix": "aaa", "nzbplanet": "new"}) == {4: "new"}


def test_punctuated_names_match_their_slug():
    # NZBs.in is stored with a dot; the vault slug cannot carry one.
    assert keys.stale(STORED, {"nzbsin": "changed"}) == {5: "changed"}


def test_indexers_absent_from_the_vault_are_left_alone():
    assert keys.stale(STORED, {}) == {}


def test_vault_slug_with_no_indexer_fails_instead_of_being_skipped():
    # A typo'd slug would otherwise leave the real indexer on its stale key forever.
    try:
        keys.stale(STORED, {"nzbplant": "new"})
    except SystemExit as e:
        assert "nzbplant" in str(e)
    else:
        raise AssertionError("expected SystemExit")


def test_two_indexers_with_one_slug_fail_instead_of_one_being_dropped():
    # "NZB.su" and "Nzbsu" both slug to nzbsu; one would never get its key.
    try:
        keys.stale({1: ("NZB.su", "a"), 2: ("Nzbsu", "b")}, {"nzbsu": "c"})
    except SystemExit as e:
        assert "share a slug" in str(e)
    else:
        raise AssertionError("expected SystemExit")


def test_validation_error_never_echoes_the_submitted_key():
    raw = json.dumps([{"propertyName": "ApiKey", "errorMessage": "Invalid API Key",
                       "attemptedValue": "s3cret-key"}]).encode()
    msg = keys.error_messages(raw)
    assert "Invalid API Key" in msg and "s3cret-key" not in msg


def test_error_message_quoting_the_request_url_has_its_key_redacted():
    # Prowlarr's test failure can quote the indexer URL it called.
    msg = keys.redact("Unable to connect: https://api.example/api?t=caps&apikey=s3cret-key&o=xml", ())
    assert "s3cret-key" not in msg and "apikey=***" in msg


def test_known_keys_are_redacted_wherever_they_appear():
    assert "s3cret" not in keys.redact("Invalid key s3cret for indexer", ("s3cret",))


if __name__ == "__main__":
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok", name)
