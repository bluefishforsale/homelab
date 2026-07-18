#!/usr/bin/env bash
# homelab-health.sh — comprehensive fleet health gate, by diff.
#
# A deploy can break things it never touched (shared config, a restarted dependency,
# DNS). So this does NOT enumerate the changed service — it asks the monitoring stack
# about the WHOLE fleet and reports what got WORSE across the deploy:
#   • Prometheus  up==0                 → any scrape target down (every service
#                                          exporter + every host's node_exporter)
#   • Alertmanager active alerts        → anything the fleet's own rules call broken
#   • cadvisor    container_last_seen   → any container that died (across all hosts)
#
# The fleet always has baseline noise (down targets, firing alerts), so absolute
# "all-green" gating is useless. Instead: snapshot BEFORE deploy, verify AFTER, and
# fail only on NEW breakage.
#
# Usage:
#   homelab-health.sh snapshot <baseline.json>       # capture pre-deploy state
#   homelab-health.sh verify   <baseline.json> [settle_seconds]  # default settle 45s
#
# Exit: 0 no new breakage · 1 new breakage (HALT) · 2 could not verify (monitoring
#       unreachable — e.g. roaming off the homelab LAN)
set -uo pipefail

PROM="${HOMELAB_PROM:-http://192.168.1.143:9090}"      # ocean
ALERT="${HOMELAB_ALERTMANAGER:-http://192.168.1.143:9093}"
TIMEOUT="${HOMELAB_HEALTH_TIMEOUT:-3}"                  # per-request; fast-fails when roaming
say() { printf '%s\n' "$*" >&2; }

# Capture the fleet's "what's wrong" state as {down, alerts, dead}. Returns 2 if the
# monitoring stack can't be reached (so callers never mistake unreachable for healthy).
snapshot_json() {
  local up al dd
  up=$(curl -fsS -m "$TIMEOUT" -G "$PROM/api/v1/query" --data-urlencode 'query=up==0' 2>/dev/null) || return 2
  printf '%s' "$up" | jq -e '.status=="success"' >/dev/null 2>&1 || return 2
  al=$(curl -fsS -m "$TIMEOUT" "$ALERT/api/v2/alerts?active=true&silenced=false&inhibited=false" 2>/dev/null) || return 2
  printf '%s' "$al" | jq -e 'type=="array"' >/dev/null 2>&1 || return 2
  dd=$(curl -fsS -m "$TIMEOUT" -G "$PROM/api/v1/query" --data-urlencode 'query=time()-container_last_seen{name!=""}>120' 2>/dev/null) || return 2
  jq -n --argjson up "$up" --argjson al "$al" --argjson dd "$dd" '{
    down:   [$up.data.result[]? | (.metric.job + "/" + .metric.instance)] | unique,
    alerts: [$al[]?             | (.labels.alertname + "@" + (.labels.instance // "-"))] | unique,
    dead:   [$dd.data.result[]? | .metric.name] | unique
  }'
}

cmd="${1:-}"; file="${2:-}"
[ -n "$file" ] || { say "usage: homelab-health.sh {snapshot|verify} <baseline.json> [settle_seconds]"; exit 2; }

case "$cmd" in
  snapshot)
    snap=$(snapshot_json) || { say "UNVERIFIED — cannot reach monitoring ($PROM / $ALERT). Roaming?"; exit 2; }
    printf '%s\n' "$snap" > "$file"
    say "baseline captured: $(printf '%s' "$snap" | jq -c '{down:(.down|length),alerts:(.alerts|length),dead:(.dead|length)}')  -> $file"
    ;;
  verify)
    [ -f "$file" ] || { say "UNVERIFIED — no baseline at $file (run 'snapshot' before the deploy)."; exit 2; }
    base=$(cat "$file")
    settle="${3:-45}"
    say "settling ${settle}s for scrapes/alerts to catch up ..."; sleep "$settle"
    cur=$(snapshot_json) || { say "UNVERIFIED — cannot reach monitoring ($PROM / $ALERT). Roaming?"; exit 2; }

    diff=$(jq -n --argjson b "$base" --argjson c "$cur" '{
      new_down:       ($c.down   - $b.down),
      new_alerts:     ($c.alerts - $b.alerts),
      new_dead:       ($c.dead   - $b.dead),
      resolved_down:  ($b.down   - $c.down),
      resolved_alerts:($b.alerts - $c.alerts)
    }')
    nd=$(printf '%s' "$diff" | jq -r '.new_down|length')
    na=$(printf '%s' "$diff" | jq -r '.new_alerts|length')
    nk=$(printf '%s' "$diff" | jq -r '.new_dead|length')

    say "── comprehensive health diff ─────────────────────────"
    say "pre-existing (ignored): $(printf '%s' "$base" | jq -c '{down:(.down|length),alerts:(.alerts|length),dead:(.dead|length)}')"
    [ "$(printf '%s' "$diff" | jq -r '.resolved_down|length')" -gt 0 ]   && say "recovered targets: $(printf '%s' "$diff" | jq -c '.resolved_down')"
    [ "$(printf '%s' "$diff" | jq -r '.resolved_alerts|length')" -gt 0 ] && say "cleared alerts:    $(printf '%s' "$diff" | jq -c '.resolved_alerts')"
    if [ "$nd" -gt 0 ] || [ "$na" -gt 0 ] || [ "$nk" -gt 0 ]; then
      say ""
      [ "$nd" -gt 0 ] && say "NEW down targets:  $(printf '%s' "$diff" | jq -c '.new_down')"
      [ "$na" -gt 0 ] && say "NEW alerts firing: $(printf '%s' "$diff" | jq -c '.new_alerts')"
      [ "$nk" -gt 0 ] && say "NEW dead cntnrs:   $(printf '%s' "$diff" | jq -c '.new_dead')"
      say "RESULT: UNHEALTHY — the deploy introduced new breakage. HALT."
      exit 1
    fi
    say "RESULT: HEALTHY — no new down targets, alerts, or dead containers across the deploy."
    exit 0
    ;;
  *)
    say "usage: homelab-health.sh {snapshot|verify} <baseline.json> [settle_seconds]"; exit 2 ;;
esac
