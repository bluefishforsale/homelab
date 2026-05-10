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

# 1. Empty input → []
out=$(printf '' | bash "$SCRIPT")
assert_eq "empty input" "[]" "$out"

# 2. Single individual playbook → just that playbook
out=$(printf 'playbooks/individual/ocean/services/terrac_com.yaml\n' | bash "$SCRIPT")
assert_eq "homepage playbook only" \
  '["playbooks/individual/ocean/services/terrac_com.yaml"]' "$out"

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
out=$(printf 'playbooks/individual/ocean/services/terrac_com.yaml\nfiles/nginx-compose/proxy_hostname_web_proxy.conf\nfiles/nginx-compose/proxy_hostname_web_proxy.conf\n' | bash "$SCRIPT")
assert_eq "mixed homepage+nginx (deduped, sorted)" \
  '["playbooks/individual/ocean/network/nginx_compose.yaml","playbooks/individual/ocean/services/terrac_com.yaml"]' "$out"

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

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
