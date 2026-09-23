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
# The spawn-mode prompt ([1] same-dir / [2] worktree) that newer Claude Code
# shows on first launch of a project IS flagged: pass --spawn=same-dir so a
# fresh lane doesn't hang forever waiting for a keypress (it never connects,
# no :443 socket). same-dir matches the previous default behaviour.
#
# ponytail: auto-confirming the prompt + the undocumented trust key are both
# unsupported community patterns; re-verify after a Claude Code upgrade.
set -euo pipefail

repo="$1"

# A remote-control session reads whatever this checkout was last left in, and
# nothing else refreshes it. repos/homelab sat 34 commits behind master on a
# feature branch that never merged, and a session reasoning from it reported
# code as absent that had landed weeks earlier: a grep over a stale tree is an
# artifact, not an absence. The issue-watcher lanes already cut every branch
# from a fresh origin/HEAD; only this path lacked it.
#
# Fetch unconditionally so origin/* is current even when the tree cannot be
# moved. Fast-forward only a clean checkout sitting on the default branch:
# a dirty tree or a feature branch is someone's work in progress, and silently
# moving it would be worse than the staleness. Every refusal says so on stderr
# so the reason is in the journal rather than inferred later.
refresh_checkout() {
  local dir="$1" default branch behind
  [ -d "$dir/.git" ] || return 0

  if ! git -C "$dir" fetch -q --prune origin 2>/dev/null; then
    echo "WARN: fetch failed for ${dir}; treat this checkout as possibly stale" >&2
    return 0
  fi

  # origin/HEAD is absent on clones made with --single-branch; ask the remote.
  default=$(git -C "$dir" symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null) || default=""
  if [ -z "$default" ]; then
    git -C "$dir" remote set-head -a origin >/dev/null 2>&1 || true
    default=$(git -C "$dir" symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null) || default=""
  fi
  [ -n "$default" ] || { echo "WARN: ${dir} has no origin/HEAD; not fast-forwarding" >&2; return 0; }
  default=${default#origin/}

  # --untracked-files=no on purpose: a fast-forward preserves untracked files,
  # and repos/ accumulates them (a stray scripts/*.sh sat there for days).
  # Counting them as "dirty" would pin the checkout stale forever, which is the
  # bug this function exists to fix. Only tracked modifications block.
  if [ -n "$(git -C "$dir" status --porcelain --untracked-files=no)" ]; then
    echo "WARN: ${dir} has uncommitted tracked changes; fetched but not fast-forwarded" >&2
    return 0
  fi

  branch=$(git -C "$dir" rev-parse --abbrev-ref HEAD)
  if [ "$branch" != "$default" ]; then
    behind=$(git -C "$dir" rev-list --count "HEAD..origin/${default}" 2>/dev/null || echo 0)
    echo "WARN: ${dir} is on ${branch}, not ${default} (${behind} commit(s) behind); fetched but not fast-forwarded" >&2
    return 0
  fi

  behind=$(git -C "$dir" rev-list --count "HEAD..origin/${default}" 2>/dev/null || echo 0)
  [ "$behind" -gt 0 ] || return 0
  if git -C "$dir" merge --ff-only "origin/${default}" >/dev/null 2>&1; then
    echo "refreshed ${dir}: fast-forwarded ${behind} commit(s) to origin/${default}"
  else
    echo "WARN: ${dir} is ${behind} commit(s) behind origin/${default} and would not fast-forward" >&2
  fi
}

refresh_checkout "${HOME}/repos/${repo}"
/usr/local/bin/agentbox-trust-dir.sh "${HOME}/repos/${repo}"

# One launch attempt under a pty, exiting with the child's status so the caller
# can branch on it. expect exits 0 on a failed child, hence the `catch wait`.
rc() {
  expect -c "
    set timeout -1
    spawn -noecho claude remote-control $1
    expect {
      \"Enable Remote Control?\" { send \"y\r\"; exp_continue }
      eof
    }
    catch wait result
    exit [lindex \$result 3]
  "
}

# A lane killed by systemd can die before it deregisters with claude.ai, and the
# replacement then loses the folder to the corpse, exit 1 on:
#   Error: Registration: Failed with status 409: This folder is already served
#   by a terminal `claude remote-control` on this device. Stop it first.
# Restart=always then re-409s every 30s until the server-side registration ages
# out (observed: 6 restarts / ~3min for photonic_inventory). --continue reclaims
# whatever session was last recorded here instead of racing that corpse, and it
# refuses only while a LIVE local pid still holds the session, so it is exactly
# the stale-registration key.
#
# NOTE: order matters, do not flip these. --continue reattaches to a SESSION,
# not to the server: it comes up "Single session, exits when complete" where a
# fresh launch comes up "Capacity: 0/32, new sessions created in the current
# directory". Trying it first would quietly demote every lane to single-session
# for as long as anything is recorded (~4h window). Fresh first keeps the normal
# path byte-identical to before; --continue is the recovery path only, and once
# that reclaimed session ends the next restart gets a clean multi-session server.
# --continue is also rejected alongside --spawn/--capacity/--create-session-in-dir,
# so it cannot carry --spawn=same-dir; it adopts the recorded session's mode and
# shows no spawn-mode prompt.
rc "--name ${repo} --spawn=same-dir" || rc --continue
