#!/usr/bin/env bash
# Moves the `deployed` branch on the remote to the commit a clean apply
# covered. It is a plain push, so the remote itself refuses anything but a
# fast-forward: a slower run can never drag the marker back over changes a
# newer run already applied.
# Usage: mark-deployed.sh <sha> [remote]
set -euo pipefail

sha="$1"
remote="${2:-origin}"
git fetch -q "$remote" deployed 2>/dev/null || true
current="$(git ls-remote "$remote" refs/heads/deployed | cut -f1)"

if [[ -n "$current" ]] && git merge-base --is-ancestor "$sha" "$current" 2>/dev/null; then
  echo "deployed already covers $sha (at $current)"
  exit 0
fi
git push "$remote" "$sha:refs/heads/deployed"
echo "deployed moved ${current:-<none>} -> $sha"
