#!/usr/bin/env bash
# Push an ntfy alert when a premium lane's Claude session has expired.
#
# The premium lanes run Claude Code on the Pro/Max subscription with
# ANTHROPIC_API_KEY unset. That OAuth session expires every few weeks and cannot
# refresh headlessly, so the only fix is a human running `claude /login` on this
# box. Until 2026-08-31 that failure was invisible: the scripts swallowed it with
# `|| true`, the systemd oneshot still exited 0, and the lane looked idle rather
# than dead for weeks at a time.
#
# Called by the RC watchdog, the only place Claude Code still runs unattended
# enough to strand silently. The issue/escalation lanes moved to opencode and
# free models, which have no OAuth to expire.
#
# Usage: agentbox-notify-auth-expired.sh <context> <log text>
# Exits 0 and pushes nothing unless the output actually names an auth failure,
# so callers can pipe every failure through it without filtering first.
set -uo pipefail

context="${1:-premium lane}"
text="${2:-}"

grep -qiE 'OAuth session expired|Failed to authenticate|Invalid API key|Please run /login' <<<"$text" || exit 0

: "${ALERT_NTFY_URL:?}"

# An expired session fails on every issue in the queue, every hour, and the
# roadmap reconciler on top. Without a throttle one expiry means dozens of
# identical pushes a day.
# NOTE: the window is fixed, so a re-expiry inside it is silent. Re-login is a
# manual act anyway, so the failure mode is "you already know", not "you miss it".
STAMP="{{ home }}/.cache/agentbox/auth-expired-notified"
WINDOW=43200  # 12h
mkdir -p "$(dirname "$STAMP")"
if [ -f "$STAMP" ]; then
  age=$(( $(date +%s) - $(stat -c %Y "$STAMP") ))
  [ "$age" -lt "$WINDOW" ] && exit 0
fi

if curl -sf --max-time 10 \
    -u "${ALERT_NTFY_USER:-}:${ALERT_NTFY_PASS:-}" \
    -H "Title: agentbox premium lane needs a login" \
    -H "Priority: high" \
    -H "Tags: warning,key" \
    -d "Claude OAuth expired ($context).

The premium lanes (escalate + roadmap reconcile) are dead until someone runs:
  ssh agentbox
  claude /login" \
    "$ALERT_NTFY_URL" >/dev/null; then
  touch "$STAMP"
  echo "ntfy: pushed auth-expired alert ($context)" >&2
else
  # Deliberately not fatal: a lane that cannot warn you is still a lane that
  # should keep trying its next issue.
  echo "ntfy: push FAILED for auth-expired alert ($context)" >&2
fi
