#!/usr/bin/env bash
# Tests that no workflow pastes attacker-reachable text into a `run:` script.
# Run from repo root: bash .github/workflows/lib/workflow-run-injection.test.sh
#
# GitHub substitutes ${{ ... }} into the run block as script SOURCE before bash
# parses it, so a value carrying $(...) or backticks executes. Every deploy here
# takes a repository_dispatch payload whose fields an attacker sets freely, and
# the steps that consume them hold the vault password. Passing the value through
# step-level env: instead makes bash see a variable, and the characters stay data.
#
# This has been fixed three times on the instance and come back twice, because
# the fix was applied to the file that broke rather than to the pattern. The rule
# is absolute on purpose: laundering a payload value through a step output does
# not clean it, so `steps.params.outputs.*` is refused in run text the same as
# the raw event is.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
PASS=0
FAIL=0

ok()  { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# Reports every ${{ ... }} sitting in run: script text whose expression names a
# context an outsider can influence. Prints "file:line<TAB>expression".
scan() {
  python3 - "$@" <<'PY'
import re, sys, pathlib

# Contexts an outsider can set. client_payload/head_commit arrive from the
# event; steps.params.outputs.* is those same fields after a round trip through
# $GITHUB_OUTPUT, which launders nothing.
TAINTED = re.compile(
    r'github\.event\.client_payload|github\.event\.head_commit|'
    r'github\.event\.commits|github\.event\.issue|github\.event\.pull_request|'
    r'github\.event\.comment|github\.head_ref|steps\.params\.outputs')

for path in sys.argv[1:]:
    lines = pathlib.Path(path).read_text().splitlines()
    in_run = False
    run_indent = 0
    for n, line in enumerate(lines, 1):
        stripped = line.strip()
        if re.match(r'^\s*-?\s*run:', line):
            in_run = True
            run_indent = len(line) - len(line.lstrip())
            continue
        if in_run and stripped and (len(line) - len(line.lstrip())) <= run_indent:
            in_run = False
        if in_run:
            for expr in re.findall(r'\$\{\{([^}]*)\}\}', line):
                if TAINTED.search(expr):
                    print(f"{path}:{n}\t{expr.strip()}")
PY
}

mapfile -t WORKFLOWS < <(find "$ROOT/.github/workflows" "$ROOT/docs" -name '*.yml' -o -name '*.yaml' 2>/dev/null | sort)

# A scanner that silently matches nothing would pass this suite against every
# vulnerable file in the repo, so prove it catches a known-bad sample first.
SAMPLE=$(mktemp -d)/bad.yml
cat > "$SAMPLE" <<'YAML'
jobs:
  x:
    steps:
      - name: safe
        env:
          MSG: ${{ github.event.head_commit.message }}
        run: |
          echo "$MSG"
      - name: unsafe
        run: |
          echo "Message: ${{ steps.params.outputs.commit_message }}"
YAML
SAMPLE_HITS=$(scan "$SAMPLE" | wc -l | tr -d ' ')
[ "$SAMPLE_HITS" = "1" ] \
  && ok "scanner flags run-block interpolation and not the env: form" \
  || bad "scanner self-check (wanted 1 hit on the sample, got $SAMPLE_HITS)"
rm -rf "$(dirname "$SAMPLE")"

[ "${#WORKFLOWS[@]}" -gt 0 ] \
  && ok "found ${#WORKFLOWS[@]} workflow files to scan" \
  || bad "found no workflow files (bad ROOT?)"

# Moving a value into env: edits indentation, and a workflow that no longer
# parses does not run this guard either. Cheap to assert, silent when skipped.
UNPARSEABLE=$(for wf in "${WORKFLOWS[@]}"; do
  python3 -c "import yaml,sys; yaml.safe_load(open(sys.argv[1]))" "$wf" 2>/dev/null || echo "$wf"
done)
[ -z "$UNPARSEABLE" ] \
  && ok "every workflow file parses as YAML" \
  || { bad "unparseable workflow YAML"; echo "$UNPARSEABLE" | sed 's/^/       /'; }

HITS=$(scan "${WORKFLOWS[@]}")
if [ -z "$HITS" ]; then
  ok "no workflow interpolates an attacker-reachable value into run: script text"
else
  bad "attacker-reachable values in run: script text"
  echo "$HITS" | sed 's/^/       /'
fi

# Every payload field a workflow reads has to arrive as an env: binding. The
# scan above proves none reach run text; this proves the ones that exist are
# bound rather than reaching some other unquoted context. Keyed on the env
# binding's shape, not on a NAME_ prefix, since the workflows use several.
for wf in "$ROOT"/.github/workflows/*.yml; do
  refs=$(grep -cE '\$\{\{ github\.event\.client_payload\.' "$wf")
  [ "$refs" -gt 0 ] || continue
  name=$(basename "$wf")
  bound=$(grep -cE '^\s+[A-Z][A-Z0-9_]*: \$\{\{ github\.event\.client_payload\.' "$wf")
  [ "$refs" -eq "$bound" ] \
    && ok "$name binds all $refs payload references through env:" \
    || bad "$name binds $bound of $refs payload references through env:"
done

echo
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -eq 0 ]
