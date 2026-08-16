#!/usr/bin/env bash
# ownership-coverage.test.sh
#
# Guarantees the detector's precision invariant across the WHOLE repo, not just
# hand-picked cases: every owned input (files/<dir>, vars/vars_*, roles/<role>)
# must resolve to a specific playbook or be a declared no-op (allowlisted in
# detect-impacted-playbooks.sh). Anything that resolves to nothing and is not
# allowlisted fails here — so a new, unwired service is caught at PR time
# instead of silently fanning out across the fleet on push.
#
# Run from repo root: bash .github/workflows/lib/ownership-coverage.test.sh
set -uo pipefail

SCRIPT="$(dirname "$0")/detect-impacted-playbooks.sh"
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT"

ORPHANS=()

# Probe one representative path per input surface. The detector maps by dir /
# vars-name / role-name, so the probe filename is irrelevant.
probe() {
  local path="$1" rc
  printf '%s\n' "$path" | bash "$SCRIPT" >/dev/null 2>&1
  rc=$?
  # exit 3 == unmapped-and-not-allowlisted; anything else (0 map, 0 no-op) is fine
  [[ $rc -eq 3 ]] && ORPHANS+=("$path")
}

for d in files/*/; do
  probe "${d}__probe"
done
for f in vars/vars_*.yaml; do
  [[ -e "$f" ]] || continue
  probe "$f"
done
for r in roles/*/; do
  probe "${r}defaults/main.yml"
done

if [[ ${#ORPHANS[@]} -gt 0 ]]; then
  echo "FAIL: ${#ORPHANS[@]} input(s) map to no playbook and are not allowlisted:"
  printf '  - %s\n' "${ORPHANS[@]}"
  echo ""
  echo "Wire each to an owning playbook (files/<svc>, service: <svc>, or a"
  echo "playbook named <svc>.yaml), or add it to is_known_unowned() in"
  echo "detect-impacted-playbooks.sh if it is genuinely deployed by nothing."
  exit 1
fi

echo "PASS: every files/, vars/, and roles/ input resolves to an owner or a declared no-op."
