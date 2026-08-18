#!/usr/bin/env python3
"""agentbox alert receiver — the Alertmanager -> (ntfy + GitHub issues) bridge.

Rendered from files/agentbox/alert-receiver.py by the agentbox playbook.

Alertmanager POSTs its webhook JSON here. For each FIRING alert we:
  - push a notification to ntfy so a human is aware (all severities);
  - open a GitHub issue on the infra repo so it becomes trackable/fixable —
    critical alerts get `needs-claude` (premium lane / drive via RC), others get
    a plain issue. Resolved alerts close the matching tracking issue (by
    fingerprint) and push a "resolved" ntfy — completing the fire->fix->close loop.

Deliberately dependency-free (stdlib only) and single-file: it reads config from
the environment (rendered into the systemd unit) and shells out to `gh`.
Idempotent on issues: an open issue carrying the alert fingerprint is reused.
"""
import base64
import json
import os
import subprocess
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

NTFY_URL = os.environ["ALERT_NTFY_URL"]           # e.g. http://ntfy.home:8090/homelab
ISSUE_REPO = os.environ["ALERT_ISSUE_REPO"]       # e.g. bluefishforsale/homelab
LISTEN_PORT = int(os.environ.get("ALERT_LISTEN_PORT", "9098"))
# Critical alerts kick an immediate premium-lane remediation. Set 0 to disable
# unattended infra edits (issue + ntfy still fire).
REMEDIATE = os.environ.get("ALERT_REMEDIATE", "1") == "1"
RC_URL = os.environ.get("ALERT_RC_URL", "https://claude.ai/code")  # live RC console
ESCALATE = "/usr/local/bin/agentbox-escalate-to-claude.sh"
REPO_NAME = ISSUE_REPO.split("/")[-1]
# ntfy is deny-all (public via ntfy.terrac.com); publish with basic auth.
NTFY_USER = os.environ.get("ALERT_NTFY_USER", "")
NTFY_PASS = os.environ.get("ALERT_NTFY_PASS", "")
FP_MARKER = "alert-fp:"  # embedded in the issue body to dedupe by fingerprint
COMP_MARKER = "replace-component:"  # dedupe hardware issues by physical drive
HW_LABEL = "hardware"
RUNBOOK_URL = ("https://github.com/bluefishforsale/homelab/blob/master/"
               "docs/operations/zfs-data01-disk-replacement-runbook.md")

PRIO = {"critical": "urgent", "warning": "high", "info": "default"}


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


def close_issue(alert):
    """Resolve half of the loop: close the open tracking issue(s) carrying this
    alert's fingerprint when Alertmanager reports it resolved."""
    fp = alert.get("fingerprint", alert["labels"].get("alertname", "unknown"))
    r = gh("issue", "list", "--repo", ISSUE_REPO, "--state", "open",
           "--search", f"{FP_MARKER}{fp} in:body", "--json", "number")
    if r.returncode != 0:
        print(f"gh issue list failed (close): {r.stderr.strip()}", flush=True)
        return
    try:
        nums = [i["number"] for i in json.loads(r.stdout or "[]")]
    except json.JSONDecodeError:
        return
    for num in nums:
        c = gh("issue", "close", str(num), "--repo", ISSUE_REPO,
               "--reason", "completed",
               "--comment", "Alert resolved in Alertmanager; auto-closing.")
        print((c.stdout or c.stderr).strip(), flush=True)


def open_issue(alert):
    """Create (or reuse) the tracking issue; return its number, or None if it
    already exists / creation failed."""
    labels = alert["labels"]
    ann = alert.get("annotations", {})
    fp = alert.get("fingerprint", labels.get("alertname", "unknown"))
    if issue_exists(fp):
        return None
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
    out = (r.stdout or r.stderr).strip()
    print(out, flush=True)
    if r.returncode != 0:
        return None
    try:
        return int(out.rsplit("/", 1)[-1])  # gh prints the new issue URL
    except ValueError:
        return None


def find_open_issue(marker):
    """Number of an open issue whose body carries `marker`, else None. Uses the
    REST list endpoint (read-after-write consistent) rather than
    `--search ... in:body` — the search index lags issue creation by seconds and
    re-mints duplicates during an alert burst. Returns "error" on gh failure so
    callers fail closed (skip creation) instead of spamming."""
    r = gh("issue", "list", "--repo", ISSUE_REPO, "--state", "open",
           "--limit", "500", "--json", "number,body")
    if r.returncode != 0:
        print(f"gh issue list failed: {r.stderr.strip()}", flush=True)
        return "error"
    try:
        for i in json.loads(r.stdout or "[]"):
            if marker in (i.get("body") or ""):
                return i["number"]
    except json.JSONDecodeError:
        return "error"
    return None


def open_replace_issue(alert):
    """Collapse every disk/ZFS alert for one physical drive into a single
    [replace] issue keyed on the device (across alertname AND error-type). A
    failing drive needs hands, never a PR — this issue never carries
    needs-claude and is never auto-closed; a human closes it after the swap.
    Returns the issue number (existing or new), or None for a device-less
    hardware alert (e.g. pool-level ZpoolDegraded) -> ntfy-only."""
    labels = alert["labels"]
    dev = labels.get("device")
    inst = labels.get("instance", labels.get("job", ""))
    if not dev:
        return None
    marker = f"{COMP_MARKER}{inst}/{dev}"
    found = find_open_issue(marker)
    if found is not None:  # existing issue OR gh error -> never mint a duplicate
        return found if isinstance(found, int) else None
    ann = alert.get("annotations", {})
    pool = labels.get("pool", "")
    title = f"[replace] failing drive {dev}{(' on ' + inst) if inst else ''}"
    body = (f"A physical drive is failing and needs replacement. This issue "
            f"collapses every disk/ZFS alert for **{dev}**"
            f"{(' in pool ' + pool) if pool else ''}"
            f"{(' on ' + inst) if inst else ''}.\n\n"
            f"- **Component:** {dev}\n- **Pool:** {pool}\n"
            f"- **First symptom:** {labels.get('alertname', 'alert')} — "
            f"{ann.get('summary', '')}\n\n"
            f"Replacement is a hand-run, never-automated task — follow the "
            f"[disk-replacement runbook]({ann.get('runbook_url') or RUNBOOK_URL}). "
            f"Close this after the drive is swapped and the pool resilvers.\n\n"
            f"<!-- {marker} -->")
    gh("label", "create", HW_LABEL, "--repo", ISSUE_REPO,
       "--color", "B60205", "--force")  # idempotent
    r = gh("issue", "create", "--repo", ISSUE_REPO, "--title", title,
           "--body", body, "--label", HW_LABEL)
    out = (r.stdout or r.stderr).strip()
    print(out, flush=True)
    if r.returncode != 0:
        return None
    try:
        return int(out.rsplit("/", 1)[-1])
    except ValueError:
        return None


def remediate(num):
    """Kick an immediate premium-lane fix for a critical alert. Detached so a
    multi-minute claude run never blocks the HTTP handler; it only ever opens a
    PR (never auto-merges infra), so a human still gates the change.
    ponytail: child dies if the receiver service restarts mid-run (cgroup kill);
    acceptable — the needs-claude issue survives and the periodic drain retries.
    """
    if not os.path.exists(ESCALATE):
        print(f"escalate script missing: {ESCALATE}", flush=True)
        return
    print(f"remediating critical alert via issue #{num}", flush=True)
    subprocess.Popen([ESCALATE, REPO_NAME, str(num)], start_new_session=True)


def handle(payload):
    for alert in payload.get("alerts", []):
        labels = alert["labels"]
        sev = labels.get("severity", "warning")
        name = labels.get("alertname", "alert")
        inst = labels.get("instance", labels.get("job", ""))
        summary = alert.get("annotations", {}).get("summary", "")
        # Hardware alerts (remediation: manual) never enter the code lane — a
        # failing disk needs hands, not a PR. They get ntfy + one per-drive
        # [replace] issue, and are never auto-closed.
        manual = labels.get("remediation") == "manual"
        # NB: emoji must go in Tags (ntfy renders them), never the Title header —
        # HTTP headers are latin-1 and non-ASCII there raises.
        if alert.get("status") == "firing":
            if manual:
                num = open_replace_issue(alert)
                issue_url = f"https://github.com/{ISSUE_REPO}/issues/{num}" if num else ""
                ntfy(f"{name} ({sev})",
                     "\n".join(x for x in (inst, summary, issue_url) if x),
                     PRIO.get(sev, "default"), "wrench,homelab", click=issue_url)
                continue
            num = open_issue(alert)
            issue_url = f"https://github.com/{ISSUE_REPO}/issues/{num}" if num else ""
            body = "\n".join(x for x in (inst, summary, issue_url) if x)
            # Critical: one-tap into the live RC console to supervise; warning:
            # tap opens the tracking issue.
            ntfy(f"{name} ({sev})", body, PRIO.get(sev, "default"),
                 "rotating_light,homelab",
                 click=RC_URL if sev == "critical" else issue_url)
            if num and sev == "critical" and REMEDIATE:
                remediate(num)
        else:
            if manual:
                # A replace issue's lifecycle is the physical swap, not the
                # alert flapping; a human closes it. Still ntfy the all-clear.
                ntfy(f"cleared: {name}", f"{inst}\n{summary}".strip(),
                     "min", "white_check_mark,homelab")
                continue
            close_issue(alert)
            ntfy(f"resolved: {name}", f"{inst}\n{summary}".strip(),
                 "min", "white_check_mark,homelab")


def _selftest():
    """Network-free defect ledger for the hardware-alert path. Stubs `gh`."""
    import types
    global gh
    calls = []

    def fake_list_empty(*args):
        calls.append(args)
        if args[:2] == ("issue", "list"):
            return types.SimpleNamespace(returncode=0, stdout="[]", stderr="")
        if args[:2] == ("issue", "create"):
            return types.SimpleNamespace(
                returncode=0, stdout="https://github.com/x/y/issues/42", stderr="")
        return types.SimpleNamespace(returncode=0, stdout="", stderr="")

    gh = fake_list_empty
    # device-less hardware alert (pool-level) -> ntfy only, never touches gh
    assert open_replace_issue({"labels": {"remediation": "manual", "pool": "data01"}}) is None
    assert calls == [], "device-less alert must not create an issue"
    # device-bearing -> exactly one [replace] issue, keyed on instance/device
    n = open_replace_issue({"labels": {"remediation": "manual", "device": "sde",
                                       "instance": "ocean.home", "pool": "data01"},
                            "annotations": {"summary": "s"}})
    assert n == 42, n
    created = [c for c in calls if c[:2] == ("issue", "create")]
    assert len(created) == 1, created
    args = created[0]
    body = args[args.index("--body") + 1]
    assert f"{COMP_MARKER}ocean.home/sde" in body, "marker must key on instance/device"
    assert "needs-claude" not in " ".join(args), "hardware issue must never get needs-claude"
    # a second alert for the same drive reuses the issue (consistent-list dedup)
    calls.clear()

    def fake_list_hit(*args):
        calls.append(args)
        if args[:2] == ("issue", "list"):
            return types.SimpleNamespace(returncode=0, stderr="", stdout=json.dumps(
                [{"number": 42, "body": f"x {COMP_MARKER}ocean.home/sde y"}]))
        return types.SimpleNamespace(returncode=0, stdout="", stderr="")

    gh = fake_list_hit
    assert open_replace_issue({"labels": {"remediation": "manual", "device": "sde",
                                          "instance": "ocean.home"},
                               "annotations": {}}) == 42
    assert not any(c[:2] == ("issue", "create") for c in calls), "must reuse, not recreate"
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
    print(f"alert-receiver listening on :{LISTEN_PORT} -> ntfy {NTFY_URL}, issues {ISSUE_REPO}", flush=True)
    ThreadingHTTPServer(("0.0.0.0", LISTEN_PORT), Handler).serve_forever()
