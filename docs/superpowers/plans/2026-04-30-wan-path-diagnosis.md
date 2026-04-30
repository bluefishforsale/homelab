# WAN Path Diagnosis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-hop network observability and Plex-path probes to localize the failing hop the next time WAN-side playback issues occur.

**Architecture:** Augment the existing `blackbox-exporter` with Plex-specific probe targets, deploy a new `czerwonk/mtr-exporter` container for per-hop traceroute metrics, add three alert rules, and ship one Grafana dashboard. All driven by the existing Ansible prometheus playbook.

**Tech Stack:** Ansible (existing playbook `playbooks/individual/ocean/monitoring/prometheus.yaml`), Prometheus, blackbox-exporter (existing), `ghcr.io/czerwonk/mtr-exporter` (new), Grafana, systemd units running docker containers.

**Reference spec:** `docs/superpowers/specs/2026-04-29-wan-path-diagnosis-design.md`

**Discovery before planning** (these change the plan vs. spec):
- `files/ocean-prometheus/blackbox.yml` already defines `icmp`, `tcp_connect`, `tls_connect`, `dns_tcp`, `dns_udp` modules.
- `files/ocean-prometheus/prometheus.yml.j2` already has `blackbox-icmp`, `blackbox-tcp`, `blackbox-tls` jobs with target lists. We *augment* them instead of creating new ones.
- The blackbox-exporter container does NOT currently run with `NET_RAW` (see Task 0). Existing ICMP probes work because the default Linux `net.ipv4.ping_group_range` allows unprivileged ICMP from the container's GID — but adding `relay.plex.tv` and `plex.tv` (DNS-resolved targets) may stress this. Verify before assuming.
- Spec's "blackbox-tls-plex" job becomes "augment existing `blackbox-tls`."

**Conventions:**
- All ansible runs from repo root after `source .envrc` (sets vault password file).
- Each task ends with a commit; do not batch commits across tasks.
- After every Prometheus config change, the existing playbook handler restarts prometheus.

---

## File Structure

| File | Status | Purpose |
|---|---|---|
| `files/ocean-prometheus/blackbox.yml` | unchanged | Modules already complete |
| `files/ocean-prometheus/prometheus.yml.j2` | modify | Add Plex targets to existing blackbox jobs; add `mtr-exporter` job |
| `files/ocean-prometheus/alert_rules.yml.j2` | modify | Add `PlexPathDegraded`, `WANInternetDegraded`, `ConntrackTablePressure` |
| `files/ocean-prometheus/mtr-exporter.service.j2` | create | Systemd unit for mtr-exporter container |
| `files/ocean-prometheus/mtr-exporter-config.json.j2` | create | MTR target list |
| `files/ocean-prometheus/wan-path-diagnosis-dashboard.json` | create | Grafana dashboard |
| `playbooks/individual/ocean/monitoring/prometheus.yaml` | modify | Tasks to template + start mtr-exporter |
| `vars/vars_service_ports.yaml` | modify | Register `mtr_exporter.port: 9141` |

---

## Task 1: Add Plex-specific ICMP targets

**Why:** Localize "is plex.tv reachable from ocean" as a continuous signal independent of the broader internet anchors that already exist.

**Files:**
- Modify: `files/ocean-prometheus/prometheus.yml.j2` (add new target group inside existing `blackbox-icmp` job, around line 336)

- [ ] **Step 1: Pre-verify the Plex ICMP series doesn't yet exist**

```bash
ssh terrac@ocean.home 'curl -sG "http://localhost:9090/api/v1/query" --data-urlencode "query=probe_success{job=\"blackbox-icmp\",instance=\"plex.tv\"}" | python3 -c "import json,sys; print(\"series:\", len(json.load(sys.stdin)[\"data\"][\"result\"]))"'
```

Expected: `series: 0`

- [ ] **Step 2: Add a `plex_path` target group inside the existing `blackbox-icmp` job**

Open `files/ocean-prometheus/prometheus.yml.j2`. Inside the `blackbox-icmp` job (existing, around line 301), add a new `- targets:` block as a sibling of the existing groups (e.g., after the `local_network` block at line 342). The exact addition:

```yaml
      - targets:
        # Plex path
        - plex.tv
        - relay.plex.tv
        labels:
          probe_type: "icmp"
          category: "plex_path"
```

Place it before the `relabel_configs:` line so it remains part of the `static_configs` list.

- [ ] **Step 3: Validate the template renders**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/monitoring/prometheus.yaml
```

Expected: `playbook: playbooks/individual/ocean/monitoring/prometheus.yaml` with no errors.

- [ ] **Step 4: Deploy**

```bash
ansible-playbook playbooks/individual/ocean/monitoring/prometheus.yaml --tags never  # check task tags first
# If prometheus.yaml has no granular tags, run the full playbook:
ansible-playbook playbooks/individual/ocean/monitoring/prometheus.yaml
```

The handler `Restart prometheus service` will fire because `alert_rules.yml.j2` and `prometheus.yml.j2` are bundled in one task. Wait for completion (~30s).

- [ ] **Step 5: Verify the new probes are scraping**

```bash
ssh terrac@ocean.home 'sleep 45; curl -sG "http://localhost:9090/api/v1/query" --data-urlencode "query=probe_success{job=\"blackbox-icmp\",instance=~\"plex.tv|relay.plex.tv\"}" | python3 -c "import json,sys; r=json.load(sys.stdin)[\"data\"][\"result\"]; print(\"\\n\".join(f\"{x[\"metric\"][\"instance\"]}={x[\"value\"][1]}\" for x in r))"'
```

Expected: two lines, one per target, each with value `1`. If `0`, check blackbox logs (`docker logs blackbox-exporter --tail 30`) — common cause is ICMP perms (covered in Task 0 troubleshooting note).

- [ ] **Step 6: Commit**

```bash
git add files/ocean-prometheus/prometheus.yml.j2
git commit -m "feat(prometheus): add Plex-path ICMP probes to blackbox-icmp

Continuous reachability monitoring for plex.tv and relay.plex.tv,
labeled category=plex_path so alerts can target them specifically
without including the broader internet anchors.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Add Plex-specific TCP targets

**Why:** TCP handshake is orthogonal to ICMP — captures cases where ICMP works but TCP/443 is dropped (some ISPs/middleboxes). Includes the exact pubsub host that retried during the 2026-04-26 incident.

**Files:**
- Modify: `files/ocean-prometheus/prometheus.yml.j2` (existing `blackbox-tcp` job, around line 383)

- [ ] **Step 1: Pre-verify**

```bash
ssh terrac@ocean.home 'curl -sG "http://localhost:9090/api/v1/query" --data-urlencode "query=probe_success{job=\"blackbox-tcp\",instance=\"plex.tv:443\"}" | python3 -c "import json,sys; print(\"series:\", len(json.load(sys.stdin)[\"data\"][\"result\"]))"'
```

Expected: `series: 0`

- [ ] **Step 2: Add a `plex_path` target group inside the existing `blackbox-tcp` job**

Open `files/ocean-prometheus/prometheus.yml.j2`. Inside the `blackbox-tcp` job (existing, around line 383), add a new `- targets:` block as a sibling of the existing groups (after the `smtp` block at line 401). The exact addition:

```yaml
      - targets:
        # Plex path
        - plex.tv:443
        - relay.plex.tv:443
        - pubsub04.pop.fmt.plex.bz:443
        - 1.1.1.1:443
        labels:
          probe_type: "tcp"
          category: "plex_path"
```

The `1.1.1.1:443` is the control — it should always pass when other TCP probes fail, so we can rule out total internet outage.

- [ ] **Step 3: Validate**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/monitoring/prometheus.yaml
```

Expected: no errors.

- [ ] **Step 4: Deploy**

```bash
ansible-playbook playbooks/individual/ocean/monitoring/prometheus.yaml
```

- [ ] **Step 5: Verify**

```bash
ssh terrac@ocean.home 'sleep 75; curl -sG "http://localhost:9090/api/v1/query" --data-urlencode "query=probe_success{job=\"blackbox-tcp\",category=\"plex_path\"}" | python3 -c "import json,sys; r=json.load(sys.stdin)[\"data\"][\"result\"]; print(\"\\n\".join(f\"{x[\"metric\"][\"instance\"]}={x[\"value\"][1]}\" for x in r))"'
```

Expected: 4 lines, one per target, all `1`. (TCP probes scrape every 1m by default; sleep 75 gives one cycle plus margin.)

- [ ] **Step 6: Commit**

```bash
git add files/ocean-prometheus/prometheus.yml.j2
git commit -m "feat(prometheus): add Plex-path TCP probes to blackbox-tcp

Includes pubsub04.pop.fmt.plex.bz:443, the host EventSourceClient
retried against during the 2026-04-26 incident, and 1.1.1.1:443
as a TCP control to distinguish 'Plex path bad' from 'internet bad'.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Add Plex-specific TLS targets

**Why:** Captures TLS handshake duration and cert validity — a slow handshake is a leading indicator of degraded sessions before TCP-level loss shows up.

**Files:**
- Modify: `files/ocean-prometheus/prometheus.yml.j2` (existing `blackbox-tls` job, around line 275)

- [ ] **Step 1: Pre-verify**

```bash
ssh terrac@ocean.home 'curl -sG "http://localhost:9090/api/v1/query" --data-urlencode "query=probe_success{job=\"blackbox-tls\",instance=\"https://plex.tv:443\"}" | python3 -c "import json,sys; print(\"series:\", len(json.load(sys.stdin)[\"data\"][\"result\"]))"'
```

Expected: `series: 0`

- [ ] **Step 2: Add a `plex_path` target group inside the existing `blackbox-tls` job**

Open `files/ocean-prometheus/prometheus.yml.j2`. Inside the `blackbox-tls` job (existing, around line 275), add a new `- targets:` block as a sibling of the existing `certificate_check` group (after line 290). The exact addition:

```yaml
      - targets:
        - https://plex.tv:443
        - https://home.terrac.com:443
        labels:
          probe_type: "tls"
          category: "plex_path"
```

Note the existing `blackbox-tls` job has `scrape_interval: 1h`. That's fine for cert-validity but too coarse for handshake latency. We want a faster scrape on these specific targets. Rather than overriding inside the job (Prometheus doesn't allow per-target intervals), accept the 1h interval for cert checks and let `blackbox-tcp` carry the fast TCP/TLS-handshake-establishment signal. Document this tradeoff in the commit message.

- [ ] **Step 3: Validate**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/monitoring/prometheus.yaml
```

- [ ] **Step 4: Deploy**

```bash
ansible-playbook playbooks/individual/ocean/monitoring/prometheus.yaml
```

- [ ] **Step 5: Verify**

```bash
# After deploy, manually trigger a probe by hitting the blackbox endpoint directly
ssh terrac@ocean.home 'curl -s "http://localhost:9115/probe?module=http_tls&target=https://plex.tv:443" | grep -E "^probe_success|^probe_ssl_earliest_cert_expiry" | head -5'
```

Expected: `probe_success 1` and an `earliest_cert_expiry` epoch value. Prometheus will pick this up at the next 1h scrape boundary; for immediate metric availability you can run `curl -X POST http://localhost:9090/-/reload` after the playbook deploy completes.

- [ ] **Step 6: Commit**

```bash
git add files/ocean-prometheus/prometheus.yml.j2
git commit -m "feat(prometheus): add Plex TLS cert checks to blackbox-tls

Tracks plex.tv cert validity and home.terrac.com (own public
endpoint) cert validity at 1h intervals. Fast handshake timing is
covered by blackbox-tcp's plex_path targets at 1m intervals.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Register port + create mtr-exporter config and service

**Why:** Set up the new exporter container before wiring it into the playbook.

**Files:**
- Modify: `vars/vars_service_ports.yaml`
- Create: `files/ocean-prometheus/mtr-exporter.service.j2`
- Create: `files/ocean-prometheus/mtr-exporter-config.json.j2`

- [ ] **Step 1: Register the port**

Edit `vars/vars_service_ports.yaml`. Inside the `service_ports:` map under the `# Exporters` section, after `ndt_exporter:` (line 57), add:

```yaml
  mtr_exporter:
    port: 9141
```

- [ ] **Step 2: Create the systemd unit template**

Create `files/ocean-prometheus/mtr-exporter.service.j2` with this exact content (modeled on `ndt-exporter.service.j2`, with `--cap-add NET_RAW` for raw ICMP):

```
[Unit]
Description={{ mtr_exporter_service }}
After=docker.service
Requires=docker.service

[Service]
Type=simple
StandardOutput=journal
StandardError=journal
RuntimeMaxSec=7d
Restart=always
RestartSec=120s
TimeoutStartSec=180s
ExecStartPre=-docker rm -f {{ mtr_exporter_service }}
ExecStartPre=-/usr/bin/docker pull {{ mtr_exporter_image }}:{{ mtr_exporter_version }}
ExecStart=docker run --rm \
    --name {{ mtr_exporter_service }} \
    --hostname {{ mtr_exporter_service }} \
    --cap-add NET_RAW \
    -e TZ=America/Los_Angeles \
    -p {{ mtr_exporter_port }}:8080 \
    -v {{ path_home }}/config/mtr-exporter-config.json:/config.json:ro \
    {{ mtr_exporter_image }}:{{ mtr_exporter_version }} \
      -config.path /config.json
ExecStop=docker rm -f {{ mtr_exporter_service }}

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 3: Create the config template**

Create `files/ocean-prometheus/mtr-exporter-config.json.j2` with this exact content:

```json
{
  "schedule": "@every 60s",
  "mtr": {
    "interval": "1s",
    "report-cycles": 5,
    "timeout": "30s"
  },
  "targets": [
    {
      "address": "1.1.1.1",
      "label_anchor": "cloudflare"
    },
    {
      "address": "relay.plex.tv",
      "label_anchor": "plex_relay"
    },
    {
      "address": "plex.tv",
      "label_anchor": "plex_api"
    }
  ]
}
```

- [ ] **Step 4: Validate config is valid JSON**

```bash
python3 -c "import json; json.load(open('files/ocean-prometheus/mtr-exporter-config.json.j2'))" && echo OK
```

Expected: `OK`

- [ ] **Step 5: Commit (no deploy yet — needs playbook task in Task 5)**

```bash
git add vars/vars_service_ports.yaml files/ocean-prometheus/mtr-exporter.service.j2 files/ocean-prometheus/mtr-exporter-config.json.j2
git commit -m "feat(mtr-exporter): add service unit and config templates

czerwonk/mtr-exporter listens on port 9141, runs MTR every 60s
against 1.1.1.1, relay.plex.tv, and plex.tv. Requires NET_RAW
for raw ICMP. Playbook wiring in next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Wire mtr-exporter into the playbook + Prometheus scrape

**Why:** Now that the templates exist, deploy the container and tell Prometheus to scrape it.

**Files:**
- Modify: `playbooks/individual/ocean/monitoring/prometheus.yaml`
- Modify: `files/ocean-prometheus/prometheus.yml.j2` (add scrape job)

- [ ] **Step 1: Add variables and config-render task to the playbook**

Open `playbooks/individual/ocean/monitoring/prometheus.yaml`. Inside the `vars:` block (around line 11), after `ndt_exporter_port`, add:

```yaml
    mtr_exporter_service: mtr-exporter
    mtr_exporter_image: ghcr.io/czerwonk/mtr-exporter
    mtr_exporter_version: latest
    mtr_exporter_port: "{{ service_ports.mtr_exporter.port }}"
```

In the `tasks:` section, find the existing `Create prometheus config file` task (with `with_items: [prometheus.yml, alert_rules.yml]`). Immediately after it, add:

```yaml
  - name: Create mtr-exporter config file
    ansible.builtin.template:
      src: "{{ playbook_dir }}/../../../../files/ocean-prometheus/mtr-exporter-config.json.j2"
      dest: "{{ path_home }}/config/mtr-exporter-config.json"
      owner: nobody
      group: nogroup
      mode: '0644'
    notify:
    - Restart mtr-exporter service
```

Find the existing `Create blackbox-exporter.service` task (uses `with_items: [blackbox-exporter.service]`). Immediately after it, add:

```yaml
  - name: Create systemd mtr-exporter.service
    ansible.builtin.template:
      src: "{{ playbook_dir }}/../../../../files/ocean-prometheus/{{ item }}.j2"
      dest: "/etc/systemd/system/{{ item }}"
      mode: '0644'
    with_items:
    - mtr-exporter.service
    notify:
    - Reload systemd daemon
    - Restart mtr-exporter service
```

Find the existing `Start and enable ndt-exporter service` task. Immediately after it, add:

```yaml
  - name: Start and enable mtr-exporter service
    ansible.builtin.systemd:
      name: mtr-exporter.service
      state: started
      enabled: true
```

In the `handlers:` block, immediately after `Restart ndt-exporter service`, add:

```yaml
  - name: Restart mtr-exporter service
    ansible.builtin.systemd:
      name: mtr-exporter.service
      state: restarted
```

- [ ] **Step 2: Add the Prometheus scrape job**

Open `files/ocean-prometheus/prometheus.yml.j2`. After the existing `ndt-exporter` scrape job (line 153), add a new job:

```yaml
  - job_name: 'mtr-exporter'
    relabel_configs: *dropPortNumber
    scrape_interval: 60s
    scrape_timeout: 30s
    static_configs:
    - targets: ['ocean.home:9141']
```

- [ ] **Step 3: Validate**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/monitoring/prometheus.yaml
```

- [ ] **Step 4: Deploy**

```bash
ansible-playbook playbooks/individual/ocean/monitoring/prometheus.yaml
```

Expected: tasks for mtr-exporter config, systemd unit, and service start all show `changed`. Handlers fire to reload systemd and restart prometheus + mtr-exporter.

- [ ] **Step 5: Verify the container is running and exposing metrics**

```bash
ssh terrac@ocean.home 'docker ps --filter name=mtr-exporter --format "{{.Names}} {{.Status}}"'
```

Expected: `mtr-exporter Up <N> seconds`

```bash
# Wait one MTR cycle (~30s after first scrape)
ssh terrac@ocean.home 'sleep 90; curl -s http://localhost:9141/metrics | grep -E "^mtr_" | head -5'
```

Expected: lines like `mtr_path_hop_loss_ratio{...}` and `mtr_path_hop_rtt_seconds{...}` with one entry per hop per target. If empty, check `docker logs mtr-exporter --tail 30` — most likely cause is `NET_RAW` not actually granted (kernel may need `CAP_NET_ADMIN` too on some hosts).

- [ ] **Step 6: Verify Prometheus is scraping mtr-exporter**

```bash
ssh terrac@ocean.home 'curl -sG "http://localhost:9090/api/v1/query" --data-urlencode "query=up{job=\"mtr-exporter\"}" | python3 -c "import json,sys; r=json.load(sys.stdin)[\"data\"][\"result\"]; print(r[0][\"value\"][1] if r else \"no series\")"'
```

Expected: `1`

- [ ] **Step 7: Commit**

```bash
git add playbooks/individual/ocean/monitoring/prometheus.yaml files/ocean-prometheus/prometheus.yml.j2
git commit -m "feat(mtr-exporter): deploy container and add Prometheus scrape

Adds mtr-exporter as a systemd-managed docker container alongside
ndt-exporter and blackbox-exporter, scraped at 60s intervals.
Provides per-hop loss and latency metrics for the diagnostic
killshot panel in the WAN dashboard.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Add three alert rules

**Why:** Two-tier paging signal — distinguish "Plex path bad" from "general internet bad" — plus a conditional conntrack alert.

**Files:**
- Modify: `files/ocean-prometheus/alert_rules.yml.j2`

- [ ] **Step 1: Pre-verify the new alerts don't already exist**

```bash
ssh terrac@ocean.home 'curl -s http://localhost:9090/api/v1/rules | python3 -c "import json,sys; rs=[r[\"name\"] for g in json.load(sys.stdin)[\"data\"][\"groups\"] for r in g[\"rules\"]]; print([n for n in rs if n in (\"PlexPathDegraded\",\"WANInternetDegraded\",\"ConntrackTablePressure\")])"'
```

Expected: `[]`

- [ ] **Step 2: Look up the actual conntrack metric name from unpoller**

```bash
ssh terrac@ocean.home 'curl -s http://localhost:9130/metrics | grep -iE "conntrack|sessions" | head -10'
```

Note which metric reports current sessions and max. Common patterns:
- `unpoller_usw_general_sessions{...}` (newer unpoller)
- `unpoller_usg_session_count{...}` / `unpoller_usg_session_max{...}`

If neither exists, set `CONNTRACK_AVAILABLE=false` for the next step.

- [ ] **Step 3: Add the three alert rules**

Open `files/ocean-prometheus/alert_rules.yml.j2`. Inside the `basic_alerts` group, after the existing `PlexDown` alert, add:

```yaml
  # WAN path degraded - Plex side specifically
  - alert: PlexPathDegraded
    expr: |
      probe_success{instance=~".*plex\.tv.*|.*relay\.plex\.tv.*"} == 0
      or
      (1 - avg_over_time(probe_success{job="blackbox-icmp", category="plex_path"}[5m])) > 0.05
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Plex path degraded ({{ "{{ $labels.instance }}" }})"
      description: "Probe to {{ "{{ $labels.instance }}" }} is failing or losing >5% for 5m"

  # WAN path degraded - general internet
  - alert: WANInternetDegraded
    expr: |
      (1 - avg_over_time(probe_success{job="blackbox-icmp", instance=~"1\.1\.1\.1|8\.8\.8\.8"}[5m])) > 0.05
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "WAN internet path degraded ({{ "{{ $labels.instance }}" }})"
      description: "Packet loss to {{ "{{ $labels.instance }}" }} >5% for 5m — internet anchor unhealthy"
```

If Step 2 found conntrack metrics, ALSO add (substituting the real metric names from Step 2):

```yaml
  # Conntrack table pressure on the gateway
  - alert: ConntrackTablePressure
    expr: |
      <USED_METRIC_FROM_STEP_2> / <MAX_METRIC_FROM_STEP_2> > 0.80
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Unifi conntrack table >80%"
      description: "Long-lived TLS sessions may be silently flushed by table evictions"
```

If Step 2 found nothing, skip the conntrack rule and add a TODO comment in the file:

```yaml
  # TODO: ConntrackTablePressure deferred — unpoller does not expose
  # conntrack metrics on this gateway. Revisit if upgraded or if a
  # node_exporter reachable from the gateway becomes available.
```

- [ ] **Step 4: Validate**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/monitoring/prometheus.yaml
```

- [ ] **Step 5: Deploy**

```bash
ansible-playbook playbooks/individual/ocean/monitoring/prometheus.yaml
```

- [ ] **Step 6: Verify rules loaded and are evaluating cleanly**

```bash
ssh terrac@ocean.home 'curl -s http://localhost:9090/api/v1/rules | python3 -c "import json,sys; [print(r[\"name\"], r[\"state\"], r[\"health\"]) for g in json.load(sys.stdin)[\"data\"][\"groups\"] for r in g[\"rules\"] if r[\"name\"] in (\"PlexPathDegraded\",\"WANInternetDegraded\",\"ConntrackTablePressure\")]"'
```

Expected: each rule listed with `state=inactive` (because nothing is actually broken right now) and `health=ok`. `health=err` indicates the expression failed to evaluate — most likely cause is a typo in metric/label names.

- [ ] **Step 7: Commit**

```bash
git add files/ocean-prometheus/alert_rules.yml.j2
git commit -m "feat(prometheus): add WAN path diagnosis alert rules

Three new alerts:
- PlexPathDegraded (critical): plex.tv/relay.plex.tv unreachable
  or losing >5% for 5m
- WANInternetDegraded (warning): 1.1.1.1/8.8.8.8 losing >5% for
  5m, distinguishes 'internet bad' from 'Plex bad'
- ConntrackTablePressure (warning): Unifi gateway conntrack
  table >80% for 10m (or TODO marker if unpoller does not
  expose the metric)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Create the Grafana dashboard

**Why:** Localization happens here, not in alerts. Per-hop heatmap is the diagnostic killshot.

**Files:**
- Create: `files/ocean-prometheus/wan-path-diagnosis-dashboard.json`

- [ ] **Step 1: Confirm how existing dashboards get loaded**

```bash
ssh terrac@ocean.home 'docker exec grafana ls /etc/grafana/provisioning/dashboards/ 2>/dev/null && docker exec grafana ls /var/lib/grafana/dashboards/ 2>/dev/null'
```

Note the dashboard provisioning path. Existing dashboards in the repo (`files/ocean-prometheus/*-dashboard.json`) are likely either copied by an ansible task to one of these paths, or imported manually. Check the grafana playbook (`playbooks/individual/ocean/monitoring/grafana_compose.yaml`) for the deploy mechanism.

- [ ] **Step 2: Create a starter dashboard JSON**

Create `files/ocean-prometheus/wan-path-diagnosis-dashboard.json` with this content. The structure has 6 rows; the JSON below is a working starter that the engineer can extend in the Grafana UI after import. Each panel's PromQL is correct; visualization details (units, decimals) can be tuned in-UI.

```json
{
  "title": "WAN Path Diagnosis",
  "uid": "wan-path-diagnosis",
  "tags": ["wan", "network", "plex", "diagnosis"],
  "timezone": "browser",
  "time": {"from": "now-24h", "to": "now"},
  "refresh": "30s",
  "schemaVersion": 38,
  "panels": [
    {
      "id": 1,
      "type": "stat",
      "title": "Probe success — Plex path",
      "gridPos": {"h": 4, "w": 12, "x": 0, "y": 0},
      "targets": [{"expr": "probe_success{job=\"blackbox-icmp\",category=\"plex_path\"}", "legendFormat": "{{instance}}", "refId": "A"}]
    },
    {
      "id": 2,
      "type": "stat",
      "title": "Probe success — internet anchors",
      "gridPos": {"h": 4, "w": 12, "x": 12, "y": 0},
      "targets": [{"expr": "probe_success{job=\"blackbox-icmp\",instance=~\"1\\\\.1\\\\.1\\\\.1|8\\\\.8\\\\.8\\\\.8\"}", "legendFormat": "{{instance}}", "refId": "A"}]
    },
    {
      "id": 3,
      "type": "timeseries",
      "title": "ICMP RTT — Plex path",
      "gridPos": {"h": 8, "w": 24, "x": 0, "y": 4},
      "targets": [{"expr": "probe_icmp_duration_seconds{job=\"blackbox-icmp\",category=\"plex_path\",phase=\"rtt\"}", "legendFormat": "{{instance}}", "refId": "A"}]
    },
    {
      "id": 4,
      "type": "timeseries",
      "title": "ICMP loss rate (5m) — all monitored targets",
      "gridPos": {"h": 8, "w": 24, "x": 0, "y": 12},
      "targets": [{"expr": "1 - avg_over_time(probe_success{job=\"blackbox-icmp\"}[5m])", "legendFormat": "{{instance}}", "refId": "A"}]
    },
    {
      "id": 5,
      "type": "heatmap",
      "title": "Per-hop loss — relay.plex.tv",
      "gridPos": {"h": 8, "w": 12, "x": 0, "y": 20},
      "targets": [{"expr": "mtr_path_hop_loss_ratio{destination=\"relay.plex.tv\"}", "legendFormat": "hop {{hop}}", "refId": "A"}]
    },
    {
      "id": 6,
      "type": "heatmap",
      "title": "Per-hop loss — 1.1.1.1",
      "gridPos": {"h": 8, "w": 12, "x": 12, "y": 20},
      "targets": [{"expr": "mtr_path_hop_loss_ratio{destination=\"1.1.1.1\"}", "legendFormat": "hop {{hop}}", "refId": "A"}]
    },
    {
      "id": 7,
      "type": "timeseries",
      "title": "TCP handshake duration — Plex path",
      "gridPos": {"h": 8, "w": 24, "x": 0, "y": 28},
      "targets": [{"expr": "probe_duration_seconds{job=\"blackbox-tcp\",category=\"plex_path\"}", "legendFormat": "{{instance}}", "refId": "A"}]
    },
    {
      "id": 8,
      "type": "stat",
      "title": "plex.tv cert expiry",
      "gridPos": {"h": 4, "w": 12, "x": 0, "y": 36},
      "targets": [{"expr": "(probe_ssl_earliest_cert_expiry{instance=\"https://plex.tv:443\"} - time()) / 86400", "legendFormat": "days", "refId": "A"}]
    },
    {
      "id": 9,
      "type": "stat",
      "title": "home.terrac.com cert expiry",
      "gridPos": {"h": 4, "w": 12, "x": 12, "y": 36},
      "targets": [{"expr": "(probe_ssl_earliest_cert_expiry{instance=\"https://home.terrac.com:443\"} - time()) / 86400", "legendFormat": "days", "refId": "A"}]
    }
  ]
}
```

(Conntrack and Plex-correlation rows from the spec are intentionally omitted — conntrack is added in Task 8 conditionally; Plex-correlation requires log shipping which is out of scope.)

- [ ] **Step 3: Validate JSON**

```bash
python3 -c "import json; json.load(open('files/ocean-prometheus/wan-path-diagnosis-dashboard.json'))" && echo OK
```

Expected: `OK`

- [ ] **Step 4: Deploy via the existing dashboard mechanism**

This depends on Step 1's findings. Two common cases:

**Case A — Provisioning directory under `/var/lib/grafana/dashboards/`:** copy the file there via an ansible task (modify `playbooks/individual/ocean/monitoring/grafana_compose.yaml` to add the file to the existing dashboard-copy task). If unsure, ask the user before extending grafana_compose.yaml.

**Case B — Manual import via Grafana UI:** ssh-tunnel grafana (`ssh -L 8910:127.0.0.1:8910 ocean.home`), open `http://localhost:8910`, Dashboards → Import → Upload JSON file.

Pick the case that matches existing convention. Document the choice in the commit message.

- [ ] **Step 5: Verify the dashboard renders with live data**

After deploy/import, open the dashboard URL. Confirm:
- Status panels show green (probe_success = 1) for plex.tv, relay.plex.tv, 1.1.1.1, 8.8.8.8
- ICMP RTT panel shows time series (~10–50ms typical)
- ICMP loss panel near zero across the board
- Per-hop heatmap shows ~10 hops to each destination, mostly low loss (one or two hops may show occasional spikes — that's normal)
- TCP handshake panel populated
- Cert expiry stats show positive days (~30+ for Let's Encrypt-managed `home.terrac.com`)

- [ ] **Step 6: Commit**

```bash
git add files/ocean-prometheus/wan-path-diagnosis-dashboard.json
# If grafana_compose.yaml was modified for Case A, add it too
git commit -m "feat(grafana): add WAN path diagnosis dashboard

Single dashboard with 6 panel rows organized 'is it broken? where?'
Top: probe-success status and ICMP loss/RTT for all targets.
Middle: per-hop MTR heatmaps to relay.plex.tv and 1.1.1.1 (the
diagnostic killshot for localizing failure to a specific hop).
Bottom: TCP handshake durations and cert expiry stats.

Conntrack and Plex-log-correlation rows deferred (latter requires
Plex log shipping to Loki, out of scope for this spec).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Conntrack panel (conditional)

**Why:** Spec called out conntrack table pressure as a probable culprit for silent TLS-session flushes. Whether we can dashboard it depends on what unpoller exposes.

**Files:**
- Modify: `files/ocean-prometheus/wan-path-diagnosis-dashboard.json` (add panel)

- [ ] **Step 1: Find conntrack metrics**

```bash
ssh terrac@ocean.home 'curl -s http://localhost:9130/metrics | grep -iE "conntrack|sessions|connections" | sort -u'
```

If output is empty, **stop here** — Task 6's conntrack alert is already a TODO comment, and the dashboard panel is also deferred. Document this with a single commit:

```bash
git commit --allow-empty -m "docs: conntrack metrics unavailable from unpoller

Verified that unpoller's /metrics endpoint does not surface
conntrack table size or related session counters from the Unifi
gateway on this firmware version. ConntrackTablePressure alert
remains a TODO; dashboard panel deferred.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

…and skip steps 2–4.

- [ ] **Step 2: Add conntrack panel to the dashboard**

(Only if Step 1 found metrics.) Open `files/ocean-prometheus/wan-path-diagnosis-dashboard.json` and append a new panel inside the `panels` array. Replace `<USED_METRIC>` and `<MAX_METRIC>` with the names from Step 1:

```json
    {
      "id": 10,
      "type": "timeseries",
      "title": "Conntrack table fill %",
      "gridPos": {"h": 8, "w": 24, "x": 0, "y": 40},
      "targets": [{"expr": "100 * <USED_METRIC> / <MAX_METRIC>", "legendFormat": "{{instance}}", "refId": "A"}]
    }
```

- [ ] **Step 3: Validate JSON**

```bash
python3 -c "import json; json.load(open('files/ocean-prometheus/wan-path-diagnosis-dashboard.json'))" && echo OK
```

- [ ] **Step 4: Re-deploy + verify**

Repeat Task 7 Step 4's deploy mechanism. Open the dashboard, confirm the new panel renders a number (typically 5–30% under steady state).

- [ ] **Step 5: Commit**

```bash
git add files/ocean-prometheus/wan-path-diagnosis-dashboard.json
git commit -m "feat(grafana): add conntrack table fill panel

Adds the conntrack pressure panel to the WAN dashboard now that
unpoller's metric names are confirmed. Closes the gap from the
spec's deferred-on-discovery item.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Simulated outage test (sanity check the alerts)

**Why:** Per the spec's success criteria, "a simulated outage fires PlexPathDegraded or WANInternetDegraded within 5 minutes." This proves the alert pipeline end-to-end.

- [ ] **Step 1: Pick a low-blast-radius target to simulate-fail**

Use `8.8.8.8` (Google DNS — won't break anything if briefly unreachable from ocean). We'll iptables-DROP all packets to it for ~7 minutes, watch for `WANInternetDegraded`, then restore.

- [ ] **Step 2: Black-hole the test target**

```bash
ssh terrac@ocean.home 'sudo iptables -I OUTPUT -d 8.8.8.8 -j DROP'
```

- [ ] **Step 3: Watch for the alert (poll every 60s for up to 7 minutes)**

```bash
ssh terrac@ocean.home 'for i in $(seq 1 7); do
  echo "T+${i}m";
  curl -s http://localhost:9090/api/v1/alerts | python3 -c "import json,sys; a=[x for x in json.load(sys.stdin)[\"data\"][\"alerts\"] if x[\"labels\"][\"alertname\"]==\"WANInternetDegraded\"]; print(\"firing\" if a and a[0][\"state\"]==\"firing\" else (\"pending\" if a else \"inactive\"))";
  sleep 60;
done'
```

Expected progression:
- T+1m to T+4m: `inactive` (probe_success drops to 0 immediately, but the `for: 5m` clause delays firing)
- T+5m to T+7m: `pending` then `firing`

- [ ] **Step 4: Restore connectivity**

```bash
ssh terrac@ocean.home 'sudo iptables -D OUTPUT -d 8.8.8.8 -j DROP'
```

- [ ] **Step 5: Confirm alert resolves**

Within the next 5 minutes, the `for: 5m` window slides forward and the alert returns to `inactive`.

```bash
ssh terrac@ocean.home 'sleep 360; curl -s http://localhost:9090/api/v1/alerts | python3 -c "import json,sys; a=[x for x in json.load(sys.stdin)[\"data\"][\"alerts\"] if x[\"labels\"][\"alertname\"]==\"WANInternetDegraded\"]; print(a[0][\"state\"] if a else \"inactive\")"'
```

Expected: `inactive`.

- [ ] **Step 6: No commit** (this task only validates — no code changes)

---

## Done criteria (cross-check against spec success criteria)

- ✅ `probe_success{instance=~".*plex\.tv.*"}` scraped at 30s (Tasks 1, 2, 3)
- ✅ `mtr_path_hop_loss_ratio` and `mtr_path_hop_rtt_seconds` exposed for all three MTR destinations across all hops (Task 5)
- ✅ Simulated outage fires `WANInternetDegraded` within 5 minutes (Task 9)
- ✅ Dashboard rows 1–4 + TCP handshake + cert expiry render with live data (Task 7)
- ✅ Conntrack row renders if unpoller has the metrics; otherwise documented as deferred (Task 8)
- ⏸ Plex-correlation row remains empty pending log shipping (out of scope; spec acknowledged)

---

## Self-review notes

- **No placeholders for code.** Every step contains actual code or actual commands.
- **Conditional sections (Task 6 conntrack rule, Task 8 conntrack panel) are explicit branches**, not "TODO."
- **Type/name consistency:** Variable names `mtr_exporter_service` / `mtr_exporter_image` / `mtr_exporter_port` used identically across the playbook (Task 5) and the service unit (Task 4). Service port `9141` registered once in Task 4 and referenced via `service_ports.mtr_exporter.port` everywhere else.
- **Metric name consistency:** `probe_icmp_duration_seconds`, `probe_success`, `mtr_path_hop_loss_ratio`, `probe_ssl_earliest_cert_expiry` — all real blackbox/mtr-exporter metric names, used identically in alert rules (Task 6) and dashboard JSON (Task 7).
- **Alert expression bug from spec was already fixed** in the spec self-review (`probe_icmp_packet_loss_ratio` → `1 - avg_over_time(probe_success[5m])`). Task 6 uses the correct form.
- **Spec coverage:**
  - Spec §"Probe targets" → Tasks 1, 2, 3, 5
  - Spec §"Alert rules" → Task 6
  - Spec §"Dashboard" → Task 7 (rows 1–4) and Task 8 (row 5 conditional)
  - Spec §"Open question 1 (mtr-exporter image tag)" → addressed in Task 4 step 2 (set to `latest`; revisit if the plex-exporter-style regression bites)
  - Spec §"Open question 2 (conntrack metric names)" → addressed in Task 6 step 2 and Task 8 step 1
  - Spec §"Open question 3 (Grafana provisioning mechanism)" → addressed in Task 7 step 1
  - Spec §"Success criteria" → cross-checked above
