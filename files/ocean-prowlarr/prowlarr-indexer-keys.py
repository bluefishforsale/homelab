#!/usr/bin/env python3
"""Push indexer API keys from the vault into Prowlarr.

Rotating a key at the indexer used to mean editing it in Prowlarr's UI, with
no record anywhere else. The vault (media_services.prowlarr.indexers.<slug>)
is now the source, and Sonarr, Radarr and Lidarr pick the change up for free
because they reach every indexer through Prowlarr's proxy, never directly.

The API reads keys back masked, so the comparison reads Prowlarr's database
(read-only) and only a differing key is PUT through the API. Prowlarr tests
the indexer on save, so a bad key fails the deploy instead of being stored.
"""
import json
import os
import re
import sqlite3
import sys
import urllib.error
import urllib.request


def slug(name):
    return re.sub(r"[^a-z0-9]", "", name.lower())


def stale(stored, wanted):
    """Map indexer id -> wanted key for every indexer whose stored key differs.

    stored: {id: (name, key)} from the database. wanted: {slug: key} from the vault.
    """
    by_slug = {slug(name): (i, key) for i, (name, key) in stored.items()}
    unknown = sorted(set(wanted) - set(by_slug))
    if unknown:
        raise SystemExit(f"vault has keys for indexers Prowlarr does not: {unknown} "
                         f"(known: {sorted(by_slug)})")
    return {by_slug[s][0]: k for s, k in wanted.items() if by_slug[s][1] != k}


def stored_keys(db):
    conn = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    try:
        rows = conn.execute("SELECT Id, Name, Settings FROM Indexers").fetchall()
    finally:
        conn.close()
    return {i: (name, json.loads(s).get("apiKey")) for i, name, s in rows}


def api(base, key, path, method="GET", body=None):
    req = urllib.request.Request(
        base + path,
        method=method,
        data=None if body is None else json.dumps(body).encode(),
        headers={"X-Api-Key": key, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            return json.loads(r.read() or "null")
    except urllib.error.HTTPError as e:
        raise SystemExit(f"{method} {path}: {e.code} {error_messages(e.read())}")


def error_messages(raw):
    """Only the messages: Prowlarr's validation errors echo the submitted key as attemptedValue."""
    try:
        body = json.loads(raw)
    except ValueError:
        return "(unparseable error body)"
    items = body if isinstance(body, list) else [body]
    return "; ".join(str(i.get("errorMessage") or i.get("message")) for i in items if isinstance(i, dict))


def main():
    env = os.environ
    base, key = env["PROWLARR_URL"], env["PROWLARR_API_KEY"]
    wanted = {s: v["api_key"] for s, v in json.loads(env["INDEXER_KEYS"]).items()}
    for i, new in stale(stored_keys(env["PROWLARR_DB"]), wanted).items():
        idx = api(base, key, f"/api/v1/indexer/{i}")
        for f in idx["fields"]:
            if f["name"] == "apiKey":
                f["value"] = new
        api(base, key, f"/api/v1/indexer/{i}", "PUT", idx)
        print(f"changed: {idx['name']} API key")


if __name__ == "__main__":
    sys.exit(main())
