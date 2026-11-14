#!/usr/bin/env bash
# Picks the commit a push deploy diffs from: the `deployed` branch, which only
# moves after an apply with no failures. Diffing from it instead of HEAD~1
# carries a cancelled or failed run's changes into the next run, where a
# one-commit diff dropped them for good.
# Usage: deploy-base.sh <head> [remote]   prints a commit sha
set -euo pipefail

head="$1"
remote="${2:-origin}"
marker="$(git rev-parse -q --verify "refs/remotes/$remote/deployed^{commit}" || true)"

# NOTE: a marker that is not an ancestor of head (history rewritten, branch
# moved by hand) can't bound a range; fall back to the one-commit diff.
if [[ -n "$marker" ]] && git merge-base --is-ancestor "$marker" "$head"; then
  echo "$marker"
  exit 0
fi
git rev-parse "$head~1"
