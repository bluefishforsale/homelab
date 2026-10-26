#!/usr/bin/env python3
"""Pins exporter.rate_at(): run `python3 files/emporia-exporter/test_rates.py` (no pytest).

A wrong rate silently misprices every minute it covers, and the edges (peak
start/end, season change, a UTC date that is still yesterday in Oakland) are
where it goes wrong.
"""
import datetime
import importlib.util
import json
import os
import sys
import types

HERE = os.path.dirname(os.path.abspath(__file__))
# The exporter imports pyemvue/prometheus_client and reads env at import; CI has
# neither package, and rate_at needs neither.
for name in ("pyemvue", "prometheus_client"):
    sys.modules[name] = types.SimpleNamespace(PyEmVue=None, Gauge=lambda *a, **k: None, start_http_server=None)
os.environ.update(EMPORIA_USERNAME="u", EMPORIA_PASSWORD="p", EMPORIA_DEVICES="1=a", ELECTRICITY_RATES=json.dumps({
    "timezone": "America/Los_Angeles",
    "summer_months": [6, 7, 8, 9],
    "peak_hours": [16, 21],
    "rates": {"summer": {"peak": 0.56, "off_peak": 0.43}, "winter": {"peak": 0.43, "off_peak": 0.40}},
}))
_spec = importlib.util.spec_from_file_location("exporter", os.path.join(HERE, "exporter.py"))
exporter = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(exporter)

PT = exporter.TZ


def at(*args):
    return exporter.rate_at(datetime.datetime(*args, tzinfo=PT))


def test_peak_starts_at_1600_local():
    assert at(2026, 8, 1, 15, 59) == 0.43
    assert at(2026, 8, 1, 16, 0) == 0.56


def test_peak_ends_before_2100_local():
    assert at(2026, 8, 1, 20, 59) == 0.56
    assert at(2026, 8, 1, 21, 0) == 0.43


def test_season_is_judged_in_oakland_not_utc():
    # 2026-09-30 20:00 PDT is already 2026-10-01 in UTC; still summer peak.
    utc = datetime.datetime(2026, 10, 1, 3, 0, tzinfo=datetime.timezone.utc)
    assert exporter.rate_at(utc) == 0.56


def test_peak_hours_follow_dst_not_a_fixed_utc_offset():
    # 16:00 PST in January is 00:00 UTC the next day.
    utc = datetime.datetime(2027, 1, 16, 0, 0, tzinfo=datetime.timezone.utc)
    assert exporter.rate_at(utc) == 0.43
    assert exporter.rate_at(utc - datetime.timedelta(minutes=1)) == 0.40


def test_winter_runs_october_through_may():
    assert at(2026, 10, 1, 12, 0) == 0.40
    assert at(2027, 5, 31, 12, 0) == 0.40
    assert at(2027, 6, 1, 12, 0) == 0.43


if __name__ == "__main__":
    tests = [v for k, v in sorted(globals().items()) if k.startswith("test_")]
    for t in tests:
        t()
        print(f"ok  {t.__name__}")
    print(f"{len(tests)} passed")
