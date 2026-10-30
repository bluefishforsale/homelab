#!/usr/bin/env python3
"""Converge Lidarr's settings to what the repo declares, through its API.

Lidarr keeps download clients, profiles and indexers in its database, not in
config.xml, so the playbook runs this after every start. Idempotent: it reads
first and writes only what differs, printing one line per change. The playbook
reads "changed" from stdout.

Indexers are not configured here. Prowlarr owns them for every *arr app, so
this registers Lidarr as a Prowlarr application and removes any indexer that
Prowlarr did not put there.
"""
import json
import os
import sys
import urllib.request

PROFILE = "MP3 320"
# Highest preference last, which is the order Lidarr's profile items use.
WANTED = ["MP3-256", "MP3-VBR-V0", "MP3-320"]
CUTOFF = "MP3-320"
MUSIC_CATEGORY = "Music"


def api(base, key, path, method="GET", body=None):
    req = urllib.request.Request(
        base + path,
        method=method,
        data=None if body is None else json.dumps(body).encode(),
        headers={"X-Api-Key": key, "Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=60) as r:
        raw = r.read()
    return json.loads(raw) if raw else None


def flatten(items):
    for it in items:
        if it.get("items"):
            yield from flatten(it["items"])
        elif it.get("quality"):
            yield it["quality"]


def profile_items(schema_items):
    """Every quality exactly once, flat, with only WANTED allowed, in WANTED order at the top."""
    qualities = {q["name"]: q for q in flatten(schema_items)}
    missing = [n for n in WANTED if n not in qualities]
    if missing:
        raise SystemExit(f"Lidarr has no quality named {missing}")
    rest = [q for n, q in qualities.items() if n not in WANTED]
    return ([{"quality": q, "items": [], "allowed": False} for q in rest]
            + [{"quality": qualities[n], "items": [], "allowed": True} for n in WANTED])


def profile_matches(profile):
    flat = [i for i in profile["items"] if not i.get("items")]
    allowed = [i["quality"]["name"] for i in flat if i["allowed"]]
    grouped_allowed = any(i.get("items") and i["allowed"] for i in profile["items"])
    cutoff = next((i["quality"]["name"] for i in flat if i["quality"]["id"] == profile["cutoff"]), None)
    return (allowed == WANTED and not grouped_allowed and cutoff == CUTOFF
            and profile["upgradeAllowed"])


def set_fields(fields, values):
    changed = False
    for f in fields:
        if f["name"] in values and f.get("value") != values[f["name"]]:
            f["value"] = values[f["name"]]
            changed = True
    return changed


def ensure_profile(base, key):
    profiles = api(base, key, "/api/v1/qualityprofile")
    profile = next((p for p in profiles if p["name"] == PROFILE), None)
    if profile and profile_matches(profile):
        return profile["id"]
    body = profile or api(base, key, "/api/v1/qualityprofile/schema")
    body["name"] = PROFILE
    body["items"] = profile_items(body["items"])
    body["cutoff"] = next(i["quality"]["id"] for i in body["items"] if i["quality"]["name"] == CUTOFF)
    body["upgradeAllowed"] = True
    if profile:
        api(base, key, f"/api/v1/qualityprofile/{profile['id']}", "PUT", body)
    else:
        profile = api(base, key, "/api/v1/qualityprofile", "POST", body)
    print(f"changed: quality profile {PROFILE}")
    return profile["id"]


def ensure_profile_in_use(base, key, profile_id):
    for rf in api(base, key, "/api/v1/rootfolder"):
        if rf["defaultQualityProfileId"] != profile_id:
            rf["defaultQualityProfileId"] = profile_id
            api(base, key, f"/api/v1/rootfolder/{rf['id']}", "PUT", rf)
            print(f"changed: root folder {rf['path']} defaults to {PROFILE}")
    stale = [a["id"] for a in api(base, key, "/api/v1/artist") if a["qualityProfileId"] != profile_id]
    if stale:
        api(base, key, "/api/v1/artist/editor", "PUT",
            {"artistIds": stale, "qualityProfileId": profile_id})
        print(f"changed: {len(stale)} artists moved to {PROFILE}")


def ensure_download_client(base, key, host, port, password):
    clients = api(base, key, "/api/v1/downloadclient")
    client = next((c for c in clients if c["implementation"] == "Nzbget"), None)
    body = client or next(s for s in api(base, key, "/api/v1/downloadclient/schema")
                          if s["implementation"] == "Nzbget")
    category = next(f["name"] for f in body["fields"] if f["name"].endswith("Category"))
    values = {"host": host, "port": port, "username": "nzbget", "useSsl": False,
              "addPaused": False, category: MUSIC_CATEGORY}
    # NOTE: the password reads back masked, so it is compared by nothing and sent
    # only alongside a real change. A rotated NZBGet password will not be pushed
    # here until some other field differs.
    if client and not set_fields(client["fields"], values) and client["enable"]:
        return
    values["password"] = password
    set_fields(body["fields"], values)
    body.update(name="NZBGet", enable=True)
    if client:
        api(base, key, f"/api/v1/downloadclient/{client['id']}?forceSave=true", "PUT", body)
    else:
        api(base, key, "/api/v1/downloadclient?forceSave=true", "POST", body)
    print(f"changed: download client NZBGet at {host}:{port}")


def ensure_prowlarr_app(prowlarr, pkey, prowlarr_public, lidarr_public, lkey):
    app = next((a for a in api(prowlarr, pkey, "/api/v1/applications")
                if a["implementation"] == "Lidarr"), None)
    urls = {"prowlarrUrl": prowlarr_public, "baseUrl": lidarr_public}
    # apiKey reads back masked like the NZBGet password, so it rides along with
    # a URL change. A leftover app with the right URLs and a stale key would
    # sync nothing; Prowlarr's own health check reports that one.
    if app and not set_fields(app["fields"], urls) and app["syncLevel"] == "fullSync":
        return
    body = app or next(s for s in api(prowlarr, pkey, "/api/v1/applications/schema")
                       if s["implementation"] == "Lidarr")
    urls["apiKey"] = lkey
    set_fields(body["fields"], urls)
    body.update(name="Lidarr", syncLevel="fullSync")
    if app:
        api(prowlarr, pkey, f"/api/v1/applications/{app['id']}?forceSave=true", "PUT", body)
    else:
        api(prowlarr, pkey, "/api/v1/applications?forceSave=true", "POST", body)
    print("changed: Lidarr application in Prowlarr")


def remove_foreign_indexers(base, key, prowlarr_public):
    for idx in api(base, key, "/api/v1/indexer"):
        url = next((f.get("value") or "" for f in idx["fields"] if f["name"] == "baseUrl"), "")
        if not url.startswith(prowlarr_public):
            api(base, key, f"/api/v1/indexer/{idx['id']}", "DELETE")
            print(f"changed: removed indexer {idx['name']} (not from Prowlarr)")


def main():
    env = os.environ
    base, key = env["LIDARR_URL"], env["LIDARR_API_KEY"]
    profile_id = ensure_profile(base, key)
    ensure_profile_in_use(base, key, profile_id)
    ensure_download_client(base, key, env["NZBGET_HOST"], int(env["NZBGET_PORT"]), env["NZBGET_PASSWORD"])
    ensure_prowlarr_app(env["PROWLARR_URL"], env["PROWLARR_API_KEY"], env["PROWLARR_PUBLIC_URL"],
                        env["LIDARR_PUBLIC_URL"], key)
    remove_foreign_indexers(base, key, env["PROWLARR_PUBLIC_URL"])


if __name__ == "__main__":
    sys.exit(main())
