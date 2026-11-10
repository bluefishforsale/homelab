#!/usr/bin/env bash
# The [macos] hosts have no apt, systemd or docker, so a play aimed at `all`
# fails on them (node_exporter.yaml did on every deploy until 2026-11-10). Any
# play whose host pattern starts from `all` has to subtract macos. Plays meant
# for a Mac target the macos group by name instead.
# Run from repo root: bash .github/workflows/lib/playbooks-exclude-macos.test.sh
set -euo pipefail

bad=$(grep -rnE '^[[:space:]-]*hosts:[[:space:]]*["'\'']?all([:"'\''[:space:]]|$)' playbooks \
  --include='*.yaml' --include='*.yml' \
  | grep -v '^playbooks/archive/' | grep -v '^playbooks/deprecated/' \
  | grep -v '!macos' || true)

if [ -n "$bad" ]; then
  echo "FAIL: these plays target all without excluding macos; use all:!macos:"
  echo "$bad" | sed 's/^/  /'
  exit 1
fi
echo "PASS: every play aimed at all excludes macos"
