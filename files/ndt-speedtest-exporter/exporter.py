#!/usr/bin/env python3
"""M-Lab NDT7 multi-location speed-test exporter.

Every INTERVAL seconds, ask the M-Lab locate service for the nearest ndt7
servers (it returns ~4, each with a per-test access token), run a download and
upload test against each, and expose the results as Prometheus gauges labelled
by server machine + city. Multiple servers = the multi-location view the old
ndt-exporter gave; a single one still works if locate only returns one.

Server-measured throughput (from the peer's TCP_INFO) is authoritative and is
what we export; we fall back to the client-side byte count if the server sends
no measurement. Per-server failures are isolated so one bad server never blanks
the others.
"""
import asyncio
import json
import os
import random
import time
import urllib.request

import websockets
from prometheus_client import Gauge, start_http_server

SUBPROTO = "net.measurementlab.ndt.v7"
LOCATE_URL = "https://locate.measurementlab.net/v2/nearest/ndt/ndt7"

INTERVAL = int(os.environ.get("NDT_INTERVAL_SECONDS", "1800"))
DURATION = int(os.environ.get("NDT_TEST_SECONDS", "8"))
PORT = int(os.environ.get("NDT_PORT", "9140"))
MAX_SERVERS = int(os.environ.get("NDT_MAX_SERVERS", "4"))
# M-Lab rate-limits fixed-interval "periodic" Locate callers per-IP (429 "too
# many periodic requests"). Reuse the servers/tokens from one Locate response
# across several test cycles instead of re-locating every cycle. LOCATE_TTL must
# stay under the access-token lifetime (~2h); the all-fail path below forces an
# early refresh if a token expires sooner.
LOCATE_TTL = int(os.environ.get("NDT_LOCATE_TTL_SECONDS", "5400"))
# +/- fraction of jitter on the sleep so we are not perfectly periodic.
JITTER = float(os.environ.get("NDT_JITTER_FRACTION", "0.2"))

LABELS = ["machine", "city", "country"]
g_download = Gauge("ndt_download_mbps", "NDT7 download throughput (Mbps)", LABELS)
g_upload = Gauge("ndt_upload_mbps", "NDT7 upload throughput (Mbps)", LABELS)
g_rtt = Gauge("ndt_min_rtt_ms", "NDT7 minimum round-trip time (ms)", LABELS)
g_last_success = Gauge("ndt_last_success_timestamp_seconds", "Unix time of last fully successful cycle")
g_cycle_seconds = Gauge("ndt_cycle_duration_seconds", "Wall-clock seconds for the last full test cycle")
g_up = Gauge("ndt_server_up", "1 if the last test against this server succeeded", LABELS)
# HTTP status of the last M-Lab Locate request: 200 ok, 429 rate-limited, 0
# non-HTTP error. Stays populated even when we can't run tests, so a dashboard
# panel shows exactly when the 429s stop.
g_locate_status = Gauge("ndt_locate_http_status", "HTTP status of the last M-Lab Locate request (200 ok, 429 rate-limited, 0 error)")


def locate_servers():
    with urllib.request.urlopen(LOCATE_URL, timeout=15) as r:
        data = json.load(r)
    return data.get("results", [])[:MAX_SERVERS]


# Cached Locate result plus a backoff clock. On a Locate failure (typically 429)
# we keep the last-known servers so the dashboard never blanks, and refuse to
# re-hit Locate until cooldown_until, doubling the wait each consecutive failure.
_locate = {"servers": [], "at": 0.0, "fails": 0, "cooldown_until": 0.0}


def cached_servers():
    now = time.time()
    fresh = _locate["servers"] and now - _locate["at"] < LOCATE_TTL
    if fresh or now < _locate["cooldown_until"]:
        return _locate["servers"]  # reuse cache; or backing off -> stale/empty
    try:
        servers = locate_servers()
        g_locate_status.set(200)
        if servers:
            _locate.update(servers=servers, at=now, fails=0, cooldown_until=0.0)
        return _locate["servers"]
    except Exception as e:
        g_locate_status.set(getattr(e, "code", 0))  # HTTPError has .code (e.g. 429); other errors -> 0
        _locate["fails"] += 1
        backoff = min(LOCATE_TTL, 300 * 2 ** (_locate["fails"] - 1))
        _locate["cooldown_until"] = now + backoff
        print(f"[err] locate failed (#{_locate['fails']}, backoff {backoff:.0f}s): {e}", flush=True)
        return _locate["servers"]


def _server_mbps(measurement, field="BytesAcked"):
    """Server-side Mbps from a TCPInfo measurement: <field>*8 / ElapsedTime(us).
    Download is the receiver's acked bytes (BytesAcked); upload is what the
    server actually received (BytesReceived)."""
    ti = measurement.get("TCPInfo", {})
    et = ti.get("ElapsedTime")
    if et and ti.get(field) is not None:
        return ti[field] * 8.0 / et  # bytes*8 / microseconds == Mbps
    return None


def _min_rtt_ms(measurement):
    ti = measurement.get("TCPInfo", {})
    if ti.get("MinRTT"):
        return ti["MinRTT"] / 1000.0  # microseconds -> ms
    return None


async def run_download(url):
    total = 0
    best_mbps = None
    rtt = None
    start = time.monotonic()
    async with websockets.connect(url, subprotocols=[SUBPROTO], max_size=None,
                                  open_timeout=15, ping_interval=None) as ws:
        while time.monotonic() - start < DURATION:
            try:
                msg = await asyncio.wait_for(ws.recv(), timeout=DURATION)
            except (asyncio.TimeoutError, websockets.ConnectionClosed):
                break
            if isinstance(msg, (bytes, bytearray)):
                total += len(msg)
            else:
                m = json.loads(msg)
                best_mbps = _server_mbps(m) or best_mbps
                rtt = _min_rtt_ms(m) or rtt
    elapsed = max(time.monotonic() - start, 1e-6)
    client_mbps = total * 8 / elapsed / 1e6
    return (best_mbps or client_mbps), rtt


async def run_upload(url):
    """ndt7 upload: stream binary as fast as possible while draining the
    server's measurement frames; the server's BytesReceived is the truth."""
    best_mbps = None
    rtt = None
    sent = 0
    payload = bytes(1 << 16)  # 64 KiB messages (ndt7 spec starts here)
    start = time.monotonic()
    async with websockets.connect(url, subprotocols=[SUBPROTO], max_size=None,
                                  open_timeout=15, ping_interval=None) as ws:
        async def drain():
            nonlocal best_mbps, rtt
            try:
                while True:
                    msg = await ws.recv()
                    if isinstance(msg, str):
                        m = json.loads(msg)
                        best_mbps = _server_mbps(m, "BytesReceived") or best_mbps
                        rtt = _min_rtt_ms(m) or rtt
            except (websockets.ConnectionClosed, asyncio.CancelledError):
                pass

        reader = asyncio.ensure_future(drain())
        while time.monotonic() - start < DURATION:
            await ws.send(payload)
            sent += len(payload)
            await asyncio.sleep(0)  # yield so drain() consumes server measurements
        # let drain catch the final (highest-BytesReceived) frames before closing
        await asyncio.sleep(0.3)
        reader.cancel()
        try:
            await reader
        except asyncio.CancelledError:
            pass
    # Server-measured BytesReceived is authoritative. The client byte count
    # overcounts locally-buffered data, so only use it if the server sent
    # nothing at all.
    elapsed = max(time.monotonic() - start, 1e-6)
    client_mbps = sent * 8 / elapsed / 1e6
    return (best_mbps or client_mbps), rtt


async def test_server(result):
    machine = result.get("machine", "unknown")
    loc = result.get("location", {})
    labels = dict(machine=machine, city=loc.get("city", ""), country=loc.get("country", ""))
    urls = result.get("urls", {})
    try:
        dl, rtt_d = await run_download(urls["wss:///ndt/v7/download"])
        ul, rtt_u = await run_upload(urls["wss:///ndt/v7/upload"])
        g_download.labels(**labels).set(round(dl, 2))
        g_upload.labels(**labels).set(round(ul or 0, 2))
        rtt = rtt_d or rtt_u
        if rtt:
            g_rtt.labels(**labels).set(round(rtt, 3))
        g_up.labels(**labels).set(1)
        print(f"[ok] {machine} {loc.get('city')}: down={dl:.1f} up={(ul or 0):.1f} Mbps rtt={rtt}", flush=True)
        return True
    except Exception as e:  # one bad server must not blank the rest
        g_up.labels(**labels).set(0)
        print(f"[err] {machine}: {e}", flush=True)
        return False


async def cycle():
    start = time.monotonic()
    servers = cached_servers()
    if not servers:
        print("[warn] no servers available (locate rate-limited?)", flush=True)
        return
    results = [await test_server(s) for s in servers]  # serial: don't let tests contend
    g_cycle_seconds.set(round(time.monotonic() - start, 2))
    if results and all(results):
        g_last_success.set(time.time())
    elif not any(results):
        _locate["at"] = 0.0  # every server failed: tokens likely expired, re-locate next cycle


def main():
    start_http_server(PORT)
    print(f"ndt-speedtest-exporter on :{PORT}, interval={INTERVAL}s, duration={DURATION}s", flush=True)
    while True:
        asyncio.run(cycle())
        time.sleep(INTERVAL * (1 + random.uniform(-JITTER, JITTER)))


if __name__ == "__main__":
    main()
