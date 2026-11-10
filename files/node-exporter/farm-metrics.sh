#!/usr/bin/env bash
# farm-metrics.sh - publish Seagate FARM log counters to the node_exporter
# textfile collector. READ-ONLY: `smartctl -l farm` reads a drive log page and
# never writes to a disk.
#
# Why: resellers zero the SMART counters on used Seagate drives and sell them as
# recertified, but the FARM log keeps the drive's real lifetime. ZJV4M3HE showed
# 2,052 SMART power-on hours against 53,201 in FARM when it faulted in data01
# (2026-11-10), after 85 days in service. Plotting both side by side shows which
# drives arrived worn out.
#
# smartctl lives in the smart-exporter container, not on the host, so this runs
# it there. Drives without a FARM log (non-Seagate, NVMe, virtual) are skipped.
#
# Usage: farm-metrics.sh <textfile_dir>   (writes <dir>/smart_farm.prom atomically)
set -uo pipefail
OUTDIR="${1:?usage: farm-metrics.sh <textfile_dir>}"
OUT="$OUTDIR/smart_farm.prom"
TMP="$OUT.$$.tmp"

# Exporter down: keep the last good file rather than publish an empty one.
docker exec smart-exporter true 2>/dev/null || exit 0

{
  echo "# HELP smart_farm_power_on_hours Lifetime powered-on hours from the Seagate FARM log (not reset with SMART)"
  echo "# TYPE smart_farm_power_on_hours gauge"
  echo "# HELP smart_farm_spindle_power_on_hours Spindle-on hours from the Seagate FARM log"
  echo "# TYPE smart_farm_spindle_power_on_hours gauge"
  echo "# HELP smart_farm_power_cycle_count Lifetime power cycles from the Seagate FARM log"
  echo "# TYPE smart_farm_power_cycle_count gauge"
  echo "# HELP smart_farm_reallocated_sectors Reallocated sectors from the Seagate FARM log"
  echo "# TYPE smart_farm_reallocated_sectors gauge"
  echo "# HELP smart_farm_unrecoverable_read_errors Unrecoverable read errors from the Seagate FARM log"
  echo "# TYPE smart_farm_unrecoverable_read_errors gauge"
  for b in /sys/block/sd*; do
    dev=$(basename "$b")
    docker exec smart-exporter smartctl --json=c -l farm "/dev/$dev" 2>/dev/null \
      | jq -r --arg dev "$dev" '
          .seagate_farm_log // empty
          | select(.page_1_drive_information.poh != null)
          | .page_1_drive_information as $i | .page_3_error_statistics as $e
          | "{device=\"\($dev)\",serial_number=\"\($i.serial_number)\",wwn=\"\($i.world_wide_name)\"}" as $l
          | "smart_farm_power_on_hours\($l) \($i.poh)",
            "smart_farm_spindle_power_on_hours\($l) \($i.spoh)",
            "smart_farm_power_cycle_count\($l) \($i.power_cycle_count)",
            "smart_farm_reallocated_sectors\($l) \($e.number_of_reallocated_sectors)",
            "smart_farm_unrecoverable_read_errors\($l) \($e.number_of_unrecoverable_read_errors)"'
  done
} > "$TMP"

mv -f "$TMP" "$OUT"
