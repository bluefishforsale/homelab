#!/usr/bin/env bash
# Self-check for docker-refresh.sh. Stubs the docker CLI so the whole script
# runs without a daemon. Each test is named after the bug it pins.
#
# Usage: bash files/node-exporter/test_docker_refresh.sh
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/docker-refresh.sh"
pass=0 fail=0

check() { # check <name> <expected-substring> <file>
  if grep -qF "$2" "$3"; then echo "PASS: $1"; pass=$((pass + 1))
  else echo "FAIL: $1 (missing '$2' in $(basename "$3"))"; fail=$((fail + 1)); fi
}
refute() {
  if grep -qF "$2" "$3"; then echo "FAIL: $1 (unexpected '$2')"; fail=$((fail + 1))
  else echo "PASS: $1"; pass=$((pass + 1)); fi
}

run_with_stub() { # run_with_stub <stub-body-file> <textfile-dir>
  local bin; bin=$(mktemp -d)
  { echo '#!/usr/bin/env bash'; cat "$1"; } > "$bin/docker"
  chmod +x "$bin/docker"
  PATH="$bin:$PATH" bash "$SCRIPT" "$2" >/dev/null 2>&1
  rm -rf "$bin"
}

# A project whose image ID changes between the two `images` calls: one image
# was replaced, and exactly one must be reported.
TD=$(mktemp -d); STUB=$(mktemp)
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
run_with_stub "$STUB" "$TD"
check "a replaced image is counted" 'docker_refresh_images_updated{project="plex"} 1' "$TD/docker_refresh.prom"
check "a successful project reports not-failed" 'docker_refresh_project_failed{project="plex"} 0' "$TD/docker_refresh.prom"
rm -f /tmp/.dr_pulled

# Multiple config files come back comma-separated; only the first is the
# compose file to act on.
cat > "$STUB" <<'STUBEOF'
case "$*" in
  "compose ls --format json")
    echo '[{"Name":"mem0","ConfigFiles":"/a/docker-compose.yml,/a/override.yml"}]' ;;
  *images\ --quiet) echo sha256:same ;;
  *) : ;;
esac
STUBEOF
run_with_stub "$STUB" "$TD"
check "comma-separated ConfigFiles still yields the project" 'docker_refresh_project_failed{project="mem0"} 0' "$TD/docker_refresh.prom"
check "an unchanged project reports zero updates" 'docker_refresh_images_updated{project="mem0"} 0' "$TD/docker_refresh.prom"
prev_success=$(awk '/^docker_refresh_last_success_timestamp_seconds /{print $2}' "$TD/docker_refresh.prom")

# A pull that fails must mark the project failed and must NOT stamp a fresh
# success, or the staleness alert would measure the wrong thing.
cat > "$STUB" <<'STUBEOF'
case "$*" in
  "compose ls --format json")
    echo '[{"Name":"paia","ConfigFiles":"/a/docker-compose.yml"}]' ;;
  *images\ --quiet) echo sha256:same ;;
  *pull*) exit 1 ;;
  *) : ;;
esac
STUBEOF
run_with_stub "$STUB" "$TD"
check "a failed pull marks the project failed" 'docker_refresh_project_failed{project="paia"} 1' "$TD/docker_refresh.prom"
# The previous run above succeeded, so its timestamp must survive this failure
# verbatim: the staleness alert measures time since things last worked, not
# time since the file was written.
after=$(awk '/^docker_refresh_last_success_timestamp_seconds /{print $2}' "$TD/docker_refresh.prom")
if [ -n "$prev_success" ] && [ "$prev_success" != "0" ] && [ "$after" = "$prev_success" ]; then
  echo "PASS: a failed run carries the previous success forward"; pass=$((pass + 1))
else
  echo "FAIL: a failed run carries the previous success forward (was '$prev_success', now '$after')"; fail=$((fail + 1))
fi

# The bug this guards: an unparseable `compose ls` used to be swallowed, which
# enumerated zero projects and then wrote a clean success.
cat > "$STUB" <<'STUBEOF'
case "$*" in
  "compose ls --format json") echo 'not json at all' ;;
  *) : ;;
esac
STUBEOF
run_with_stub "$STUB" "$TD"
check "broken enumeration is reported, not swallowed" 'docker_refresh_project_failed{project="_enumeration"} 1' "$TD/docker_refresh.prom"

rm -rf "$TD" "$STUB"
echo
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
