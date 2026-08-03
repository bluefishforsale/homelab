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

g_requests = Gauge("cloudflare_zone_requests_1h", "Requests in the latest 1h bucket", ["zone"])
g_bytes = Gauge("cloudflare_zone_bytes_1h", "Bytes served in the latest 1h bucket", ["zone"])
g_cached_req = Gauge("cloudflare_zone_cached_requests_1h", "Cached requests in the latest 1h bucket", ["zone"])
g_cached_bytes = Gauge("cloudflare_zone_cached_bytes_1h", "Cached bytes in the latest 1h bucket", ["zone"])
g_threats = Gauge("cloudflare_zone_threats_1h", "Threats in the latest 1h bucket", ["zone"])
g_uniques = Gauge("cloudflare_zone_uniques_1h", "Unique visitors in the latest 1h bucket", ["zone"])
g_by_status = Gauge("cloudflare_zone_requests_by_status_1h", "Requests by edge response status, latest 1h bucket", ["zone", "status"])
g_by_country = Gauge("cloudflare_zone_requests_by_country_1h", "Requests by client country, latest 1h bucket", ["zone", "country"])
g_up = Gauge("cloudflare_zone_up", "1 if the last analytics query for this zone succeeded", ["zone"])
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
