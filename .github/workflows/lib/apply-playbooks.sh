#!/usr/bin/env bash
# Applies main-apply's playbook list to one host pattern. main-apply calls it
# twice, once for every host but the runner's own and once, from a runner on
# another host, for that host alone, so no job restarts docker under itself.
# Usage: apply-playbooks.sh '<json array of playbooks>' '<host pattern>'
# Env: EXTRA_VARS (optional), GITHUB_OUTPUT (gets total/passed/failed).
# Writes /tmp/apply-results.txt, /tmp/failed-playbooks.txt and
# /tmp/playbook-N.log; exits 1 if any playbook failed.
set -uo pipefail

PLAYBOOKS="$1"
PATTERN="$2"
INVENTORY="inventories/production/hosts.ini"
: > /tmp/apply-results.txt

EXTRA_VARS_FLAG=()
if [ -n "${EXTRA_VARS:-}" ]; then
  EXTRA_VARS_FLAG=(--extra-vars "$EXTRA_VARS")
fi

# A pattern matching no host makes every play error out on --limit, so treat
# it as nothing to do (a dispatch limited to the runner's own host, say).
HOST_COUNT=$(ansible -i "$INVENTORY" --vault-password-file=/tmp/.vault_pass \
  "$PATTERN" --list-hosts 2>/dev/null | sed -nE 's/^ *hosts \(([0-9]+)\):.*/\1/p')
if [ "${HOST_COUNT:-0}" -eq 0 ]; then
  echo "No hosts match '$PATTERN'; nothing to apply here."
  { echo "total=0"; echo "passed=0"; echo "failed=0"; } >> "$GITHUB_OUTPUT"
  exit 0
fi

echo "Applying to: $PATTERN ($HOST_COUNT hosts)"
TOTAL=0
PASSED=0
FAILED=0
FAILED_PLAYBOOKS=""

for PLAYBOOK in $(echo "$PLAYBOOKS" | jq -r '.[]'); do
  if [ ! -f "$PLAYBOOK" ]; then
    echo "Skipping $PLAYBOOK: gone from the current tip"
    continue
  fi
  TOTAL=$((TOTAL + 1))
  echo "========================================"
  echo "Applying: $PLAYBOOK"
  echo "========================================"
  START_TIME=$(date +%s)

  ansible-playbook \
    -i "$INVENTORY" \
    --vault-password-file=/tmp/.vault_pass \
    "${EXTRA_VARS_FLAG[@]}" \
    --limit "$PATTERN" \
    "$PLAYBOOK" 2>&1 | tee "/tmp/playbook-${TOTAL}.log"
  ANSIBLE_EXIT=${PIPESTATUS[0]}
  DURATION=$(( $(date +%s) - START_TIME ))

  if [ "$ANSIBLE_EXIT" -eq 0 ]; then
    PASSED=$((PASSED + 1))
    HOSTS=$(grep -E "PLAY \[.*\]" "/tmp/playbook-${TOTAL}.log" | head -1 | sed 's/PLAY \[//' | sed 's/\].*//' || echo "N/A")
    echo "| \`$PLAYBOOK\` | ✅ Success | ${DURATION}s | $HOSTS |" >> /tmp/apply-results.txt
    echo ""
    echo "✅ SUCCESS: $PLAYBOOK (${DURATION}s)"
  else
    FAILED=$((FAILED + 1))
    FAILED_PLAYBOOKS="${FAILED_PLAYBOOKS}${PLAYBOOK}\n"
    echo "| \`$PLAYBOOK\` | ❌ Failed | ${DURATION}s | N/A |" >> /tmp/apply-results.txt
    echo ""
    echo "❌ FAILED: $PLAYBOOK (${DURATION}s)"
    echo "Continuing with remaining playbooks..."
  fi
  echo ""
done

{ echo "total=$TOTAL"; echo "passed=$PASSED"; echo "failed=$FAILED"; } >> "$GITHUB_OUTPUT"
if [ -n "$FAILED_PLAYBOOKS" ]; then
  echo -e "$FAILED_PLAYBOOKS" > /tmp/failed-playbooks.txt
fi

echo "========================================"
echo "Deployment Summary ($PATTERN)"
echo "========================================"
echo "Total: $TOTAL"
echo "Passed: $PASSED"
echo "Failed: $FAILED"

if [ "$FAILED" -gt 0 ]; then
  echo "❌ Some playbooks failed to apply"
  exit 1
fi
echo "✅ All playbooks applied successfully"
