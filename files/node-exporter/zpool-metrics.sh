#!/usr/bin/env bash
# zpool-metrics.sh — publish ZFS pool health to the node_exporter textfile
# collector. READ-ONLY: it only runs `zpool list`/`zpool status` and NEVER
# mutates a pool (no import/export/mount/replace/clear). Storage changes stay a
# coordinated, hand-run task (see docs/operations and agents.md). Safe on hosts
# without ZFS (no-op). Every zpool call is timeout-bounded so a suspended pool
# cannot hang the collector.
#
# Usage: zpool-metrics.sh <textfile_dir>   (writes <dir>/zpool.prom atomically)
set -uo pipefail

OUTDIR="${1:?usage: zpool-metrics.sh <textfile_dir>}"
OUT="$OUTDIR/zpool.prom"
TMP="$OUT.$$.tmp"
ZT=20 # per-command timeout seconds

zp() { timeout "$ZT" zpool "$@" 2>/dev/null; }
zf() { timeout "$ZT" zfs "$@" 2>/dev/null; }

if ! command -v zpool >/dev/null 2>&1; then
  rm -f "$OUT"          # no ZFS here — clear any stale file
  exit 0
fi

health_num() {
  case "$1" in
    ONLINE) echo 0 ;;
    DEGRADED) echo 1 ;;
    FAULTED|UNAVAIL|SUSPENDED|REMOVED|OFFLINE) echo 2 ;;
    *) echo 3 ;;
  esac
}

# zpool status abbreviates large counts (48, 1.2K, 3M); expand to a plain number.
num() {
  case "$1" in
    *K) awk -v n="${1%K}" 'BEGIN{printf "%.0f", n*1000}' ;;
    *M) awk -v n="${1%M}" 'BEGIN{printf "%.0f", n*1000000}' ;;
    *G) awk -v n="${1%G}" 'BEGIN{printf "%.0f", n*1000000000}' ;;
    ''|*[!0-9]*) echo 0 ;;   # non-numeric (e.g. '-') -> 0
    *) echo "$1" ;;
  esac
}

{
  echo "# HELP zpool_health Pool health (0=ONLINE 1=DEGRADED 2=FAULTED/other 3=unknown)"
  echo "# TYPE zpool_health gauge"
  echo "# HELP zpool_device_errors Per-leaf-device error counter from zpool status"
  echo "# TYPE zpool_device_errors gauge"
  echo "# HELP zpool_device_faulted 1 only if a leaf device is FAULTED (DEGRADED/REMOVED/OFFLINE are not faulted)"
  echo "# TYPE zpool_device_faulted gauge"
  echo "# HELP zpool_device_state Current leaf-device state, 1 on the state label that is currently active"
  echo "# TYPE zpool_device_state gauge"
  echo "# HELP zpool_resilver_active 1 if a resilver/scrub is in progress"
  echo "# TYPE zpool_resilver_active gauge"
  echo "# HELP zpool_scan_percent Percent complete of the active scan; 100 when idle/complete"
  echo "# TYPE zpool_scan_percent gauge"
  echo "# HELP zpool_size_bytes Total pool size in bytes (raw, incl. parity)"
  echo "# TYPE zpool_size_bytes gauge"
  echo "# HELP zpool_alloc_bytes Allocated (used) pool bytes"
  echo "# TYPE zpool_alloc_bytes gauge"
  echo "# HELP zpool_free_bytes Free pool bytes"
  echo "# TYPE zpool_free_bytes gauge"
  echo "# HELP zpool_snapshot_used_bytes Bytes held by snapshots (sum of usedsnap over all datasets); reclaimable by destroying them"
  echo "# TYPE zpool_snapshot_used_bytes gauge"

  for pool in $(zp list -H -o name); do
    echo "zpool_health{pool=\"$pool\"} $(health_num "$(zp list -H -o health "$pool")")"

    # Capacity trend (parseable bytes). ${x:-0} guards a timed-out/empty read.
    read -r psize palloc pfree < <(zp list -Hp -o size,alloc,free "$pool")
    echo "zpool_size_bytes{pool=\"$pool\"} ${psize:-0}"
    echo "zpool_alloc_bytes{pool=\"$pool\"} ${palloc:-0}"
    echo "zpool_free_bytes{pool=\"$pool\"} ${pfree:-0}"
    # Space held by snapshots: sum usedsnap across every dataset. Tiny in
    # practice, so this reads as a snapshot-retention/leak detector, not a
    # capacity lever (the bulk lives in live datasets, see zpool_alloc_bytes).
    snap=$(zf list -Hp -o usedsnap -r "$pool" | awk '{s+=$1} END{printf "%d", s+0}')
    echo "zpool_snapshot_used_bytes{pool=\"$pool\"} ${snap:-0}"

    status="$(zp status "$pool")"
    if printf '%s\n' "$status" | grep -qiE "scan:.*(resilver|scrub) in progress"; then
      echo "zpool_resilver_active{pool=\"$pool\"} 1"
      pct=$(printf '%s\n' "$status" | grep -oE "[0-9]+\.[0-9]+% done" | grep -oE "[0-9]+\.[0-9]+" | head -1)
      echo "zpool_scan_percent{pool=\"$pool\"} ${pct:-0}"
    else
      echo "zpool_resilver_active{pool=\"$pool\"} 0"
      # No scan running: emit 100 (idle/complete) every run. Emitting nothing
      # here let the metric go stale, so the Grafana gauge's lastNotNull held
      # the pre-completion value (stuck at 99.6% after this resilver finished).
      echo "zpool_scan_percent{pool=\"$pool\"} 100"
    fi

    # Leaf devices only: skip the pool row and vdev-container rows (raidz*, mirror*, ...).
    printf '%s\n' "$status" | awk -v pool="$pool" '
      /NAME[ \t]+STATE/ {intable=1; next}
      /^errors:/ {intable=0}
      intable && NF>=5 {
        name=$1; state=$2;
        if (name==pool) next;
        if (name ~ /^(raidz|mirror|draid|spare|log|cache|special|dedup|replacing|indirect)/) next;
        if (state ~ /^(ONLINE|DEGRADED|FAULTED|UNAVAIL|OFFLINE|REMOVED|SUSPENDED)$/)
          print name"\t"state"\t"$3"\t"$4"\t"$5
      }' | while IFS=$'\t' read -r dev state r w c; do
        echo "zpool_device_errors{pool=\"$pool\",device=\"$dev\",type=\"read\"} $(num "$r")"
        echo "zpool_device_errors{pool=\"$pool\",device=\"$dev\",type=\"write\"} $(num "$w")"
        echo "zpool_device_errors{pool=\"$pool\",device=\"$dev\",type=\"cksum\"} $(num "$c")"
        # FAULTED means the disk is out; DEGRADED/REMOVED/OFFLINE are impaired but
        # not faulted (e.g. a mid-replace REMOVED old drive, or a flaky-but-serving
        # DEGRADED disk). Conflating them over-counts "faulted" (see #283 resilver).
        if [ "$state" = "FAULTED" ]; then
          echo "zpool_device_faulted{pool=\"$pool\",device=\"$dev\"} 1"
        else
          echo "zpool_device_faulted{pool=\"$pool\",device=\"$dev\"} 0"
        fi
        echo "zpool_device_state{pool=\"$pool\",device=\"$dev\",state=\"$state\"} 1"
      done
  done
} > "$TMP"

mv -f "$TMP" "$OUT"
