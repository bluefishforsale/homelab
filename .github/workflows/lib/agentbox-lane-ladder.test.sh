#!/usr/bin/env bash
# The cost model of the whole fleet is "cheapest model that can do the job",
# and it only holds if the ladder is actually ordered and actually free.
# Run from repo root: bash .github/workflows/lib/agentbox-lane-ladder.test.sh
#
# The expensive failure this guards against is silent: if the ladder collapses
# to one tier, or a paid model creeps in, nothing breaks and no alert fires —
# the bill just changes. So assert the shape.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WATCHER="$ROOT/files/agentbox/issue-watcher.sh"
ESCALATE="$ROOT/files/agentbox/escalate.sh"
RECONCILE="$ROOT/files/agentbox/roadmap-reconcile.sh"
CONF="$ROOT/files/agentbox/opencode.json.j2"
PASS=0
FAIL=0
ok()  { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# 1. The watcher tries the local GPU before it spends anyone's quota.
lanes=$(grep -m1 '^LANES=' "$WATCHER" | cut -d'"' -f2)
case "$lanes" in
  "local-llama/local google/gemini-flash-latest") ok "ladder is local first, then Gemini Flash" ;;
  local-llama/local*) ok "ladder starts local (order: $lanes)" ;;
  *) bad "ladder must try the local GPU first, got: '$lanes'" ;;
esac

# 2. opencode's own defaults must agree. If `model`/`agent.build` point at a
#    hosted model, every invocation WITHOUT an explicit -m silently bills there.
#    Anchored: an unanchored `"model"` also matches `"small_model"`, which is
#    already local, so the check passed against a config pointing everything
#    else at Gemini.
if grep -qE '^[[:space:]]*"model":[[:space:]]*"local-llama/local"' "$CONF"; then
  ok "opencode top-level default model is local"
else
  bad "opencode top-level \"model\" is not local-llama/local"
fi
for agent in plan build; do
  if grep -qE "\"$agent\":[[:space:]]*\{[[:space:]]*\"model\":[[:space:]]*\"local-llama/local\"" "$CONF"; then
    ok "opencode $agent agent defaults to the local model"
  else
    bad "opencode $agent agent does not default to local-llama/local"
  fi
done

# 3. Nothing unattended may call Claude Code. It cannot run shell commands
#    headlessly (acceptEdits covers file edits only) and its OAuth expires
#    every few weeks needing an interactive login, which stalled the lane for a
#    month. It stays installed for interactive RC sessions only.
for f in "$WATCHER" "$ESCALATE" "$RECONCILE"; do
  n=$(basename "$f")
  if grep -nE '^[^#]*\bclaude -p\b' "$f"; then
    bad "$n invokes claude -p; unattended lanes must stay in opencode"
  else
    ok "$n does not invoke Claude Code"
  fi
done

# 4. Every model named anywhere in the lanes must be one of the free ones.
#    This is the line that stops a paid model arriving by accident.
FREE='local-llama/local|google/gemini-flash-latest|google/gemini-3.1-pro-preview'
bad_models=$(grep -hoE '(-m|model=|MODEL=)"?[a-z0-9.-]+/[a-z0-9.-]+' "$WATCHER" "$ESCALATE" "$RECONCILE" \
  | grep -oE '[a-z0-9.-]+/[a-z0-9.-]+' | sort -u | grep -vE "^($FREE)$" || true)
if [ -z "$bad_models" ]; then
  ok "every model referenced by the lanes is on the free list"
else
  bad "non-free model referenced: $(tr '\n' ' ' <<<"$bad_models")"
fi

# 5. Escalation is the end of the automated road: it must hand to a human, not
#    loop an issue back into lanes that already failed it.
if grep -q 'LABEL_HUMAN=' "$ESCALATE"; then
  ok "escalation tier defines a human hand-off label"
else
  bad "escalate.sh has no needs-human label; a failed issue would loop forever"
fi

# 6. Retry policy: infrastructure failures retry, capability failures escalate.
if grep -qE '124|rate limit' "$WATCHER"; then
  ok "watcher distinguishes infrastructure failure from capability failure"
else
  bad "watcher retries blindly or not at all; it must only retry on infra failure"
fi

echo
echo "passed: $PASS  failed: $FAIL"
[[ "$FAIL" -eq 0 ]]
