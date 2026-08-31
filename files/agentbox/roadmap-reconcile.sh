#!/usr/bin/env bash
# agentbox roadmap reconciler (premium lane).
# Rendered from files/agentbox/roadmap-reconcile.sh by
# playbooks/individual/agentbox/agentbox.yaml.
#
# Daily: reconcile bluefishforsale/homelab ROADMAP.md against real fleet state.
# Moves Todo items that are actually DEPLOYED (a matching playbook exists) AND
# HEALTHY (scrape target up==1, not named by a DOWN target or firing alert) into
# Completed, and appends live work that is not listed. Never adds speculative
# future items. The fuzzy matching runs on the strongest free model via
# opencode; the evidence is gathered here and passed in.
# Result lands as ONE rolling PR a human merges -- homelab is infra, never
# auto-merged (ADR 0001).
set -euo pipefail

OWNER="bluefishforsale"
REPO="homelab"
SLUG="$OWNER/$REPO"
BRANCH="agent/roadmap-sync"
WORKROOT="{{ home }}/work"
LOGDIR="{{ home }}/agent-logs"
WT="$WORKROOT/$REPO"
EV="$LOGDIR/roadmap-evidence.txt"   # outside the worktree: never committed
PROM="${HOMELAB_PROM:-http://192.168.1.143:9090}"
ALERT="${HOMELAB_ALERTMANAGER:-http://192.168.1.143:9093}"

# shellcheck disable=SC1091
source "{{ home }}/.config/agentbox/agentbox.env"
mkdir -p "$WORKROOT" "$LOGDIR"
# Strongest free model. This runs once a day, so the quota cost is trivial.
RECONCILE_MODEL="google/gemini-3.1-pro-preview"
export OTEL_RESOURCE_ATTRIBUTES="repo=$REPO,lane=reconcile,service=agentbox"

# Rolling agent branch, always rebased on latest master (like renovate). No human
# commits land here, so the daily force-push is safe and keeps a single clean PR.
[ -d "$WT/.git" ] || gh repo clone "$SLUG" "$WT" -- -q
git -C "$WT" fetch -q origin
git -C "$WT" checkout -q -B "$BRANCH" origin/HEAD

# --- Evidence: what is deployed AND healthy right now ---
{
  echo "# Fleet reality (gathered $(date -u +%FT%TZ))"
  echo
  echo "## Healthy Prometheus scrape targets (up==1), count by job:"
  curl -s -m 10 -G --data-urlencode 'query=count by (job) (up==1)' "$PROM/api/v1/query" \
    | jq -r '.data.result[]? | "  - \(.metric.job): \(.value[1]) up"' 2>/dev/null | sort || echo "  (Prometheus unreachable)"
  echo
  echo "## DOWN targets (up==0) -- treat these as NOT healthy:"
  curl -s -m 10 -G --data-urlencode 'query=up==0' "$PROM/api/v1/query" \
    | jq -r '.data.result[]? | "  - \(.metric.job) @ \(.metric.instance)"' 2>/dev/null | sort || true
  echo
  echo "## Firing alerts -- a service named here is NOT healthy:"
  curl -s -m 10 "$ALERT/api/v2/alerts?active=true&silenced=false&inhibited=false" \
    | jq -r '.[]? | "  - \(.labels.alertname) [\(.labels.severity // "?")] \(.labels.job // .labels.service // "")"' 2>/dev/null | sort -u || echo "  (Alertmanager unreachable)"
  echo
  echo "## Deployed services (ansible playbooks in the repo):"
  ( cd "$WT" && git ls-files 'playbooks/individual/*' | grep -E '\.ya?ml$' | sed -E 's#playbooks/individual/##; s#\.ya?ml$##' | sort -u ) || true
} > "$EV"

# --- Reconcile with the free escalation model (headless) ---
prompt="Reconcile ./ROADMAP.md for the $SLUG homelab repo against the fleet evidence below. Read ./ROADMAP.md first.

Make ONLY these surgical edits with the Edit tool:
1. If an item under a '## Todo' heading (or a 'Todo'/'In Progress' row in the Vision table) is shown by the evidence to be DEPLOYED (a matching playbook exists) AND HEALTHY (its target is up==1 and it is not named by a DOWN target or firing alert), move that single line into the best-matching subsection under '## Completed' (or flip its Vision-table Status to 'Done'). Only for items the evidence clearly justifies.
2. Append services that are clearly live in the evidence but appear NOWHERE in ROADMAP.md as bullets under the best-fitting '## Completed' subsection.

Hard rules:
- NEVER add, invent, reword, or speculate about any Todo / future / planned item. If an item lacks evidence, leave it untouched in Todo.
- Do NOT reorder, reformat, or edit any line you are not explicitly moving or adding. Keep the diff minimal and surgical.
- Base every change strictly on the evidence. When unsure, change nothing.

Evidence:
$(cat "$EV")"

# Failures are logged, never swallowed: this used to exit 0 on a dead model and
# look exactly like "the roadmap was already in sync".
( cd "$WT" && timeout 900 opencode run -m "$RECONCILE_MODEL" "$prompt" ) \
  >"$LOGDIR/roadmap-reconcile.log" 2>&1 \
  || echo "roadmap reconcile failed on $RECONCILE_MODEL (see $LOGDIR/roadmap-reconcile.log)" >&2

# --- Open/refresh the single rolling PR only if the roadmap actually changed ---
if [ -z "$(git -C "$WT" status --porcelain ROADMAP.md)" ]; then
  echo "roadmap already in sync; nothing to do"
  exit 0
fi
git -C "$WT" add ROADMAP.md
git -C "$WT" commit -q -m "docs(roadmap): reconcile against live fleet state"
git -C "$WT" push -q -f -u origin "$BRANCH"
if ! gh pr view "$BRANCH" --repo "$SLUG" --json state --jq .state 2>/dev/null | grep -q OPEN; then
  gh pr create --repo "$SLUG" --base master --head "$BRANCH" \
    --title "docs(roadmap): daily reconcile against live fleet" \
    --body "Automated daily reconcile by agentbox. Todo items now deployed+healthy were moved to Completed and live-but-unlisted work was added, strictly from Prometheus/Alertmanager + playbook evidence. No future or speculative items were added. Review the diff and merge (homelab is infra: a human merges, ADR 0001)." || true
fi
