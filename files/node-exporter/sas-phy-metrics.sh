#!/usr/bin/env bash
# sas-phy-metrics.sh - publish SAS transport-layer phy error counters to the
# node_exporter textfile collector. READ-ONLY: reads /sys/class/sas_phy only,
# never touches a disk or the HBA. Rising invalid-dword / running-disparity /
# loss-of-dword-sync / phy-reset counts flag a marginal cable, backplane lane, or
# expander phy - the class of physical-layer fault behind correlated ZFS checksum
# errors (see the data01 resilver incident). No-op on hosts without a SAS HBA.
#
# Usage: sas-phy-metrics.sh <textfile_dir>   (writes <dir>/sas_phy.prom atomically)
set -uo pipefail
OUTDIR="${1:?usage: sas-phy-metrics.sh <textfile_dir>}"
OUT="$OUTDIR/sas_phy.prom"
TMP="$OUT.$$.tmp"

shopt -s nullglob
phys=(/sys/class/sas_phy/phy-*)
if [ "${#phys[@]}" -eq 0 ]; then
  rm -f "$OUT"          # no SAS HBA here - clear any stale file
  exit 0
fi

# read a sysfs attr as a leading integer (0 if absent/non-numeric)
num(){ local v; v=$(cat "$1" 2>/dev/null); v="${v%%[!0-9]*}"; echo "${v:-0}"; }

{
  echo "# HELP sas_phy_invalid_dword_total Invalid dwords on a SAS phy (cumulative since boot)"
  echo "# TYPE sas_phy_invalid_dword_total counter"
  echo "# HELP sas_phy_running_disparity_error_total Running-disparity errors on a SAS phy (cumulative)"
  echo "# TYPE sas_phy_running_disparity_error_total counter"
  echo "# HELP sas_phy_loss_of_dword_sync_total Loss-of-dword-sync events on a SAS phy (cumulative)"
  echo "# TYPE sas_phy_loss_of_dword_sync_total counter"
  echo "# HELP sas_phy_reset_problem_total Phy-reset problems on a SAS phy (cumulative)"
  echo "# TYPE sas_phy_reset_problem_total counter"
  echo "# HELP sas_phy_negotiated_linkrate_gbps Negotiated link rate of a SAS phy in Gbps"
  echo "# TYPE sas_phy_negotiated_linkrate_gbps gauge"
  for p in "${phys[@]}"; do
    phy=$(basename "$p")
    sas=$(cat "$p/sas_address" 2>/dev/null | tr -d '[:space:]')
    lbl="phy=\"$phy\",sas_address=\"${sas:-unknown}\""
    echo "sas_phy_invalid_dword_total{$lbl} $(num "$p/invalid_dword_count")"
    echo "sas_phy_running_disparity_error_total{$lbl} $(num "$p/running_disparity_error_count")"
    echo "sas_phy_loss_of_dword_sync_total{$lbl} $(num "$p/loss_of_dword_sync_count")"
    echo "sas_phy_reset_problem_total{$lbl} $(num "$p/phy_reset_problem_count")"
    # negotiated_linkrate looks like "6.0 Gbit"; emit the numeric Gbps (0 if down)
    lr=$(cat "$p/negotiated_linkrate" 2>/dev/null | grep -oE '^[0-9.]+' || echo 0)
    echo "sas_phy_negotiated_linkrate_gbps{$lbl} ${lr:-0}"
  done
} > "$TMP"

mv -f "$TMP" "$OUT"
