#!/usr/bin/env bash
# Mark a directory as trusted for Claude Code, headlessly.
# Rendered from files/agentbox/trust-dir.sh.
#
# Claude Code gates every workspace behind an interactive "trust this folder?"
# dialog that remote-control / `claude -p` cannot answer (no TTY). There is no
# documented setting or flag to bypass it, but the accepted state persists in
# ~/.claude.json under projects["<dir>"].hasTrustDialogAccepted. We set that key
# directly so RC sessions and the escalate lane's dynamic worktrees start
# without a human accepting each folder.
#
# ponytail: undocumented key, re-verify after a Claude Code upgrade — if trust
# prompts return, the schema in ~/.claude.json changed.
set -euo pipefail

dir="$1"
cfg="${HOME}/.claude.json"

# No config yet means Claude Code has never run / not logged in; nothing to do.
[ -f "$cfg" ] || exit 0

python3 - "$cfg" "$dir" <<'PY'
import json, sys
cfg, dir = sys.argv[1], sys.argv[2]
with open(cfg) as f:
    d = json.load(f)
proj = d.setdefault("projects", {}).setdefault(dir, {})
if proj.get("hasTrustDialogAccepted") is not True:
    proj["hasTrustDialogAccepted"] = True
    tmp = cfg + ".tmp"
    with open(tmp, "w") as f:
        json.dump(d, f, indent=2)
    import os
    os.replace(tmp, cfg)
PY
