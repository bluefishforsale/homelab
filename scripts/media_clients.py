#!/usr/bin/env python3
"""One client for the ocean media stack: radarr, sonarr, tdarr, plex, overseerr.

Self-discovers each service's key from the live config on ocean (env var wins),
so nothing secret is committed. Importable (get/post/SERVICES) and a CLI:

  media_clients.py ping                     # reachability + headline per service
  media_clients.py get radarr /api/v3/movie # ad-hoc GET, prints JSON
  media_clients.py get plex /library/sections
  media_clients.py get overseerr /api/v1/request count=5
  media_clients.py tdarr-stats              # tdarr transcode totals

Run on ocean (defaults to localhost). Plex Preferences.xml is 0600 owned by
media, so plex needs PLEX_TOKEN in env or run as media/root (or sudo).
"""
import json
import os
import re
import sys
import urllib.parse
import urllib.request

DATA = "/data01/services"


def _xml_key(path):
    with open(path) as f:
        m = re.search(r"<ApiKey>([a-f0-9]+)</ApiKey>", f.read())
    return m.group(1) if m else None


def _overseerr_key():
    with open(f"{DATA}/overseerr/config/settings.json") as f:
        return json.load(f)["main"]["apiKey"]


def _plex_token():
    p = f"{DATA}/plex/config/Library/Application Support/Plex Media Server/Preferences.xml"
    m = re.search(r'PlexOnlineToken="([^"]+)"', open(p).read())
    return m.group(1) if m else None


# auth: how the key rides the request. ('query', name) | ('header', name) | None
SERVICES = {
    "radarr":    {"url": "http://localhost:8903", "auth": ("query", "apikey"),
                  "key_env": "RADARR_APIKEY", "key": lambda: _xml_key(f"{DATA}/radarr/config.xml")},
    "sonarr":    {"url": "http://localhost:8902", "auth": ("query", "apikey"),
                  "key_env": "SONARR_APIKEY", "key": lambda: _xml_key(f"{DATA}/sonarr/config.xml")},
    "tdarr":     {"url": "http://localhost:8265", "auth": None,
                  "key_env": None, "key": lambda: None},
    "plex":      {"url": "http://localhost:32400", "auth": ("query", "X-Plex-Token"),
                  "key_env": "PLEX_TOKEN", "key": _plex_token},
    "overseerr": {"url": "http://localhost:5055", "auth": ("header", "X-Api-Key"),
                  "key_env": "OVERSEERR_APIKEY", "key": _overseerr_key},
}


def _key(name):
    s = SERVICES[name]
    if s["key_env"] and os.environ.get(s["key_env"]):
        return os.environ[s["key_env"]]
    return s["key"]()


def _url(name):
    return os.environ.get(f"{name.upper()}_URL", SERVICES[name]["url"])


def _request(name, path, params, data=None, method=None):
    s = SERVICES[name]
    params = dict(params or {})
    headers = {"Accept": "application/json"}
    auth = s["auth"]
    if auth:
        kind, field = auth
        key = _key(name)
        if kind == "query":
            params[field] = key
        else:
            headers[field] = key
    url = _url(name) + path
    if params:
        url += ("&" if "?" in url else "?") + urllib.parse.urlencode(params)
    body = json.dumps(data).encode() if data is not None else None
    if body:
        headers["Content-Type"] = "application/json"
    if method is None:
        method = "POST" if body else "GET"
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=120) as r:
        txt = r.read()
        return json.loads(txt) if txt else None


def get(name, path, **params):
    return _request(name, path, params)


def post(name, path, data, **params):
    return _request(name, path, params, data=data)


def delete(name, path, **params):
    return _request(name, path, params, method="DELETE")


def tdarr_stats():
    return post("tdarr", "/api/v2/cruddb",
                {"data": {"collection": "StatisticsJSONDB", "mode": "getById", "docID": "statistics"}})


def ping():
    def headline(n):
        if n in ("radarr", "sonarr"):
            v = get(n, "/api/v3/system/status")["version"]
            kind = "movie" if n == "radarr" else "series"
            c = len(get(n, f"/api/v3/{kind}"))
            return f"v{v}, {c} {kind}"
        if n == "tdarr":
            s = tdarr_stats()
            return f"{s['totalFileCount']} files, {s['sizeDiff']/1000:.1f} TB saved"
        if n == "plex":
            secs = get(n, "/library/sections")["MediaContainer"]["Directory"]
            return f"{len(secs)} libraries"
        if n == "overseerr":
            c = get(n, "/api/v1/request/count")
            return f"v{get(n, '/api/v1/status')['version']}, {c['total']} requests, {c['pending']} pending"

    for n in SERVICES:
        try:
            print(f"{n:10} OK   {headline(n)}")
        except Exception as e:
            print(f"{n:10} FAIL {type(e).__name__}: {e}")


if __name__ == "__main__":
    a = sys.argv[1:]
    if a and a[0] == "ping":
        ping()
    elif a and a[0] == "tdarr-stats":
        print(json.dumps(tdarr_stats(), indent=2))
    elif len(a) >= 3 and a[0] == "get":
        params = dict(kv.split("=", 1) for kv in a[3:])
        print(json.dumps(get(a[1], a[2], **params), indent=2)[:20000])
    else:
        sys.exit(__doc__)
