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

# 16. Regression: a genuinely shared vars file still fans out (not scoped, not fallback).
out=$(printf 'vars/vars_service_ports.yaml\n' | bash "$SCRIPT")
assert_ne "vars_service_ports still fans out (not fallback)" "$FALLBACK" "$out"
assert_ne "vars_service_ports non-empty" "[]" "$out"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
