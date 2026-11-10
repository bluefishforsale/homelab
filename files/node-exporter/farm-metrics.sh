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
  echo "# HELP smart_farm_power_on_seconds Lifetime powered-on time from the Seagate FARM log (not reset with SMART)"
  echo "# TYPE smart_farm_power_on_seconds gauge"
  echo "# HELP smart_farm_spindle_power_on_seconds Spindle-on time from the Seagate FARM log"
  echo "# TYPE smart_farm_spindle_power_on_seconds gauge"
  echo "# HELP smart_farm_power_cycles Lifetime power cycles from the Seagate FARM log"
  echo "# TYPE smart_farm_power_cycles gauge"
  echo "# HELP smart_farm_reallocated_sectors Reallocated sectors from the Seagate FARM log"
  echo "# TYPE smart_farm_reallocated_sectors gauge"
  echo "# HELP smart_farm_unrecoverable_read_errors Unrecoverable read errors from the Seagate FARM log"
  echo "# TYPE smart_farm_unrecoverable_read_errors gauge"
  echo "# HELP smart_farm_smartctl_exit_code smartctl exit status for the FARM read (143 = killed by the 60s timeout)"
  echo "# TYPE smart_farm_smartctl_exit_code gauge"
  echo "# HELP smart_farm_log_present 1 if this run parsed a FARM log from the device, else 0"
  echo "# TYPE smart_farm_log_present gauge"
  shopt -s nullglob
  for b in /sys/block/sd*; do
    dev=$(basename "$b")
    # -n standby: never spin up a sleeping disk just to read a counter.
    # The inner timeout kills smartctl itself (docker exec does not forward
    # signals); the outer one only guards a hung docker client.
    out=$(timeout 90 docker exec smart-exporter timeout 60 smartctl --json=c -n standby -l farm "/dev/$dev" 2>/dev/null)
    rc=$?
    # Per-device status keyed on device alone, so a FARM disk that stops
    # answering (dying, asleep, timed out) shows up as present=0 instead of
    # silently vanishing with the labels its failed read would have supplied.
    echo "smart_farm_smartctl_exit_code{device=\"$dev\"} $rc"
    lines=$(printf '%s' "$out" | jq -r --arg dev "$dev" '
      .seagate_farm_log // empty
      | .page_1_drive_information as $i | (.page_3_error_statistics // {}) as $e
      | select($i.poh != null)
      | "{device=\"\($dev)\",serial_number=\"\($i.serial_number)\",wwn=\"\($i.world_wide_name)\"}" as $l
      | [["smart_farm_power_on_seconds", ($i.poh | if type == "number" then . * 3600 else . end)],
         ["smart_farm_spindle_power_on_seconds", ($i.spoh | if type == "number" then . * 3600 else . end)],
         ["smart_farm_power_cycles", $i.power_cycle_count],
         ["smart_farm_reallocated_sectors", $e.number_of_reallocated_sectors],
         ["smart_farm_unrecoverable_read_errors", $e.number_of_unrecoverable_read_errors]][]
      # A field missing from this drive family drops that one line; a literal
      # null would make node_exporter reject the whole file.
      | select(.[1] | type == "number")
      | "\(.[0])\($l) \(.[1])"' 2>/dev/null)
    if [ -n "$lines" ]; then
      echo "$lines"
      echo "smart_farm_log_present{device=\"$dev\"} 1"
    else
      echo "smart_farm_log_present{device=\"$dev\"} 0"
    fi
  done
} > "$TMP"

mv -f "$TMP" "$OUT"
