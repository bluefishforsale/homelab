#!/usr/bin/env python3
"""Reconstruct a dated timeline of DNS / mail-auth signals for a domain.

There is no free API that replays a domain's full DNS history (that's what
MXToolbox sells). What we CAN do is merge every *dated* signal that real,
grounded sources expose, and lay them on one timeline:

  1. DMARC `policy_published` from the aggregate reports in reports/ — each
     report records what the DMARC record looked like to that reporter on that
     day. This is literal historical DNS, one datapoint per report.
  2. SOA serial — GoDaddy/most hosts encode it as YYYYMMDDnn, so it reveals the
     date of the most recent zone edit (current value only).
  3. whois Creation / Updated dates — when the registration last changed.
  4. crt.sh certificate transparency — earliest `not_before` per hostname, i.e.
     the first time a cert proves that hostname existed (needs network).

Every line is tagged with its source and a confidence note. Nothing is inferred
beyond what the source states. Gaps are gaps.

Usage:
  dns-timeline <domain> [--reports DIR]
  dns-timeline terrac.com
"""
import glob
import json
import os
import re
import subprocess
import sys
import urllib.request
from datetime import datetime, timezone
from xml.etree import ElementTree as ET


def sh(args, timeout=20):
    try:
        return subprocess.run(args, capture_output=True, text=True,
                              timeout=timeout).stdout.strip()
    except Exception:
        return ""


def epoch_day(ts):
    return datetime.fromtimestamp(int(ts), timezone.utc).strftime("%Y-%m-%d")


def from_dmarc_reports(domain, reports_dir):
    """Each report's policy_published = the DMARC record on that report's day."""
    events = []
    seen = set()
    for f in sorted(glob.glob(os.path.join(reports_dir, "*.xml"))):
        try:
            r = ET.parse(f).getroot()
        except ET.ParseError:
            continue
        rid = r.findtext("report_metadata/report_id")
        if rid in seen:
            continue
        seen.add(rid)
        pol = r.find("policy_published")
        if pol is None or pol.findtext("domain") != domain:
            continue
        beg = r.findtext("report_metadata/date_range/begin")
        if not beg:
            continue
        rec = "p={} sp={} adkim={} aspf={} pct={}".format(
            pol.findtext("p"), pol.findtext("sp"),
            pol.findtext("adkim"), pol.findtext("aspf"), pol.findtext("pct"))
        reporter = os.path.basename(f).split("!")[0]
        events.append((epoch_day(beg), "DMARC-observed", rec,
                       f"reporter={reporter}"))
    return events


def soa_event(domain):
    soa = sh(["dig", "+short", "SOA", domain])
    if not soa:
        return []
    parts = soa.split()
    if len(parts) < 3:
        return []
    serial = parts[2]
    note = "serial=%s" % serial
    # GoDaddy/BIND convention: YYYYMMDDnn.
    m = re.match(r"^(20\d{2})(\d{2})(\d{2})(\d{2})$", serial)
    if m and 1 <= int(m.group(2)) <= 12 and 1 <= int(m.group(3)) <= 31:
        day = f"{m.group(1)}-{m.group(2)}-{m.group(3)}"
        return [(day, "zone-edited", f"SOA serial {serial} (YYYYMMDDnn)",
                 "last zone change per serial; nn=%s" % m.group(4))]
    return [("?", "zone-serial", note, "serial not date-encoded; no date")]


def whois_events(domain):
    out = sh(["whois", domain], timeout=25)
    events = []
    for label, kind in [("Creation Date", "domain-registered"),
                        ("Updated Date", "registration-updated")]:
        m = re.search(rf"{label}:\s*([0-9]{{4}}-[0-9]{{2}}-[0-9]{{2}})", out)
        if m:
            events.append((m.group(1), kind, f"whois {label}", "registry record"))
    return events


def crtsh_events(domain):
    url = f"https://crt.sh/?q=%25.{domain}&output=json"
    try:
        with urllib.request.urlopen(url, timeout=20) as r:
            data = json.load(r)
    except Exception as e:
        return [("?", "crt.sh", "(unreachable)", str(e)[:60])]
    earliest = {}
    for x in data:
        nb = (x.get("not_before") or "")[:10]
        if not nb:
            continue
        for n in (x.get("name_value") or "").splitlines():
            n = n.strip()
            if n and (n not in earliest or nb < earliest[n]):
                earliest[n] = nb
    return [(d, "cert-first-seen", f"TLS cert for {host}", "crt.sh not_before")
            for host, d in earliest.items()]


def main():
    if len(sys.argv) < 2 or sys.argv[1] in ("-h", "--help"):
        print(__doc__)
        return
    domain = sys.argv[1]
    reports_dir = "reports"
    if "--reports" in sys.argv:
        reports_dir = sys.argv[sys.argv.index("--reports") + 1]

    events = []
    events += whois_events(domain)
    events += soa_event(domain)
    events += from_dmarc_reports(domain, reports_dir)
    events += crtsh_events(domain)

    dated = sorted([e for e in events if e[0] != "?"])
    undated = [e for e in events if e[0] == "?"]

    # Collapse runs of identical DMARC-observed policy into a date range, so a
    # stable policy reads as "unchanged from X to Y" instead of N noisy rows.
    collapsed = []
    i = 0
    while i < len(dated):
        day, kind, detail, note = dated[i]
        if kind == "DMARC-observed":
            j = i
            reporters = set()
            while j < len(dated) and dated[j][1] == "DMARC-observed" \
                    and dated[j][2] == detail:
                reporters.add(dated[j][3].replace("reporter=", ""))
                j += 1
            last = dated[j - 1][0]
            span = day if last == day else f"{day}..{last}"
            collapsed.append((span, "DMARC-observed", detail,
                              f"stable across {len(reporters)} reporter(s): "
                              + ", ".join(sorted(reporters))))
            i = j
        else:
            collapsed.append(dated[i])
            i += 1

    print(f"=== DNS / mail-auth timeline for {domain} ===")
    print(f"{'date':24}{'event':22}{'detail'}")
    print("-" * 80)
    for day, kind, detail, note in collapsed:
        print(f"{day:24}{kind:22}{detail}")
        if note:
            print(f"{'':24}{'':22}  ({note})")
    if undated:
        print("\n-- undated signals --")
        for _, kind, detail, note in undated:
            print(f"  {kind}: {detail}  ({note})")

    print("\nNote: this merges only dated signals from grounded sources. It is")
    print("NOT a full DNS change log — for record-by-record history use a paid")
    print("passive-DNS service (SecurityTrails, Farsight DNSDB) or MXToolbox.")


if __name__ == "__main__":
    main()
