#!/usr/bin/env python3
"""agentbox alert receiver — the Alertmanager -> (ntfy + GitHub issues) bridge.

Rendered from files/agentbox/alert-receiver.py by the agentbox playbook.

Alertmanager POSTs its webhook JSON here. For each FIRING alert we:
  - push a notification to ntfy so a human is aware (all severities);
  - open a GitHub issue on the infra repo so it becomes trackable/fixable —
    critical alerts get `needs-claude` (premium lane / drive via RC), others get
    a plain issue. Resolved alerts just push a "resolved" ntfy.

Deliberately dependency-free (stdlib only) and single-file: it reads config from
the environment (rendered into the systemd unit) and shells out to `gh`.
Idempotent on issues: an open issue carrying the alert fingerprint is reused.
"""
import json
import os
import subprocess
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

NTFY_URL = os.environ["ALERT_NTFY_URL"]           # e.g. http://ntfy.home:8090/homelab
ISSUE_REPO = os.environ["ALERT_ISSUE_REPO"]       # e.g. bluefishforsale/homelab
LISTEN_PORT = int(os.environ.get("ALERT_LISTEN_PORT", "9098"))
FP_MARKER = "alert-fp:"  # embedded in the issue body to dedupe by fingerprint

PRIO = {"critical": "urgent", "warning": "high", "info": "default"}


def ntfy(title, message, priority="default", tags=""):
    req = urllib.request.Request(
        NTFY_URL, data=message.encode(),
        headers={"Title": title, "Priority": priority, "Tags": tags})
    try:
        urllib.request.urlopen(req, timeout=10).read()
    except Exception as e:  # awareness is best-effort; never crash the receiver
        print(f"ntfy push failed: {e}", flush=True)


def gh(*args):
    return subprocess.run(["gh", *args], capture_output=True, text=True, timeout=60)


def issue_exists(fp):
    r = gh("issue", "list", "--repo", ISSUE_REPO, "--state", "open",
           "--search", f"{FP_MARKER}{fp} in:body", "--json", "number")
    if r.returncode != 0:
        print(f"gh issue list failed: {r.stderr.strip()}", flush=True)
        return True  # on error, assume it exists so we don't spam duplicates
    try:
        return len(json.loads(r.stdout or "[]")) > 0
    except json.JSONDecodeError:
        return True


def open_issue(alert):
    labels = alert["labels"]
    ann = alert.get("annotations", {})
    fp = alert.get("fingerprint", labels.get("alertname", "unknown"))
    if issue_exists(fp):
        return
    sev = labels.get("severity", "warning")
    name = labels.get("alertname", "alert")
    inst = labels.get("instance", labels.get("job", ""))
    title = f"[alert] {name}{(' on ' + inst) if inst else ''}"
    body = (f"Fired by Alertmanager.\n\n"
            f"- **Alert:** {name}\n- **Severity:** {sev}\n- **Instance:** {inst}\n"
            f"- **Summary:** {ann.get('summary', '')}\n"
            f"- **Description:** {ann.get('description', '')}\n\n"
            f"<!-- {FP_MARKER}{fp} -->")
    args = ["issue", "create", "--repo", ISSUE_REPO, "--title", title, "--body", body]
    if sev == "critical":
        gh("label", "create", "needs-claude", "--repo", ISSUE_REPO,
           "--color", "5319E7", "--force")  # idempotent; ignore result
        args += ["--label", "needs-claude"]
    r = gh(*args)
    print((r.stdout or r.stderr).strip(), flush=True)


def handle(payload):
    for alert in payload.get("alerts", []):
        labels = alert["labels"]
        sev = labels.get("severity", "warning")
        name = labels.get("alertname", "alert")
        inst = labels.get("instance", labels.get("job", ""))
        summary = alert.get("annotations", {}).get("summary", "")
        # NB: emoji must go in Tags (ntfy renders them), never the Title header —
        # HTTP headers are latin-1 and non-ASCII there raises.
        if alert.get("status") == "firing":
            ntfy(f"{name} ({sev})", f"{inst}\n{summary}".strip(),
                 PRIO.get(sev, "default"), "rotating_light,homelab")
            open_issue(alert)
        else:
            ntfy(f"resolved: {name}", f"{inst}\n{summary}".strip(),
                 "min", "white_check_mark,homelab")


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
    print(f"alert-receiver listening on :{LISTEN_PORT} -> ntfy {NTFY_URL}, issues {ISSUE_REPO}", flush=True)
    ThreadingHTTPServer(("0.0.0.0", LISTEN_PORT), Handler).serve_forever()
