#!/usr/bin/env bash
# Tests for detect-impacted-playbooks.sh.
# Run from repo root: bash .github/workflows/lib/detect-impacted-playbooks.test.sh
set -euo pipefail

SCRIPT="$(dirname "$0")/detect-impacted-playbooks.sh"
PASS=0
FAIL=0

assert_eq() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name"
    echo "  expected: $expected"
    echo "  actual:   $actual"
    FAIL=$((FAIL + 1))
  fi
}

assert_ne() {
  local name="$1" notexpected="$2" actual="$3"
  if [[ "$notexpected" != "$actual" ]]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name (should NOT equal $notexpected)"
    FAIL=$((FAIL + 1))
  fi
}

# Asserts the detector exits non-zero (hard-fail on an unmapped input).
# The `if` guard keeps `set -e` from aborting the suite on the expected failure.
assert_fails() {
  local name="$1" input="$2"
  if printf '%s\n' "$input" | bash "$SCRIPT" >/dev/null 2>&1; then
    echo "FAIL: $name (expected non-zero exit, got 0)"
    FAIL=$((FAIL + 1))
  else
    echo "PASS: $name"
    PASS=$((PASS + 1))
  fi
}

# 1. Empty input → []
out=$(printf '' | bash "$SCRIPT")
assert_eq "empty input" "[]" "$out"

# 2. Single individual playbook → just that playbook
out=$(printf 'playbooks/individual/ocean/services/gethomepage.yaml\n' | bash "$SCRIPT")
assert_eq "homepage playbook only" \
  '["playbooks/individual/ocean/services/gethomepage.yaml"]' "$out"

# 3. Cloudflared config file → cloudflared playbook
out=$(printf 'files/cloudflared/config.yaml.j2\n' | bash "$SCRIPT")
assert_eq "cloudflared config" \
  '["playbooks/individual/ocean/network/cloudflared.yaml"]' "$out"

# 4. Nginx vhost file → nginx playbook
out=$(printf 'files/nginx-compose/proxy_hostname_web_proxy.conf\n' | bash "$SCRIPT")
assert_eq "nginx vhost" \
  '["playbooks/individual/ocean/network/nginx_compose.yaml"]' "$out"

# 5. vars_cloudflared.yaml → cloudflared playbook
out=$(printf 'vars/vars_cloudflared.yaml\n' | bash "$SCRIPT")
assert_eq "vars_cloudflared" \
  '["playbooks/individual/ocean/network/cloudflared.yaml"]' "$out"

# 6. inventories → fallback
out=$(printf 'inventories/production/hosts.ini\n' | bash "$SCRIPT")
assert_eq "inventories fallback" \
  '["playbooks/01_base_system.yaml","playbooks/02_core_infrastructure.yaml","playbooks/03_ocean_services.yaml"]' "$out"

# 7. group_vars/all.yaml → fallback
out=$(printf 'group_vars/all.yaml\n' | bash "$SCRIPT")
assert_eq "group_vars all fallback" \
  '["playbooks/01_base_system.yaml","playbooks/02_core_infrastructure.yaml","playbooks/03_ocean_services.yaml"]' "$out"

# 8. Mixed inputs are unioned and deduped
out=$(printf 'playbooks/individual/ocean/services/gethomepage.yaml\nfiles/nginx-compose/proxy_hostname_web_proxy.conf\nfiles/nginx-compose/proxy_hostname_web_proxy.conf\n' | bash "$SCRIPT")
assert_eq "mixed homepage+nginx (deduped, sorted)" \
  '["playbooks/individual/ocean/network/nginx_compose.yaml","playbooks/individual/ocean/services/gethomepage.yaml"]' "$out"

# 9. Orchestrator playbook explicit edit → that orchestrator only
out=$(printf 'playbooks/01_base_system.yaml\n' | bash "$SCRIPT")
assert_eq "explicit orchestrator edit" \
  '["playbooks/01_base_system.yaml"]' "$out"

# 10. roles/dns_infrastructure/** → playbooks that reference that role
out=$(printf 'roles/dns_infrastructure/templates/foo.j2\n' | bash "$SCRIPT")
assert_eq "dns_infrastructure role" \
  '["playbooks/individual/core/services/dns_ha_stack.yaml"]' "$out"

# 11. roles/github_docker_runners/** → runner playbook(s)
out=$(printf 'roles/github_docker_runners/defaults/main.yml\n' | bash "$SCRIPT")
assert_eq "github_docker_runners role" \
  '["playbooks/individual/infrastructure/github_docker_runners.yaml"]' "$out"

# 12. README / docs changes are not in the workflow's paths filter,
#     but if smuggled in (e.g., trailing-doc edit), they should be no-op (fallback skipped — empty).
#     We test that .md under playbooks/ does NOT trigger fallback.
out=$(printf 'playbooks/README.md\n' | bash "$SCRIPT")
assert_eq "playbooks markdown ignored" "[]" "$out"

FALLBACK='["playbooks/01_base_system.yaml","playbooks/02_core_infrastructure.yaml","playbooks/03_ocean_services.yaml"]'

# 13. files/llamacpp/** -> llamacpp playbook via `service: llamacpp` indirection.
#     Regression: this used to grep-miss (playbook uses files/{{ service }}) and
#     fan out to the site-wide fallback, dragging in unrelated hosts/services.
out=$(printf 'files/llamacpp/docker-compose.yml.j2\n' | bash "$SCRIPT")
assert_eq "files/llamacpp via service indirection" \
  '["playbooks/individual/ocean/ai/llamacpp.yaml"]' "$out"

# 14. vars/vars_llamacpp_models.yaml -> only the llamacpp playbook.
#     terminalbench also loads this vars file but is filtered at emit().
out=$(printf 'vars/vars_llamacpp_models.yaml\n' | bash "$SCRIPT")
assert_eq "vars_llamacpp_models maps to its playbook (terminalbench excluded)" \
  '["playbooks/individual/ocean/ai/llamacpp.yaml"]' "$out"

# 15. The exact llamacpp deploy commit (both files) -> just the llamacpp playbook,
#     NOT the site-wide fallback. This is the collateral-blast-radius bug.
out=$(printf 'files/llamacpp/docker-compose.yml.j2\nvars/vars_llamacpp_models.yaml\n' | bash "$SCRIPT")
assert_eq "llamacpp commit stays scoped to its playbook" \
  '["playbooks/individual/ocean/ai/llamacpp.yaml"]' "$out"

# (vars_service_ports mapping is covered by the key-level cases 22-25 below,
#  which supersede the old "always fans out" guard now that it maps by key.)

# 17. files/<dir> with no literal ref and no service: decl resolves by matching a
#     playbook named <dir>.yaml (e.g. files/gpu-test -> gpu-test.yaml).
out=$(printf 'files/gpu-test/probe\n' | bash "$SCRIPT")
assert_eq "files/gpu-test resolves by playbook basename" \
  '["playbooks/individual/ocean/gpu-test.yaml"]' "$out"

# 18. An allowlisted (known-unowned) input is a no-op: [] and exit 0, not a fail.
out=$(printf 'files/navidrome/config\n' | bash "$SCRIPT")
assert_eq "allowlisted unowned input is a no-op" "[]" "$out"

# 19. A genuinely new, unowned service dir hard-fails (exit != 0), never fans out.
assert_fails "unmapped new files/<dir> hard-fails" "files/totally-new-service-xyz/x"

# 20. A vars file no playbook loads and that is not allowlisted hard-fails.
assert_fails "unmapped vars_<name> hard-fails" "vars/vars_nonexistent_xyz.yaml"

# 21. Global inputs still replay the orchestrators (this is the ONLY fallback now).
out=$(printf 'inventories/production/hosts.ini\n' | bash "$SCRIPT")
assert_eq "inventory change is the site-wide fallback" "$FALLBACK" "$out"

# 22-25. vars_service_ports.yaml maps by WHICH port key changed, not the filename.
PORTS="vars/vars_service_ports.yaml"
BASE_SAME="$(mktemp)"; cp "$PORTS" "$BASE_SAME"
BASE_PLEX="$(mktemp)"; sed 's/port: 9594/port: 9999/' "$PORTS" > "$BASE_PLEX"

out=$(printf '%s\n' "$PORTS" | DETECT_PORTS_BASE_FILE="$BASE_PLEX" bash "$SCRIPT")
assert_eq "single port key change maps to only that service" \
  '["playbooks/individual/ocean/media/plex.yaml"]' "$out"

out=$(printf '%s\n' "$PORTS" | DETECT_PORTS_BASE_FILE="$BASE_SAME" bash "$SCRIPT")
assert_eq "port file comment/format-only change is a no-op" "[]" "$out"

out=$(printf '%s\n' "$PORTS" | DETECT_BASE=nonexistent_ref_xyz bash "$SCRIPT")
assert_ne "port change with no base falls back to consumers, not orchestrators" "$FALLBACK" "$out"
assert_ne "port fallback is non-empty (never under-deploys)" "[]" "$out"
rm -f "$BASE_SAME" "$BASE_PLEX"

# 26-28. Non-deploying paths are ignored, never hard-failed. A workflow/scripts
#        edit must not fail the run, and a mixed commit keeps its real target.
out=$(printf '.github/workflows/ci-validate.yml\n' | bash "$SCRIPT")
assert_eq ".github change is ignored (no-op)" "[]" "$out"

out=$(printf 'scripts/prom.sh\n' | bash "$SCRIPT")
assert_eq "scripts change is ignored (no-op)" "[]" "$out"

out=$(printf 'playbooks/individual/ocean/ai/llamacpp.yaml\n.github/workflows/main-apply.yml\n' | bash "$SCRIPT")
assert_eq "mixed playbook+workflow commit keeps only the playbook" \
  '["playbooks/individual/ocean/ai/llamacpp.yaml"]' "$out"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
