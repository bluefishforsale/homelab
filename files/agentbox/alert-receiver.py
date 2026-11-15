#!/usr/bin/env python3
"""agentbox alert receiver: the Alertmanager -> ntfy bridge.

Rendered from files/agentbox/alert-receiver.py by the agentbox playbook.

Alertmanager POSTs its webhook JSON here, and every alert, firing or resolved,
becomes one ntfy push. It used to open a GitHub issue per alert as well, and on
2026-11-15 that was retired: the issues were noise nobody worked from, and the
push, the email and Alertmanager itself already carry the signal.

Deliberately dependency-free (stdlib only) and single-file: it reads config from
the environment, rendered into the systemd unit.
"""
import base64
import json
import os
import re
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

NTFY_URL = os.environ["ALERT_NTFY_URL"]           # e.g. http://ntfy.home:8090/homelab
LISTEN_PORT = int(os.environ.get("ALERT_LISTEN_PORT", "9098"))
RC_URL = os.environ.get("ALERT_RC_URL", "https://claude.ai/code")  # live RC console
# ntfy is deny-all (public via ntfy.terrac.com); publish with basic auth.
NTFY_USER = os.environ.get("ALERT_NTFY_USER", "")
NTFY_PASS = os.environ.get("ALERT_NTFY_PASS", "")

PRIO = {"critical": "urgent", "warning": "high", "info": "default"}

# Ordered most-specific first; first hit wins. These are the labels that say
# WHICH thing broke, as opposed to which rule fired or which host it fired on.
SUBJECT_LABELS = ("project", "name", "service", "container", "unit", "pool",
                  "device", "mountpoint", "phy", "controller", "target")
# Trailing systemd type, which is noise once the unit is already identified.
_UNIT_SUFFIX = re.compile(r"\.(service|timer|mount|socket|target)$")

def subject(labels):
    """The 'which one' half of a notification title.

    Without this the title was just the rule name, so DockerRefreshStale on a
    host running 31 compose projects said only "ocean.home" and you had to open
    the alert to learn it meant mem0. Fixing it here rather than in each rule's
    summary covers every alert at once, including ones not written yet, and the
    rules that already name their subject lose nothing by repeating it in the
    title where it is actually scannable.

    Returns "" when no label identifies a subject: some alerts are genuinely
    host-wide (HighCpuUsage), and some report an aggregate that has no
    per-service label at all (homelab_db_backup_* is one metric for every
    database it backs up). A title is a scanning aid, so no subject beats a
    misleading one.
    """
    for key in SUBJECT_LABELS:
        value = labels.get(key)
        if value:
            # ntfy carries the title in an HTTP header, which is latin-1, so
            # non-ASCII raises rather than degrades. Strip it here, not later.
            return _UNIT_SUFFIX.sub("", value).encode("ascii", "ignore").decode()
    return ""


def ntfy(title, message, priority="default", tags="", click=""):
    headers = {"Title": title, "Priority": priority, "Tags": tags}
    if click:
        headers["Click"] = click
    if NTFY_USER:
        cred = base64.b64encode(f"{NTFY_USER}:{NTFY_PASS}".encode()).decode()
        headers["Authorization"] = f"Basic {cred}"
    req = urllib.request.Request(NTFY_URL, data=message.encode(), headers=headers)
    try:
        urllib.request.urlopen(req, timeout=10).read()
    except Exception as e:  # awareness is best-effort; never crash the receiver
        print(f"ntfy push failed: {e}", flush=True)


def handle(payload):
    for alert in payload.get("alerts", []):
        labels = alert["labels"]
        sev = labels.get("severity", "warning")
        name = labels.get("alertname", "alert")
        inst = labels.get("instance", labels.get("job", ""))
        summary = alert.get("annotations", {}).get("summary", "")
        # "DockerRefreshStale: mem0" rather than "DockerRefreshStale", so the
        # notification list is readable without opening anything.
        who = subject(labels)
        ident = f"{name}: {who}" if who else name
        # Hardware alerts (remediation: manual) need hands, not a console.
        manual = labels.get("remediation") == "manual"
        # NB: emoji must go in Tags (ntfy renders them), never the Title header:
        # HTTP headers are latin-1 and non-ASCII there raises.
        if alert.get("status") == "firing":
            # Critical: one-tap into the live RC console to supervise.
            ntfy(f"{ident} ({sev})",
                 "\n".join(x for x in (inst, summary) if x),
                 PRIO.get(sev, "default"),
                 "wrench,homelab" if manual else "rotating_light,homelab",
                 click=RC_URL if sev == "critical" and not manual else "")
        else:
            ntfy(f"{'cleared' if manual else 'resolved'}: {ident}",
                 f"{inst}\n{summary}".strip(), "min", "white_check_mark,homelab")


def _selftest():
    """Network-free defect ledger. Stubs `ntfy`."""
    global ntfy
    pushes = []
    ntfy = lambda *a, **k: pushes.append((a, k))

    # 2026-11-15: an issue per alert was noise; every alert is one push and
    # nothing else. The receiver has no GitHub client left to call.
    handle({"alerts": [
        {"status": "firing", "labels": {"alertname": "HighCpuUsage", "severity": "critical",
                                        "instance": "ocean.home"}},
        {"status": "firing", "labels": {"alertname": "ZfsDiskErrors", "severity": "critical",
                                        "remediation": "manual", "device": "wwn-0x5000c500a1b2c3d4"}},
        {"status": "resolved", "labels": {"alertname": "HighCpuUsage", "instance": "ocean.home"}},
    ]})
    assert len(pushes) == 3, pushes
    assert pushes[0][1]["click"] == RC_URL, "critical software alert taps into the RC console"
    assert pushes[1][1]["click"] == "", "hardware needs hands, not the RC console"
    assert pushes[2][0][0] == "resolved: HighCpuUsage", pushes[2]

    # 2026-09-08: DockerRefreshStale pushed "ocean.home / docker image refresh
    # stale on ocean.home" for a host running 31 compose projects, naming none
    # of them.
    assert subject({"project": "mem0"}) == "mem0"
    assert subject({"name": "docker-refresh@mem0.timer"}) == "docker-refresh@mem0"
    assert subject({"name": "plex.service"}) == "plex"
    assert subject({"pool": "data01", "device": "sde"}) == "data01", "most specific label wins"
    # Host-wide alerts have no subject, and inventing one would be worse than none.
    assert subject({"instance": "ocean.home", "severity": "warning"}) == ""
    assert subject({}) == ""
    # The title rides in an HTTP header, which is latin-1: non-ASCII must be
    # stripped here or urllib raises and the push is lost entirely.
    assert subject({"name": "café.service"}) == "caf"
    print("selftest ok")



class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        try:
            n = int(self.headers.get("Content-Length", 0))
            handle(json.loads(self.rfile.read(n) or b"{}"))
            self.send_response(204)
        except Exception as e:
            print(f"handler error: {e}", flush=True)
            self.send_response(500)
        self.end_headers()

    def do_GET(self):  # health
        self.send_response(200); self.end_headers(); self.wfile.write(b"ok")

    def log_message(self, *a):
        pass


if __name__ == "__main__":
    import sys
    if "--selftest" in sys.argv:
        _selftest()
        raise SystemExit(0)
    print(f"alert-receiver listening on :{LISTEN_PORT} -> ntfy {NTFY_URL}", flush=True)
    ThreadingHTTPServer(("0.0.0.0", LISTEN_PORT), Handler).serve_forever()
