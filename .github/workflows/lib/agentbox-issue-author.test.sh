#!/usr/bin/env bash
# Tests the agent fleet's issue-author trust boundary.
# Run from repo root: bash .github/workflows/lib/agentbox-issue-author.test.sh
#
# An issue title and body are fed verbatim into the prompt of an agent that
# clones the repo, writes code, pushes and opens a PR. That makes "who wrote
# this issue" a security decision, and it has to hold at every point where the
# fleet picks work up — the free lane's issue list, the failure-driven
# escalation drain, and the premium lane that consumes the escalation label.
# Miss one and the others are decoration.
#
# The jq programs are extracted from the real scripts rather than copied here,
# so the test cannot pass against a script that has drifted.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
WATCHER="$ROOT/files/agentbox/issue-watcher.sh"
ESCALATE="$ROOT/files/agentbox/escalate-to-claude.sh"
ALLOW="bluefishforsale"
PASS=0
FAIL=0

ok()  { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

check() {  # $1 = name; $2 = want; $3 = got
  [[ "$2" == "$3" ]] && ok "$1" || bad "$1 (wanted '$2', got '$3')"
}

# Pull every `--jq --arg allow "$ISSUE_AUTHOR_ALLOWLIST" '<program>'` out of the
# watcher. Both of its selection points must use this form; finding a different
# number than expected means one grew, shrank, or stopped filtering.
mapfile -t PROGS < <(python3 - "$WATCHER" <<'PY'
import re, sys
src = open(sys.argv[1]).read()
# The watcher closes its single quote to interpolate the label names. Do that
# substitution first, exactly as the shell does, or the extraction stops at the
# interpolation quote instead of at the end of the jq program.
for var, val in (("LABEL_WORKING", "agent-working"), ("LABEL_CLAUDE", "needs-claude")):
    src = src.replace("'" + '"$' + var + '"' + "'", val)
pat = r"""jq -r --arg allow "\$ISSUE_AUTHOR_ALLOWLIST" '(.*?)'"""
for prog in re.findall(pat, src, re.S):
    print(prog.replace("\n", " "))
PY
)

check "watcher has two author-filtered selections" 2 "${#PROGS[@]}"

run_prog() {  # $1 = jq program; $2 = json
  printf '%s' "$2" | jq -r --arg allow "$ALLOW" "$1" | tr '\n' ' ' | sed 's/ *$//'
}

if [[ "${#PROGS[@]}" -eq 2 ]]; then
  # Open issues: only an allowlisted author's unclaimed issue is drafted.
  issues='[
    {"number":1,"labels":[],"author":{"login":"bluefishforsale"}},
    {"number":2,"labels":[],"author":{"login":"mallory"}},
    {"number":3,"labels":[{"name":"agent-working"}],"author":{"login":"bluefishforsale"}},
    {"number":4,"labels":[],"author":{"login":"Bluefishforsale"}},
    {"number":5,"labels":[{"name":"needs-claude"}],"author":{"login":"mallory"}}
  ]'
  check "free lane drafts only the owner's unclaimed issue" "1" "$(run_prog "${PROGS[0]}" "$issues")"

  # Red agent PRs: the branch name decides WHICH issue gets escalated, so an
  # outsider who can open a PR named agent/issue-N must not reach this loop.
  #
  # Shaped like the GraphQL response, not gh's --json output. The drain asks for
  # the rollup's aggregate `state` because the per-check `contexts` need the
  # Checks permission, which the Checks API restricts to GitHub Apps — no PAT
  # can ever have it. A null rollup (no CI at all) must not read as red.
  prs='{"data":{"repository":{"pullRequests":{"nodes":[
    {"number":10,"headRefName":"agent/issue-7","author":{"login":"bluefishforsale"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]}},
    {"number":11,"headRefName":"agent/issue-8","author":{"login":"mallory"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]}},
    {"number":12,"headRefName":"agent/issue-9","author":{"login":"bluefishforsale"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS"}}}]}},
    {"number":13,"headRefName":"feature/whatever","author":{"login":"bluefishforsale"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE"}}}]}},
    {"number":14,"headRefName":"agent/issue-5","author":{"login":"bluefishforsale"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]}},
    {"number":15,"headRefName":"agent/issue-6","author":{"login":"bluefishforsale"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"ERROR"}}}]}}
  ]}}}}'
  check "escalation drain ignores an outsider's forged agent/issue-N branch" \
    "10 agent/issue-7 15 agent/issue-6" "$(run_prog "${PROGS[1]}" "$prs")"
fi

# The premium lane is the backstop: it must not trust the needs-claude label
# alone, and it must decide BEFORE the issue body reaches the prompt.
# Anchored on the comparison itself, not on any mention of the variable: the
# fallback assignment near the top of the file also mentions it, and matching
# that would pass with the guard deleted.
guard_line=$(grep -n 'ISSUE_AUTHOR_ALLOWLIST" | grep -qF " \$author "' "$ESCALATE" | head -1 | cut -d: -f1)
body_line=$(grep -n 'body=\$(gh issue view' "$ESCALATE" | head -1 | cut -d: -f1)
if [[ -n "$guard_line" && -n "$body_line" && "$guard_line" -lt "$body_line" ]]; then
  ok "premium lane checks the author before reading the body into a prompt"
else
  bad "premium lane must check ISSUE_AUTHOR_ALLOWLIST before line $body_line (found at ${guard_line:-none})"
fi

# Both scripts must fall back to the owner alone, never to an empty (= allow
# nothing matches, but also never set) or unset value, if the env file is old.
for f in "$WATCHER" "$ESCALATE"; do
  if grep -q 'ISSUE_AUTHOR_ALLOWLIST="${ISSUE_AUTHOR_ALLOWLIST:-\$OWNER}"' "$f"; then
    ok "$(basename "$f") fails closed to the owner when the env file predates the setting"
  else
    bad "$(basename "$f") lacks the :-\$OWNER fallback"
  fi
done

# gh's --jq is not jq. It takes one expression and has no --arg, so
# `gh ... --jq --arg allow ...` makes gh reject the whole command. That shipped
# once and killed the watcher for every repo, silently, because the failure was
# swallowed by `|| continue`. Running the extracted program under real jq (as
# the tests above do) cannot catch it — the bug is in how gh parses its flags,
# not in the jq — so assert the shape directly.
# Matched as a bare string, not anchored to the gh line: the flag lands on a
# backslash continuation, which is how it slipped through the first time.
if grep -n -- '--jq[[:space:]]*--arg' "$WATCHER" "$ESCALATE"; then
  bad "gh --jq does not accept --arg; pipe to real jq instead"
else
  ok "no gh invocation passes --arg to gh's own --jq"
fi

# gh's `--json statusCheckRollup` expands to the per-check contexts list, which
# needs the Checks permission. GitHub restricts the Checks API to GitHub Apps,
# so no PAT can ever hold it and that field is permanently a 403 here. Ask
# GraphQL for the aggregate state instead.
# Must not match the comment above the fix, which names the broken form on
# purpose: require no `#` ahead of the flag on the line.
if grep -nE '^[^#]*--json[^#]*statusCheckRollup' "$WATCHER"; then
  bad "gh --json statusCheckRollup needs Checks, which is GitHub-App-only; query the rollup state via GraphQL"
else
  ok "CI state comes from the rollup aggregate, not the App-only Checks field"
fi

# The same silent-failure shape that hid it: a bare `|| continue` on the fetch
# turns a total outage into a clean exit 0.
if grep -qE "\|\| continue$" "$WATCHER"; then
  bad "watcher swallows a failed fetch with a bare || continue (log it instead)"
else
  ok "a failed fetch is logged, not silently skipped"
fi

# The allowlist is single-sourced in the env template both lanes read.
if grep -q 'ISSUE_AUTHOR_ALLOWLIST=' "$ROOT/files/agentbox/agentbox.env.j2"; then
  ok "allowlist is rendered into the shared env file"
else
  bad "agentbox.env.j2 does not define ISSUE_AUTHOR_ALLOWLIST"
fi

echo
echo "passed: $PASS  failed: $FAIL"
[[ "$FAIL" -eq 0 ]]
