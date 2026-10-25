#!/usr/bin/env python3
"""Emporia smart-plug power exporter.

The plugs (SSO001) have no local API, so this polls Emporia's cloud through
pyemvue and serves the result as Prometheus gauges. kWh is not exported: the
dashboard integrates watts, the same way it does for the IPMI readings, so both
sources share one definition of energy.

The 1MIN scale returns kWh consumed in the last minute; x60 is kW. The plugs'
cloud display names ("#1 (lab)") are not stable labels, so devices are mapped
by gid in EMPORIA_DEVICES.
"""
import contextlib
import os
import time

from prometheus_client import Gauge, start_http_server
from pyemvue import PyEmVue

USERNAME = os.environ["EMPORIA_USERNAME"]
PASSWORD = os.environ["EMPORIA_PASSWORD"]
# EMPORIA_DEVICES: comma list of "gid=label" pairs.
DEVICES = {}
for entry in os.environ["EMPORIA_DEVICES"].split(","):
    gid, _, label = entry.strip().partition("=")
    DEVICES[int(gid)] = label.strip()
POLL_SECONDS = int(os.environ.get("EMPORIA_POLL_SECONDS", "60"))
PORT = int(os.environ.get("EMPORIA_PORT", "8080"))
# Cognito tokens persist here so a restart refreshes instead of re-logging in.
TOKEN_FILE = os.environ.get("EMPORIA_TOKEN_FILE", "/state/tokens.json")

g_watts = Gauge("emporia_power_watts", "Average draw over the last minute", ["device"])
g_up = Gauge("emporia_up", "1 if the last usage read for this device returned a value", ["device"])
g_last_success = Gauge("emporia_exporter_last_success_timestamp_seconds", "Unix time of the last poll where every device returned a value")


def poll(vue):
    usage = vue.get_device_list_usage(list(DEVICES), None, scale="1MIN", unit="KilowattHours")
    all_ok = True
    for gid, label in DEVICES.items():
        # NOTE: the plug reports one channel, "1,2,3". A None usage means the
        # cloud has no reading for this minute (plug offline), which must not
        # read as 0 W on the dashboard, so the series is dropped instead.
        dev = usage.get(gid)
        kwh = dev.channels["1,2,3"].usage if dev and "1,2,3" in dev.channels else None
        if kwh is None:
            with contextlib.suppress(KeyError):
                g_watts.remove(label)
            g_up.labels(device=label).set(0)
            all_ok = False
            continue
        g_watts.labels(device=label).set(kwh * 60 * 1000)
        g_up.labels(device=label).set(1)
    if all_ok:
        g_last_success.set(time.time())


def main():
    start_http_server(PORT)
    print(f"emporia-exporter on :{PORT}, devices={DEVICES}, poll={POLL_SECONDS}s", flush=True)
    vue = PyEmVue()
    if not vue.login(username=USERNAME, password=PASSWORD, token_storage_file=TOKEN_FILE):
        raise SystemExit("emporia login rejected")
    while True:
        try:
            poll(vue)
        except Exception as e:  # transient cloud errors; staleness alert covers persistence
            for label in DEVICES.values():
                g_up.labels(device=label).set(0)
            print(f"[err] poll: {e}", flush=True)
        time.sleep(POLL_SECONDS)


if __name__ == "__main__":
    main()
