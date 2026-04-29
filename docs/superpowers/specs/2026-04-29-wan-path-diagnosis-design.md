# WAN Path Diagnosis — Design Spec

**Date:** 2026-04-29
**Status:** Draft (pending review)
**Goal:** Add observability to localize the *exact* hop responsible when ocean's WAN path degrades, so the next "playback was bad last night" investigation produces a definite answer instead of a probabilistic one.

## Motivation

On 2026-04-26 evening, multiple Plex transcode sessions failed mid-stream with `protocol is shutdown (SSL routines)`, `EventSourceClient` retried 22 times, and `[PlexRelay] Allocated port` appeared every 10–30 minutes. The pattern strongly suggests WAN-side instability — but the existing observability stack couldn't say *whether* it was ocean's NIC, the Unifi gateway, the ISP last mile, ISP backbone, or Plex's edge. Single-target probes show that *something* broke; per-hop probes show *where*.

This spec adds the missing per-hop visibility plus a small set of supporting probes targeted specifically at the Plex network surface area, and pairs them with one alert tier and one dashboard sized for "is it broken? where?" diagnosis.

## Goals

- Detect WAN/Plex-path degradation within 5 minutes of onset.
- On detection, the dashboard reveals which hop(s) on the path are losing packets without further investigation.
- Distinguish "general internet is bad" from "specifically the path to Plex is bad" at alert time.

## Non-goals (explicitly out of scope)

- **Client-side observability** — failures last night had a remote-client component (the SSL shutdowns originated from clients). Probing client-side requires a separate design (e.g., Plex `/status/sessions` watcher).
- **DPI / per-flow analysis** — Unifi's DPI is too shallow; real DPI (`ntopng`, `pmacct`) is a separate project.
- **Rolling pcap on WAN interface** — high value forensically but a different operational profile; revisit if these probes still leave gaps.
- **Shipping Plex logs to Loki** — would enrich the dashboard's bottom row (`PlexRelay` allocation rate from logs) but the dashboard is useful without it. P2.
- **Durable comprehensive network observability** — this is a focused weak-spot diagnosis kit, not a network NMS.

## Architecture

```
                    ┌─────────────────────────────┐
                    │  prometheus (existing)      │
                    └──────────┬──────────────────┘
                       scrape  │
              ┌────────────────┼─────────────────┐
              ▼                ▼                 ▼
      ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
      │ blackbox-    │  │ mtr-exporter │  │ unpoller     │
      │ exporter     │  │ (NEW)        │  │ (existing —  │
      │ (existing,   │  │              │  │  conntrack   │
      │  + new       │  │ czerwonk/    │  │  metrics to  │
      │  modules &   │  │ mtr-exporter │  │  dashboard)  │
      │  targets)    │  │              │  │              │
      └──────┬───────┘  └──────┬───────┘  └──────────────┘
             │                 │
             ▼                 ▼
      external probes    per-hop traceroute
      (icmp, tcp, tls)   (loss/latency by hop)
```

Three legs:
1. **Blackbox** gets two new modules (`icmp_v4`, `tcp_connect`) and probe-targets in `prometheus.yml.j2`. Single-target latency/loss/handshake telemetry.
2. **mtr-exporter** is a new container running `czerwonk/mtr-exporter`, providing per-hop loss/latency metrics for a small set of anchor destinations.
3. **Unpoller** is already deployed; we only need to confirm which conntrack/WAN-port metric names it surfaces and dashboard them.

## Probe targets

### Single-target ICMP — `blackbox` `icmp_v4` module, 30s scrape

| Target | Diagnostic role |
|---|---|
| `192.168.1.1` | Unifi gateway. LAN-side baseline. Loss here = local issue (NIC, cable, switch). |
| `1.1.1.1` | Cloudflare anchor — primary "is the internet OK" baseline. |
| `8.8.8.8` | Google anchor — alternate path; lets us tell ISP route-flap from total internet outage. |
| `plex.tv` | Plex API/control plane. Bad here + good 1.1.1.1 = path-to-Plex problem. |
| `relay.plex.tv` | Plex relay endpoint clients fall back to. |

### TCP connect — `blackbox` `tcp_connect` module, 60s scrape

| Target | Diagnostic role |
|---|---|
| `plex.tv:443` | Confirms TCP handshake works (orthogonal signal to ICMP). |
| `relay.plex.tv:443` | Same, for the relay path. |
| `pubsub04.pop.fmt.plex.bz:443` | The exact pubsub host `EventSourceClient` retried against on 2026-04-26. |
| `1.1.1.1:443` | TCP control. Fine here + failing on plex.tv:443 = Plex's path. |

### TLS handshake — `blackbox` `http_tls` module, 60s scrape

| Target | Diagnostic role |
|---|---|
| `https://plex.tv/` | Full TLS handshake + cert validity. |
| `https://home.terrac.com/` | Own public endpoint via Cloudflare tunnel — sanity check. |

### Per-hop MTR — `mtr-exporter`, 60s per target

| Target | Diagnostic role |
|---|---|
| `1.1.1.1` | Stable Anycast — best baseline for hop-loss patterns. |
| `relay.plex.tv` | The destination that mattered last night. |
| `plex.tv` | Different anycast path; differential vs 1.1.1.1. |

### Frequency / load notes

- 30s ICMP × 5 targets ≈ 0.17 packets/sec total. Negligible.
- 60s MTR × 3 targets, each run probing ~10 hops × 3 packets ≈ 1.5 packets/sec. Below any conceivable ISP rate-limit.
- No client-side probes (out of scope).

## Alert rules

Three new rules added to `files/ocean-prometheus/alert_rules.yml.j2`. The intent is **few alerts that say "something is wrong" + a dashboard that says "where."**

### PlexPathDegraded (critical)

```yaml
- alert: PlexPathDegraded
  expr: |
    probe_success{instance=~".*plex\\.tv.*|.*relay\\.plex\\.tv.*"} == 0
    or
    (1 - avg_over_time(probe_success{job="blackbox-icmp", instance=~".*plex\\.tv.*"}[5m])) > 0.05
  for: 5m
  labels:
    severity: critical
  annotations:
    summary: "Plex path degraded ({{ "{{ $labels.instance }}" }})"
    description: "Probe to {{ "{{ $labels.instance }}" }} is failing or losing >5% for 5m"
```

### WANInternetDegraded (warning)

```yaml
- alert: WANInternetDegraded
  expr: |
    (1 - avg_over_time(probe_success{job="blackbox-icmp", instance=~"1\\.1\\.1\\.1|8\\.8\\.8\\.8"}[5m])) > 0.05
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "WAN internet path degraded ({{ "{{ $labels.instance }}" }})"
    description: "Packet loss to {{ "{{ $labels.instance }}" }} >5% for 5m — internet anchor unhealthy"
```

The two-alert split means a page tells you immediately whether it's "the internet" or "specifically Plex's path" before you even open Grafana.

### ConntrackTablePressure (warning, conditional)

```yaml
- alert: ConntrackTablePressure
  expr: |
    unifi_gateway_conntrack_used / unifi_gateway_conntrack_max > 0.80
  for: 10m
  labels:
    severity: warning
  annotations:
    summary: "Unifi conntrack table >80% on {{ "{{ $labels.instance }}" }}"
    description: "Long-lived TLS sessions may be silently flushed by table evictions"
```

**Conditional:** exact metric names depend on what unpoller exposes. Implementation step: query unpoller's `/metrics`, find the conntrack metrics, pin the names. Fall back to deferring this alert if unpoller doesn't surface conntrack data.

## Dashboard: "WAN Path Diagnosis"

Single new dashboard, structured top-to-bottom from "is it broken?" → "where is it broken?".

1. **Status row** — stat panels: probe success per target (green/red), current loss to anchors (1.1.1.1, plex.tv, relay.plex.tv), current TLS handshake p95 to plex.tv.
2. **Latency / loss time series** — one panel per ICMP target, 24h window, alert annotations overlaid so an active fire shows next to its causal spike.
3. **Per-hop heatmap** *(the diagnostic killshot)* — X = time, Y = hop number, color = loss ratio. One heatmap per MTR destination. Visually shows "hop 4 went red at 22:00."
4. **TLS health** — handshake duration breakdown (`probe_http_duration_seconds{phase="connect"}` + `phase="tls"`) to plex.tv; cert expiry countdown.
5. **Conntrack & WAN port** — table fill %, evictions/sec, WAN-port discards/errors. From unpoller. (Pending metric-name confirmation.)
6. **Plex correlation** — `[PlexRelay] Allocated port` count over time and `EventSourceClient` retry rate. *Requires Plex log shipping to Loki — deferred; row appears empty until then.*

## File changes

### Modify

| File | Change |
|---|---|
| `files/ocean-prometheus/blackbox.yml` | Add `icmp_v4` module (icmp prober) and `tcp_connect` module (tcp prober). |
| `files/ocean-prometheus/prometheus.yml.j2` | Add four scrape jobs: `blackbox-icmp`, `blackbox-tcp`, `blackbox-tls-plex`, `mtr-exporter`. The blackbox jobs use `params.module` + `relabel_configs` so each module probes its target list. |
| `files/ocean-prometheus/alert_rules.yml.j2` | Add `PlexPathDegraded`, `WANInternetDegraded`, `ConntrackTablePressure`. |
| `playbooks/individual/ocean/monitoring/prometheus.yaml` | Add tasks: template `mtr-exporter-config.json` and `mtr-exporter.service`, enable + start service. |
| `vars/vars_service_ports.yaml` | Register `mtr_exporter.port: 9141`. |

### Create

| File | Purpose |
|---|---|
| `files/ocean-prometheus/mtr-exporter.service.j2` | Systemd unit running `czerwonk/mtr-exporter` as a docker container. `--cap-add NET_RAW` required for raw ICMP. Matches existing `ndt-exporter.service.j2` / `blackbox-exporter.service.j2` pattern. |
| `files/ocean-prometheus/mtr-exporter-config.json.j2` | MTR target list (1.1.1.1, relay.plex.tv, plex.tv) with 60s intervals. |
| `files/ocean-prometheus/wan-path-diagnosis-dashboard.json` | Grafana dashboard JSON (the six rows above). |

### Untouched

- `loki`, `promtail` — Plex log shipping is deferred.
- Existing alert rules, existing exporters, existing dashboards.

## Open questions for implementation

1. **mtr-exporter image tag** — `latest` vs pinned. Recommend pinning to a known-good version since `latest` was implicated in the plex-exporter regression earlier this week.
2. **Conntrack metric names** — `unifi_gateway_conntrack_used` / `_max` are placeholders. Implementer queries unpoller's `/metrics` first, pins the real names, and updates the alert + dashboard. If unpoller doesn't expose conntrack, the alert is dropped and a TODO is noted.
3. **Grafana dashboard provisioning mechanism** — confirm whether the existing dashboards under `files/ocean-prometheus/*-dashboard.json` are loaded via Grafana provisioning config or via the import UI; new dashboard follows the same path.

## Implementation phasing (rough — detailed in plan)

1. **Phase 1 — Probes only** (quick win): blackbox modules + scrape jobs + targets. Verify probes produce metrics. No alerts yet.
2. **Phase 2 — MTR exporter**: deploy container, add scrape job, verify per-hop metrics flowing.
3. **Phase 3 — Alerts**: add three alert rules; trigger a test fire (e.g., temporarily blackhole one target).
4. **Phase 4 — Dashboard**: build and publish.
5. **Phase 5 — Conntrack confirmation**: pin unpoller metric names, complete the alert and dashboard row.

## Success criteria

- After deploy, `probe_success{instance=~".*plex\\.tv.*"}` is being scraped at 30s/60s intervals and returns 1 under steady state.
- `mtr_path_hop_loss_ratio` and `mtr_path_hop_rtt_seconds` are exposed for all three MTR destinations across all hops.
- A simulated outage (e.g., temporary iptables drop on ocean for one anchor) fires `PlexPathDegraded` or `WANInternetDegraded` within 5 minutes.
- Dashboard rows 1–4 render with live data; row 5 (conntrack) renders if unpoller has the metrics, otherwise is documented as deferred; row 6 (Plex correlation) is empty pending log shipping.
