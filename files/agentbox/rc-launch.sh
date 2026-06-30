#!/usr/bin/env bash
# Launch `claude remote-control` for a repo under systemd.
# Rendered from files/agentbox/rc-launch.sh. Arg $1 = repo name (= dir under
# ~/repos and the --name shown in claude.ai/code).
#
# Two headless gates Claude Code has no flag for:
#  1. Per-folder workspace trust -> seed it (trust-dir.sh).
#  2. "Enable Remote Control? (y/n)" is re-asked on EVERY launch and needs a
#     TTY; cached grove_enabled does not suppress it. Run under `expect` to
#     give it a pty, auto-answer y, then block on eof so the unit stays active.
#
# ponytail: auto-confirming the prompt + the undocumented trust key are both
# unsupported community patterns; re-verify after a Claude Code upgrade.
set -euo pipefail

repo="$1"
/usr/local/bin/agentbox-trust-dir.sh "${HOME}/repos/${repo}"

exec expect -c "
  set timeout -1
  spawn -noecho claude remote-control --name ${repo}
  expect {
    \"Enable Remote Control?\" { send \"y\r\"; exp_continue }
    eof
  }
"
