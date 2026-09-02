#!/usr/bin/env bash
# Self-check for docker-refresh.sh. Stubs the docker CLI and logger so the whole
# script runs without a daemon. Each test is named after the bug it pins.
#
# Usage: bash files/node-exporter/test_docker_refresh.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/docker-refresh.sh"
pass=0 fail=0

ok() { echo "PASS: $1"; pass=$((pass + 1)); }
no() { echo "FAIL: $1 ($2)"; fail=$((fail + 1)); }

check() { # check <name> <expected-substring> <file>
  if grep -qF "$2" "$3"; then ok "$1"; else no "$1" "missing '$2' in $(cat "$3")"; fi
}
refute() {
  if grep -qF "$2" "$3"; then no "$1" "unexpected '$2'"; else ok "$1"; fi
}
exited() { # exited <name> <expected-rc>
  if [ "$RC" = "$2" ]; then ok "$1"; else no "$1" "exit $RC, wanted $2"; fi
}

# Runs the script with <stub-body> as the docker CLI. Journal lines land in
# $LOG, so what the script reports is assertable without a syslog.
run_with_stub() { # run_with_stub <stub-body-file> <project>
  BIN=$(mktemp -d)
  { echo '#!/usr/bin/env bash'; cat "$1"; } > "$BIN/docker"
  printf '#!/usr/bin/env bash\nshift 2\necho "$*" >> %s\n' "$LOG" > "$BIN/logger"
  chmod +x "$BIN/docker" "$BIN/logger"
  PATH="$BIN:$PATH" bash "$SCRIPT" "$2" > "$OUT" 2>&1
  RC=$?
  rm -rf "$BIN"
}

LOG=$(mktemp); OUT=$(mktemp); STUB=$(mktemp)

# A project whose image ID changes between the two `images` calls: one image was
# replaced, and the journal has to carry both digests, because the hourly prune
# deletes the old image and that line is the only rollback trail left.
cat > "$STUB" <<'STUBEOF'
case "$*" in
  "compose ls --format json")
    echo '[{"Name":"plex","Status":"running(1)","ConfigFiles":"/data01/services/plex/docker-compose.yml"}]' ;;
  *images\ --quiet)
    if [ -f /tmp/.dr_pulled ]; then echo sha256:new; else echo sha256:old; fi ;;
  *pull*) touch /tmp/.dr_pulled ;;
  *) : ;;
esac
STUBEOF
rm -f /tmp/.dr_pulled
: > "$LOG"
run_with_stub "$STUB" plex
exited "a refreshed project exits clean" 0
check "a replaced image is logged with both digests" "project=plex updated=1 before=sha256:old, after=sha256:new," "$LOG"
rm -f /tmp/.dr_pulled

# Multiple config files come back comma-separated; only the first is the compose
# file to act on. An unchanged project must log nothing at all.
cat > "$STUB" <<'STUBEOF'
case "$*" in
  "compose ls --format json")
    echo '[{"Name":"mem0","ConfigFiles":"/a/docker-compose.yml,/a/override.yml"}]' ;;
  *images\ --quiet) echo sha256:same ;;
  *) : ;;
esac
STUBEOF
: > "$LOG"
run_with_stub "$STUB" mem0
exited "comma-separated ConfigFiles still resolves the project" 0
refute "an unchanged project logs nothing" "project=mem0 updated" "$LOG"

# A failed pull has to leave the unit failed. Exiting 0 here would hand the
# whole mechanism back its old silence: SystemdUnitFailed is the only thing
# watching this now.
cat > "$STUB" <<'STUBEOF'
case "$*" in
  "compose ls --format json")
    echo '[{"Name":"paia","ConfigFiles":"/a/docker-compose.yml"}]' ;;
  *images\ --quiet) echo sha256:same ;;
  *pull*) exit 1 ;;
  *) : ;;
esac
STUBEOF
run_with_stub "$STUB" paia
exited "a failed pull fails the unit" 1

# A project that is not running is not this script's business: starting it would
# resurrect something stopped on purpose, and its own unit already alerts.
cat > "$STUB" <<'STUBEOF'
case "$*" in
  "compose ls --format json")
    echo '[{"Name":"plex","ConfigFiles":"/a/docker-compose.yml"}]' ;;
  *pull*) echo PULLED ;;
  *up\ -d*) echo STARTED ;;
  *) : ;;
esac
STUBEOF
: > "$LOG"
run_with_stub "$STUB" jellyfin
exited "a stopped project is skipped, not failed" 0
check "a skip says so in the journal" "project=jellyfin not running, skipped" "$LOG"
refute "a stopped project is never started" "STARTED" "$OUT"

# The bug this guards: an unparseable `compose ls` used to be swallowed, which
# enumerated zero projects and then reported a clean run.
cat > "$STUB" <<'STUBEOF'
case "$*" in
  "compose ls --format json") echo 'not json at all' ;;
  *) : ;;
esac
STUBEOF
: > "$LOG"
run_with_stub "$STUB" plex
exited "broken enumeration fails, not swallowed" 1
check "broken enumeration says so in the journal" "project=plex enumeration failed" "$LOG"

rm -f "$LOG" "$OUT" "$STUB"
echo
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
