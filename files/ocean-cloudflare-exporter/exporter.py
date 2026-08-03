#!/usr/bin/env python3
"""Cloudflare zone-analytics exporter (Free/Pro compatible).

Polls Cloudflare's GraphQL Analytics API `httpRequests1hGroups` dataset (the
classic hourly dataset, available on Free/Pro plans, unlike the Enterprise-only
adaptive dataset) for each configured zone and exposes the latest 1h bucket as
Prometheus gauges. No raw log lines are stored: the API returns pre-aggregated
counts and that is all we keep.

Values are the CURRENT (in-progress) hourly bucket, so a counter ramps through
the hour and resets at the top of the hour -- that is the native granularity of
this dataset, not a bug. Metrics are suffixed `_1h` to make the gauge (not
counter) semantics obvious.
"""
import json
import os
import time
import urllib.error
import urllib.request

from prometheus_client import Gauge, start_http_server

GRAPHQL_URL = "https://api.cloudflare.com/client/v4/graphql"

TOKEN = os.environ["CF_API_TOKEN"]
# CF_ZONES: comma list of "tag=name" pairs (name is the metric label). A bare tag
# with no "=" falls back to using the tag as its own label.
ZONES = []
for entry in os.environ.get("CF_ZONES", "").split(","):
    entry = entry.strip()
    if not entry:
        continue
    tag, _, name = entry.partition("=")
    ZONES.append((tag.strip(), (name.strip() or tag.strip())))
POLL_SECONDS = int(os.environ.get("CF_POLL_SECONDS", "300"))
PORT = int(os.environ.get("CF_PORT", "8080"))
# Per-URL breakdown (httpRequestsAdaptiveGroups; path dimension works on Free/Pro,
# contentType does not, so static vs dynamic is classified by URL extension).
URL_WINDOW = int(os.environ.get("CF_URL_WINDOW_SECONDS", "3600"))
URL_TOP_N = int(os.environ.get("CF_URL_TOP_N", "10"))       # top non-static URLs per zone
STATIC_TOP_N = int(os.environ.get("CF_STATIC_TOP_N", "20"))  # top static assets per zone
STATIC_EXT = {
    "css", "js", "mjs", "map", "png", "jpg", "jpeg", "gif", "svg", "ico", "webp",
    "avif", "bmp", "woff", "woff2", "ttf", "eot", "otf", "mp4", "webm", "mov",
    "mp3", "wav", "ogg", "pdf", "zip", "gz",
}


def is_static(path):
    """Static asset iff the last path segment has an extension in STATIC_EXT.
    Pages, .html, .php, /api/*, and extensionless paths are NOT static."""
    seg = path.split("?", 1)[0].rsplit("/", 1)[-1]
    ext = seg.rsplit(".", 1)[-1].lower() if "." in seg else ""
    return ext in STATIC_EXT

g_requests = Gauge("cloudflare_zone_requests_1h", "Requests in the latest 1h bucket", ["zone"])
g_bytes = Gauge("cloudflare_zone_bytes_1h", "Bytes served in the latest 1h bucket", ["zone"])
g_cached_req = Gauge("cloudflare_zone_cached_requests_1h", "Cached requests in the latest 1h bucket", ["zone"])
g_cached_bytes = Gauge("cloudflare_zone_cached_bytes_1h", "Cached bytes in the latest 1h bucket", ["zone"])
g_threats = Gauge("cloudflare_zone_threats_1h", "Threats in the latest 1h bucket", ["zone"])
g_uniques = Gauge("cloudflare_zone_uniques_1h", "Unique visitors in the latest 1h bucket", ["zone"])
g_by_status = Gauge("cloudflare_zone_requests_by_status_1h", "Requests by edge response status, latest 1h bucket", ["zone", "status"])
g_by_country = Gauge("cloudflare_zone_requests_by_country_1h", "Requests by client country, latest 1h bucket", ["zone", "country"])
g_up = Gauge("cloudflare_zone_up", "1 if the last analytics query for this zone succeeded", ["zone"])
g_url_requests = Gauge("cloudflare_zone_url_requests", "Requests for a top non-static URL (latest window)", ["zone", "path"])
g_url_bytes = Gauge("cloudflare_zone_url_bytes", "Edge bytes for a top non-static URL (latest window)", ["zone", "path"])
g_static_requests = Gauge("cloudflare_zone_static_requests", "Requests for a top static-asset URL (latest window)", ["zone", "path"])
g_static_bytes = Gauge("cloudflare_zone_static_bytes", "Edge bytes for a top static-asset URL (latest window)", ["zone", "path"])
g_last_success = Gauge("cloudflare_exporter_last_success_timestamp_seconds", "Unix time of the last fully successful poll cycle")

QUERY = """
query($z:String!,$s:Time!,$u:Time!){viewer{zones(filter:{zoneTag:$z}){
 httpRequests1hGroups(limit:1,filter:{datetime_geq:$s,datetime_leq:$u},orderBy:[datetime_DESC]){
  dimensions{datetime}
  sum{requests bytes cachedRequests cachedBytes threats
   responseStatusMap{edgeResponseStatus requests}
   countryMap{clientCountryName requests}}
  uniq{uniques}}}}}
"""


URL_QUERY = """
query($z:String!,$s:Time!,$u:Time!){viewer{zones(filter:{zoneTag:$z}){
 httpRequestsAdaptiveGroups(limit:250,filter:{datetime_geq:$s,datetime_leq:$u},orderBy:[count_DESC]){
  count dimensions{clientRequestPath} sum{edgeResponseBytes}}}}}
"""


def _graphql(query, variables):
    body = json.dumps({"query": query, "variables": variables}).encode()
    req = urllib.request.Request(GRAPHQL_URL, data=body, method="POST", headers={
        "Authorization": f"Bearer {TOKEN}",
        "Content-Type": "application/json",
    })
    with urllib.request.urlopen(req, timeout=20) as r:
        payload = json.load(r)
    if payload.get("errors"):
        raise RuntimeError(payload["errors"])
    return payload["data"]["viewer"]["zones"][0]


def query_urls(tag):
    now = time.time()
    zone = _graphql(URL_QUERY, {
        "z": tag,
        "s": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(now - URL_WINDOW)),
        "u": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(now)),
    })
    return [{"path": g["dimensions"]["clientRequestPath"], "count": g["count"],
             "bytes": g["sum"]["edgeResponseBytes"]} for g in zone["httpRequestsAdaptiveGroups"]]


def query_zone(tag):
    now = time.time()
    body = json.dumps({
        "query": QUERY,
        "variables": {
            "z": tag,
            "s": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(now - 7200)),
            "u": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(now)),
        },
    }).encode()
    req = urllib.request.Request(GRAPHQL_URL, data=body, method="POST", headers={
        "Authorization": f"Bearer {TOKEN}",
        "Content-Type": "application/json",
    })
    with urllib.request.urlopen(req, timeout=20) as r:
        payload = json.load(r)
    if payload.get("errors"):
        raise RuntimeError(payload["errors"])
    groups = payload["data"]["viewer"]["zones"][0]["httpRequests1hGroups"]
    return groups[0] if groups else None


def poll():
    all_ok = True
    # Clear the multi-label metrics ONCE per cycle (not per-zone, or each zone
    # would wipe the previous zone's series); every zone re-adds its rows below.
    g_by_status.clear()
    g_by_country.clear()
    g_url_requests.clear()
    g_url_bytes.clear()
    g_static_requests.clear()
    g_static_bytes.clear()
    for tag, name in ZONES:
        try:
            g = query_zone(tag)
            if g is None:  # no traffic in the window; report zeros, still "up"
                for m in (g_requests, g_bytes, g_cached_req, g_cached_bytes, g_threats, g_uniques):
                    m.labels(zone=name).set(0)
                g_up.labels(zone=name).set(1)
                continue
            s = g["sum"]
            g_requests.labels(zone=name).set(s["requests"])
            g_bytes.labels(zone=name).set(s["bytes"])
            g_cached_req.labels(zone=name).set(s["cachedRequests"])
            g_cached_bytes.labels(zone=name).set(s["cachedBytes"])
            g_threats.labels(zone=name).set(s["threats"])
            g_uniques.labels(zone=name).set(g["uniq"]["uniques"])
            for row in s.get("responseStatusMap", []):
                g_by_status.labels(zone=name, status=str(row["edgeResponseStatus"])).set(row["requests"])
            for row in s.get("countryMap", []):
                g_by_country.labels(zone=name, country=row["clientCountryName"]).set(row["requests"])
            g_up.labels(zone=name).set(1)
        except Exception as e:  # one bad zone must not blank the others
            g_up.labels(zone=name).set(0)
            all_ok = False
            print(f"[err] zone {name} ({tag}): {e}", flush=True)
            continue
        # Per-URL breakdown is best-effort: a failure here must not fail the zone
        # or blank its base metrics above.
        try:
            rows = query_urls(tag)
            static = sorted((r for r in rows if is_static(r["path"])), key=lambda r: r["count"], reverse=True)
            dynamic = sorted((r for r in rows if not is_static(r["path"])), key=lambda r: r["count"], reverse=True)
            for r in dynamic[:URL_TOP_N]:
                g_url_requests.labels(zone=name, path=r["path"]).set(r["count"])
                g_url_bytes.labels(zone=name, path=r["path"]).set(r["bytes"])
            for r in static[:STATIC_TOP_N]:
                g_static_requests.labels(zone=name, path=r["path"]).set(r["count"])
                g_static_bytes.labels(zone=name, path=r["path"]).set(r["bytes"])
        except Exception as e:
            print(f"[err] zone {name} urls: {e}", flush=True)
    if all_ok:
        g_last_success.set(time.time())


def main():
    start_http_server(PORT)
    print(f"cloudflare-exporter on :{PORT}, {len(ZONES)} zones, poll={POLL_SECONDS}s", flush=True)
    while True:
        poll()
        time.sleep(POLL_SECONDS)


if __name__ == "__main__":
    main()
