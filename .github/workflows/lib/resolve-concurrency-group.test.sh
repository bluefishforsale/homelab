#!/usr/bin/env bash
# Tests for resolve-concurrency-group.sh.
# Run from repo root.
set -euo pipefail

SCRIPT="$(dirname "$0")/resolve-concurrency-group.sh"
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

# 1. Empty array → deploy-none
out=$(printf '[]\n' | bash "$SCRIPT")
assert_eq "empty array" "deploy-none" "$out"

# 2. Single ocean playbook → deploy-ocean
out=$(printf '["playbooks/individual/ocean/services/homepage.yaml"]\n' | bash "$SCRIPT")
assert_eq "homepage → ocean" "deploy-ocean" "$out"

# 3. Single runner playbook → deploy-github_runners (matches playbook hosts: directive)
out=$(printf '["playbooks/individual/infrastructure/github_docker_runners.yaml"]\n' | bash "$SCRIPT")
assert_eq "github_docker_runners → github_runners" "deploy-github_runners" "$out"

# 4. Two ocean playbooks → deploy-ocean (deduped)
out=$(printf '["playbooks/individual/ocean/services/homepage.yaml","playbooks/individual/ocean/network/nginx_compose.yaml"]\n' | bash "$SCRIPT")
assert_eq "two ocean playbooks deduped" "deploy-ocean" "$out"

# 5. Mixed ocean + runners → deploy-github_runners-ocean (sorted)
out=$(printf '["playbooks/individual/ocean/services/homepage.yaml","playbooks/individual/infrastructure/github_docker_runners.yaml"]\n' | bash "$SCRIPT")
assert_eq "ocean + runners sorted" "deploy-github_runners-ocean" "$out"

# 6. Orchestrator (01_base_system has no top-level hosts:, so it should default to 'all')
#    This tests handling of playbooks without explicit hosts: directives
out=$(printf '["playbooks/01_base_system.yaml"]\n' | bash "$SCRIPT")
assert_eq "orchestrator 01 → all" "deploy-all" "$out"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
