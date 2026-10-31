# WAN path diagnosis

Probes that tell "the internet is bad" apart from "the path to Plex is bad", and
per-hop MTR that shows which hop is losing packets. Built after the 2026-04-26
evening of Plex sessions dropping with nothing able to say where. It is a
blackbox job pair, an mtr-exporter container on ocean, two alerts and one
dashboard; everything rides the existing Prometheus on ocean.

## Playbooks and files

| Path | What it owns |
|---|---|
| `playbooks/individual/ocean/monitoring/prometheus.yaml` | Renders `mtr-exporter-jobs.txt` to `/data01/services/prometheus/config/`, installs and enables `mtr-exporter.service` (image `dragoonaethis/mtr-exporter:0.3.0`). Same play owns Prometheus, blackbox and the alert rules. |
| `files/ocean-prometheus/mtr-exporter.service.j2` | systemd unit, bare `docker run --rm --init --cap-add NET_RAW`, host port `service_ports.mtr_exporter.port` to container 8080, `RuntimeMaxSec=7d`, pulls the pinned tag on every start. |
| `files/ocean-prometheus/mtr-exporter-jobs.txt.j2` | Three MTR jobs, `@every 60s`, `--no-dns --report-cycles 5`: `cloudflare` (1.1.1.1, ICMP), `google` (8.8.8.8, ICMP), `plex_api` (plex.tv, `--tcp --port 443`). |
| `files/ocean-prometheus/prometheus.yml.j2` | Jobs `mtr-exporter` (`ocean.home:9141`, 60s), `blackbox-tcp-plex` (module `tcp_connect_ip4`, 1m: `plex.tv:443`, `1.1.1.1:443`), `blackbox-tls-plex` (module `http_tls_ip4`, 1h: `https://plex.tv:443`). Plex targets carry `category: plex_path`. The internet anchors come from the pre-existing `blackbox-icmp` job (module `icmp`, 30s). |
| `files/ocean-prometheus/blackbox.yml` | Modules used: `icmp`, `tcp_connect_ip4`, `http_tls_ip4` (both `preferred_ip_protocol: ip4`, `ip_protocol_fallback: false`). |
| `files/ocean-prometheus/alert_rules.yml.j2` | `PlexPathDegraded`, `WANInternetDegraded`. |
| `files/grafana-compose/dashboards/wan-path-diagnosis.json` | "WAN Path Diagnosis" (uid `wan-path-diagnosis`), provisioned by `grafana_compose.yaml` copying the dashboards dir. |
| `vars/vars_service_ports.yaml` | `mtr_exporter.port: 9141`. |

## Gotchas

- NOTE: Plex probes are IPv4-forced. plex.tv is dual-stack and ocean has no
  IPv6 uplink, so a fallback-enabled module would report a broken v6 path as a
  Plex outage.
- plex.tv sits on AWS and drops ICMP, so the Plex path is probed by TCP/443
  (blackbox and MTR `--tcp`), never ICMP.
- NOTE: only stable published endpoints belong in the Plex targets. The incident's
  pubsub node `pubsub04.pop.fmt.plex.bz` was removed (#175): Plex assigns
  pubsub POPs dynamically, so a hardcoded node eventually hard-fails and pages
  a permanent false `PlexPathDegraded` critical.
- `blackbox-tls-plex` scrapes hourly, longer than Prometheus's 5m staleness
  window, so instant queries usually return nothing. On 2026-10-30
  `probe_ssl_earliest_cert_expiry{job="blackbox-tls-plex"}` was empty while
  `last_over_time(probe_success{job="blackbox-tls-plex"}[2h])` was 1. The
  dashboard's cert-expiry stat is blank most of the hour.
- The scrape target `ocean.home:9141` is a literal in `prometheus.yml.j2`, not
  the port var. Changing `mtr_exporter.port` moves the container and silently
  breaks the scrape.
- mtr-exporter is a bare `docker run` unit, not a compose project, so
  `docker-refresh@` never covers it. The tag is pinned, so updates mean bumping
  `mtr_exporter_version` in the playbook.

## Alerts

| Alert | Expr (summary) | for | Severity |
|---|---|---|---|
| `PlexPathDegraded` | `probe_success{job="blackbox-tcp-plex", instance!="1.1.1.1:443"} == 0`, or loss over `[30m]` > 0.15 | 10m | critical |
| `WANInternetDegraded` | ICMP loss to `1.1.1.1` or `8.8.8.8` in `blackbox-icmp` over `[10m]` > 0.15 | 10m | warning |

`1.1.1.1:443` sits in the Plex TCP job as the control and is excluded from the
Plex alert, so internet-wide loss pages only the warning.

## Metrics and dashboard

MTR series are `mtr_report_avg` and `mtr_report_loss`, keyed by
`mtr_exporter_job` (`cloudflare`, `google`, `plex_api`). The hop number is the
`count` label; `hop` is only `first`/`intermediate`/`last`. Loss is a percent
(0 to 100), not a ratio. Silent hops show `host="???"` at 100 loss in steady
state, so loss on an intermediate hop that recovers downstream is noise. Live on
2026-10-30: `up{job="mtr-exporter"}` 1, hop series 20/20/29 for
cloudflare/google/plex_api, both `blackbox-tcp-plex` targets at 1.

Dashboard panels: Plex path probe success, internet anchor probe success, ICMP
loss rate (anchors plus gateway `192.168.1.1`), ICMP RTT, TCP/443 success for
the Plex path, per-hop avg RTT and per-hop loss for `cloudflare` and `plex_api`
(no `google` panels), plex.tv cert expiry, plex.tv TCP handshake duration.

## Divergences from the spec

Spec named `czerwonk/mtr-exporter` with a JSON config; deployed is
`dragoonaethis/mtr-exporter` 0.3.0 with a jobs.txt. Spec alerted on 5% loss over
5m (ICMP for Plex); code alerts on 15% (TCP for Plex) over 30m/10m windows with
`for: 10m`, so detection is slower than the spec's 5-minute goal. Spec added
modules to the shared `blackbox-*` jobs; code uses separate `-plex` jobs and an
added `google` MTR job.

## Not built

- ICMP probes to `plex.tv` and `relay.plex.tv`, and anything at all for
  `relay.plex.tv` (no MTR, TCP or TLS target).
- The pubsub node probe (dropped on purpose, #175).
- TLS probe of the own public endpoint through the Cloudflare tunnel.
- `ConntrackTablePressure` and the conntrack/WAN-port dashboard row. Deferred in
  `alert_rules.yml.j2`; unpoller is scraped (`up{job="unpoller"}` 1) but exposes
  no conntrack series.
- Per-hop heatmaps (time series panels instead) and the TLS phase breakdown
  (`probe_http_duration_seconds` is not on the dashboard).
- The Plex correlation row and Plex log shipping to Loki.

## Backout

Remove the two alerts, the three scrape jobs, the jobs file and unit template,
the mtr-exporter tasks in `prometheus.yaml`, the dashboard and the port entry.
The play does not stop or delete the unit, so also `systemctl disable --now
mtr-exporter` on ocean through a one-off task. Nothing else depends on these
series.
