#!/usr/bin/env bash
# docker-refresh.sh — pull newer images for every running compose project and
# recreate only the services whose image actually changed.
#
# Floating tags do not update on their own. `docker compose up -d` fetches only
# when an image is missing locally, and `--quiet-pull` just silences a pull that
# was already going to happen, so a host keeps running whatever it first pulled
# until someone deletes the image. `compose pull` then `compose up -d` is the
# whole mechanism: compose compares image IDs and leaves unchanged services
# alone.
#
# Publishes the outcome to the node_exporter textfile collector, because the
# failure mode this replaces was silent for months. A project that stops
# refreshing, or a refresh that stops running at all, has to be visible.
#
# Usage: docker-refresh.sh <textfile_dir>   (writes <dir>/docker_refresh.prom)
set -uo pipefail

OUTDIR="${1:?usage: docker-refresh.sh <textfile_dir>}"
OUT="$OUTDIR/docker_refresh.prom"
TMP="$OUT.$$.tmp"

command -v docker >/dev/null 2>&1 || exit 0

# name<TAB>config-file for every running compose project. Services started with
# plain `docker run` (cloudflare-exporter, ndt-speedtest-exporter) are not
# compose projects and are deliberately not covered.
# Deliberately NOT fault-tolerant: if the output shape changes, this must fail
# loudly. Swallowing the error would enumerate zero projects and then report a
# clean run, which is the exact silence this whole thing exists to end.
projects() {
  docker compose ls --format json | python3 -c '
import json, sys
for p in json.load(sys.stdin):
    cfg = p.get("ConfigFiles", "").split(",")[0].strip()
    if cfg:
        print(p["Name"] + "\t" + cfg)'
}

# Image IDs currently backing a project, one per line, sorted.
image_ids() { docker compose -p "$1" -f "$2" images --quiet 2>/dev/null | sort -u; }

refresh() {
  local name="$1" cfg="$2" before after updated
  before=$(image_ids "$name" "$cfg")
  docker compose -p "$name" -f "$cfg" pull --quiet >/dev/null 2>&1 || return 1
  docker compose -p "$name" -f "$cfg" up -d >/dev/null 2>&1 || return 1
  after=$(image_ids "$name" "$cfg")
  updated=$(comm -13 <(printf '%s\n' "$before") <(printf '%s\n' "$after") | grep -c . || true)
  # The old image IDs stay on disk until the weekly prune, so the journal line
  # is the rollback target: pin one of these digests in the playbook.
  [ "$updated" -gt 0 ] && logger -t docker-refresh \
    "project=$name updated=$updated before=$(echo "$before" | tr '\n' ',') after=$(echo "$after" | tr '\n' ',')"
  echo "$updated"
  return 0
}

failed_any=0
if ! PROJECT_LIST=$(projects 2>/dev/null); then
  PROJECT_LIST=""
  failed_any=1
fi

{
  echo "# HELP docker_refresh_project_failed Last refresh of this compose project failed."
  echo "# TYPE docker_refresh_project_failed gauge"
  echo "# HELP docker_refresh_images_updated Images replaced in this project by the last refresh."
  echo "# TYPE docker_refresh_images_updated gauge"
  while IFS=$'\t' read -r name cfg; do
    [ -n "$name" ] || continue
    if updated=$(refresh "$name" "$cfg"); then
      printf 'docker_refresh_project_failed{project="%s"} 0\n' "$name"
      printf 'docker_refresh_images_updated{project="%s"} %s\n' "$name" "$updated"
    else
      failed_any=1
      printf 'docker_refresh_project_failed{project="%s"} 1\n' "$name"
      printf 'docker_refresh_images_updated{project="%s"} 0\n' "$name"
    fi
  done <<< "$PROJECT_LIST"

  # Enumeration itself broke, so no per-project row above is meaningful. Report
  # it as a failed pseudo-project rather than letting a zero-project run look
  # like success.
  [ "$failed_any" -eq 1 ] && [ -z "$PROJECT_LIST" ] \
    && printf 'docker_refresh_project_failed{project="_enumeration"} 1\n'

  echo "# HELP docker_refresh_last_run_timestamp_seconds Unix time of the last refresh attempt."
  echo "# TYPE docker_refresh_last_run_timestamp_seconds gauge"
  printf 'docker_refresh_last_run_timestamp_seconds %s\n' "$(date +%s)"
  echo "# HELP docker_refresh_last_success_timestamp_seconds Unix time of the last refresh where every project succeeded."
  echo "# TYPE docker_refresh_last_success_timestamp_seconds gauge"
  if [ "$failed_any" -eq 0 ]; then
    printf 'docker_refresh_last_success_timestamp_seconds %s\n' "$(date +%s)"
  else
    # Carry the previous success forward so the staleness alert measures time
    # since things last actually worked, not time since this file was written.
    grep '^docker_refresh_last_success_timestamp_seconds ' "$OUT" 2>/dev/null \
      || printf 'docker_refresh_last_success_timestamp_seconds 0\n'
  fi
} > "$TMP"

mv -f "$TMP" "$OUT"
