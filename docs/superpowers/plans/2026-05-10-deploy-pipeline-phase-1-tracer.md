# Deploy Pipeline Phase 1 — Tracer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the orchestrator-replay sledgehammer in `main-apply.yml` with file-pattern-driven narrow detection, add per-host concurrency groups to deploy workflows, and validate the chain end-to-end with three `gh`-driven tracer commits (homepage playbook edit, runner playbook edit, homepage nginx vhost edit).

**Architecture:** Two shell helpers under `.github/workflows/lib/` (one for "which playbooks does this change set imply", one for "which concurrency group does this playbook set imply"), each test-driven with sample-input fixtures so the logic is verifiable without pushing. The helpers are wired into `main-apply.yml`'s existing two-job structure (`detect-changes` → `apply-playbooks`); concurrency is set at the apply job level so it can reference detect outputs. Static concurrency groups are added to the auto-targeted dispatch workflows (`deploy-ocean-service.yml`, `deploy-terrac-com.yml`, `deploy-critical-service.yml`).

**Tech Stack:** Bash + jq, GitHub Actions, `gh` CLI for validation, Ansible playbooks (no changes to playbooks themselves in this phase).

---

## File structure

**Create:**
- `.github/workflows/lib/detect-impacted-playbooks.sh` — given changed-file list on stdin (one path per line), emits compact JSON array of playbook paths to apply. Implements the spec's narrow-detection table with a documented fallback to orchestrator replay (`01_base_system.yaml`, `02_core_infrastructure.yaml`, `03_ocean_services.yaml`).
- `.github/workflows/lib/detect-impacted-playbooks.test.sh` — bash test runner with `assert_eq` helper; runs the script against fixture inputs and asserts expected JSON outputs.
- `.github/workflows/lib/resolve-concurrency-group.sh` — given playbook-paths JSON on stdin, emits the concurrency-group key (e.g., `deploy-ocean`, `deploy-dns01-ocean`). Multi-host playbooks use sorted joined hosts.
- `.github/workflows/lib/resolve-concurrency-group.test.sh` — same pattern.
- `docs/operations/deploy-tracer.md` — the runbook: what to push, what `gh` commands to run, what to expect, how to interpret failures.

**Modify:**
- `.github/workflows/main-apply.yml` — replace the inline detection in the `detect-changes` job with a call to `detect-impacted-playbooks.sh`; add an output for the concurrency-group key; add `concurrency:` block on the `apply-playbooks` job.
- `.github/workflows/deploy-ocean-service.yml` — add static `concurrency: { group: deploy-ocean, cancel-in-progress: false }` at workflow level.
- `.github/workflows/deploy-terrac-com.yml` — add static `concurrency: { group: deploy-ocean, cancel-in-progress: false }`.
- `.github/workflows/deploy-critical-service.yml` — add `concurrency: { group: deploy-${{ inputs.service }}, cancel-in-progress: false }` at job level (uses input).

**Out of scope (Phase 1):**
- `deploy-services.yml`, `rollback.yml` — manually triggered, low-frequency; concurrency wiring deferred.
- Changing playbooks themselves.
- The fallback sledgehammer remains — only its scope shrinks.

---

## Task 1: Author detect-impacted-playbooks.sh test fixtures and runner

**Files:**
- Create: `.github/workflows/lib/detect-impacted-playbooks.test.sh`
- Create: `.github/workflows/lib/detect-impacted-playbooks.sh` (stub only in this task)

The script's contract: reads changed-file paths from stdin (one per line), writes a compact JSON array of playbook paths to stdout, exits 0. Empty input → `[]`. Unknown paths → trigger fallback (`01_base_system.yaml`, `02_core_infrastructure.yaml`, `03_ocean_services.yaml`). Paths in the JSON output are sorted and deduplicated.

- [ ] **Step 1: Write the failing test runner**

Create `.github/workflows/lib/detect-impacted-playbooks.test.sh`:

```bash
#!/usr/bin/env bash
# Tests for detect-impacted-playbooks.sh.
# Run from repo root: bash .github/workflows/lib/detect-impacted-playbooks.test.sh
set -euo pipefail

SCRIPT="$(dirname "$0")/detect-impacted-playbooks.sh"
PASS=0
FAIL=0

assert_eq() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name"
    echo "  expected: $expected"
    echo "  actual:   $actual"
    FAIL=$((FAIL + 1))
  fi
}

# 1. Empty input → []
out=$(printf '' | bash "$SCRIPT")
assert_eq "empty input" "[]" "$out"

# 2. Single individual playbook → just that playbook
out=$(printf 'playbooks/individual/ocean/services/homepage.yaml\n' | bash "$SCRIPT")
assert_eq "homepage playbook only" \
  '["playbooks/individual/ocean/services/homepage.yaml"]' "$out"

# 3. Cloudflared config file → cloudflared playbook
out=$(printf 'files/cloudflared/config.yaml.j2\n' | bash "$SCRIPT")
assert_eq "cloudflared config" \
  '["playbooks/individual/ocean/network/cloudflared.yaml"]' "$out"

# 4. Nginx vhost file → nginx playbook
out=$(printf 'files/nginx-compose/proxy_hostname_web_proxy.conf\n' | bash "$SCRIPT")
assert_eq "nginx vhost" \
  '["playbooks/individual/ocean/network/nginx_compose.yaml"]' "$out"

# 5. vars_cloudflared.yaml → cloudflared playbook
out=$(printf 'vars/vars_cloudflared.yaml\n' | bash "$SCRIPT")
assert_eq "vars_cloudflared" \
  '["playbooks/individual/ocean/network/cloudflared.yaml"]' "$out"

# 6. inventories → fallback
out=$(printf 'inventories/production/hosts.ini\n' | bash "$SCRIPT")
assert_eq "inventories fallback" \
  '["playbooks/01_base_system.yaml","playbooks/02_core_infrastructure.yaml","playbooks/03_ocean_services.yaml"]' "$out"

# 7. group_vars/all.yaml → fallback
out=$(printf 'group_vars/all.yaml\n' | bash "$SCRIPT")
assert_eq "group_vars all fallback" \
  '["playbooks/01_base_system.yaml","playbooks/02_core_infrastructure.yaml","playbooks/03_ocean_services.yaml"]' "$out"

# 8. Mixed inputs are unioned and deduped
out=$(printf 'playbooks/individual/ocean/services/homepage.yaml\nfiles/nginx-compose/proxy_hostname_web_proxy.conf\nfiles/nginx-compose/proxy_hostname_web_proxy.conf\n' | bash "$SCRIPT")
assert_eq "mixed homepage+nginx (deduped, sorted)" \
  '["playbooks/individual/ocean/network/nginx_compose.yaml","playbooks/individual/ocean/services/homepage.yaml"]' "$out"

# 9. Orchestrator playbook explicit edit → that orchestrator only
out=$(printf 'playbooks/01_base_system.yaml\n' | bash "$SCRIPT")
assert_eq "explicit orchestrator edit" \
  '["playbooks/01_base_system.yaml"]' "$out"

# 10. roles/dns_infrastructure/** → playbooks that reference that role
out=$(printf 'roles/dns_infrastructure/templates/foo.j2\n' | bash "$SCRIPT")
assert_eq "dns_infrastructure role" \
  '["playbooks/individual/core/services/dns_ha_stack.yaml"]' "$out"

# 11. roles/github_docker_runners/** → runner playbook(s)
out=$(printf 'roles/github_docker_runners/defaults/main.yml\n' | bash "$SCRIPT")
assert_eq "github_docker_runners role" \
  '["playbooks/individual/infrastructure/github_docker_runners.yaml"]' "$out"

# 12. README / docs changes are not in the workflow's paths filter,
#     but if smuggled in (e.g., trailing-doc edit), they should be no-op (fallback skipped — empty).
#     We test that .md under playbooks/ does NOT trigger fallback.
out=$(printf 'playbooks/README.md\n' | bash "$SCRIPT")
assert_eq "playbooks markdown ignored" "[]" "$out"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
```

- [ ] **Step 2: Stub the script so tests can be run (and fail loudly)**

Create `.github/workflows/lib/detect-impacted-playbooks.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
# TODO: implement in Task 2
echo "[]"
```

- [ ] **Step 3: Run the tests and observe failures**

Run: `chmod +x .github/workflows/lib/*.sh && bash .github/workflows/lib/detect-impacted-playbooks.test.sh`

Expected: tests 2–11 fail (script returns `[]` for all inputs). Tests 1 and 12 pass. Final line: `Results: 2 passed, 10 failed`. Exit non-zero.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/lib/detect-impacted-playbooks.test.sh .github/workflows/lib/detect-impacted-playbooks.sh
git commit -m "test(ci): fixtures for narrow-detection script (failing)"
```

---

## Task 2: Implement detect-impacted-playbooks.sh

**Files:**
- Modify: `.github/workflows/lib/detect-impacted-playbooks.sh`

- [ ] **Step 1: Write the full implementation**

Replace the contents of `.github/workflows/lib/detect-impacted-playbooks.sh` with:

```bash
#!/usr/bin/env bash
# detect-impacted-playbooks.sh
#
# Reads changed-file paths from stdin (one per line), writes a compact JSON
# array of playbook paths to apply on stdout.
#
# Rules (first match wins per input line):
#   playbooks/individual/**/*.ya?ml      -> that playbook
#   playbooks/0[0-9]_*.ya?ml             -> that orchestrator
#   files/cloudflared/**                 -> cloudflared playbook
#   vars/vars_cloudflared.yaml           -> cloudflared playbook
#   files/nginx-compose/**               -> nginx playbook
#   files/<service-dir>/**               -> playbooks grep-referencing files/<service-dir>
#   roles/<role>/**                      -> playbooks grep-referencing the role name
#   vars/vars_service_ports.yaml         -> playbooks grep-referencing it
#   inventories/**, group_vars/all*.yaml -> fallback (orchestrator replay)
#   anything ending in .md               -> ignored
#   anything else under tracked paths    -> fallback
#
# Output: sorted, deduplicated JSON array. Empty input → "[]".
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

FALLBACK=(
  "playbooks/01_base_system.yaml"
  "playbooks/02_core_infrastructure.yaml"
  "playbooks/03_ocean_services.yaml"
)

CLOUDFLARED_PB="playbooks/individual/ocean/network/cloudflared.yaml"
NGINX_PB="playbooks/individual/ocean/network/nginx_compose.yaml"

declare -A SEEN=()
need_fallback=0

emit() {
  local pb="$1"
  if [[ -z "${SEEN[$pb]:-}" ]]; then
    SEEN["$pb"]=1
  fi
}

# Grep all playbooks (individual + orchestrator) for a literal substring.
# Prints matching playbook paths relative to repo root, NUL-safe.
grep_playbooks() {
  local needle="$1"
  (cd "$REPO_ROOT" && grep -rlF "$needle" playbooks/ 2>/dev/null \
    | grep -E '\.ya?ml$' \
    | grep -v '/tasks/' \
    || true)
}

while IFS= read -r path; do
  [[ -z "$path" ]] && continue

  # Ignored: any markdown
  if [[ "$path" == *.md ]]; then
    continue
  fi

  # Direct playbook edits
  if [[ "$path" =~ ^playbooks/individual/.*\.ya?ml$ ]] \
     && [[ "$path" != playbooks/individual/*/tasks/* ]]; then
    emit "$path"
    continue
  fi
  if [[ "$path" =~ ^playbooks/0[0-9]_.*\.ya?ml$ ]]; then
    emit "$path"
    continue
  fi

  # Cloudflared
  if [[ "$path" == files/cloudflared/* ]] \
     || [[ "$path" == vars/vars_cloudflared.yaml ]]; then
    emit "$CLOUDFLARED_PB"
    continue
  fi

  # Nginx
  if [[ "$path" == files/nginx-compose/* ]]; then
    emit "$NGINX_PB"
    continue
  fi

  # files/<service-dir>/** — find playbooks that reference this dir
  if [[ "$path" =~ ^files/([^/]+)/ ]]; then
    dir="files/${BASH_REMATCH[1]}"
    while IFS= read -r pb; do
      [[ -n "$pb" ]] && emit "$pb"
    done < <(grep_playbooks "$dir")
    continue
  fi

  # roles/<role>/** — grep playbooks for the role name
  if [[ "$path" =~ ^roles/([^/]+)/ ]]; then
    role="${BASH_REMATCH[1]}"
    while IFS= read -r pb; do
      [[ -n "$pb" ]] && emit "$pb"
    done < <(grep_playbooks "$role")
    continue
  fi

  # vars/vars_service_ports.yaml — grep playbooks that reference it
  if [[ "$path" == vars/vars_service_ports.yaml ]]; then
    while IFS= read -r pb; do
      [[ -n "$pb" ]] && emit "$pb"
    done < <(grep_playbooks "vars_service_ports")
    continue
  fi

  # Fallback triggers
  if [[ "$path" == inventories/* ]] \
     || [[ "$path" =~ ^group_vars/all ]] \
     || [[ "$path" == vars/* ]] \
     || [[ "$path" == roles/* ]] \
     || [[ "$path" == files/* ]]; then
    need_fallback=1
    continue
  fi

  # Anything else: fallback (defensive)
  need_fallback=1
done

if [[ $need_fallback -eq 1 && ${#SEEN[@]} -eq 0 ]]; then
  for pb in "${FALLBACK[@]}"; do
    emit "$pb"
  done
fi

# Emit sorted, deduped JSON array
if [[ ${#SEEN[@]} -eq 0 ]]; then
  echo "[]"
else
  printf '%s\n' "${!SEEN[@]}" | sort -u | jq -R . | jq -s -c .
fi
```

- [ ] **Step 2: Run the tests**

Run: `bash .github/workflows/lib/detect-impacted-playbooks.test.sh`

Expected: `Results: 12 passed, 0 failed`. Exit zero.

If any test fails, fix the implementation and re-run. Common failure modes:
- Tests 10–11 fail if `grep_playbooks` matches false positives (e.g., the role name appearing in a comment in another playbook). Inspect with: `grep -rlF dns_infrastructure playbooks/`. If false positives appear in `.md` files, the `grep -E '\.ya?ml$'` filter handles them; if in unrelated YAML, narrow the search to `playbooks/individual/`.
- Test 8 fails if the JSON output isn't sorted. The `sort -u` line handles this.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/lib/detect-impacted-playbooks.sh
git commit -m "feat(ci): implement narrow-detection rules for changed-file → playbook mapping"
```

---

## Task 3: Author resolve-concurrency-group.sh test fixtures and runner

**Files:**
- Create: `.github/workflows/lib/resolve-concurrency-group.test.sh`
- Create: `.github/workflows/lib/resolve-concurrency-group.sh` (stub only)

Contract: reads JSON array of playbook paths from stdin (the output of `detect-impacted-playbooks.sh`). For each playbook, extracts the value of the top-level `hosts:` directive. Emits `deploy-<sorted-hosts-joined-by-dash>` on stdout. Empty input → `deploy-none`.

Example: input `["playbooks/individual/ocean/services/homepage.yaml"]` (whose `hosts: ocean`) → `deploy-ocean`. Input listing one ocean and one dns01 playbook → `deploy-dns01-ocean`.

- [ ] **Step 1: Write the failing test runner**

Create `.github/workflows/lib/resolve-concurrency-group.test.sh`:

```bash
#!/usr/bin/env bash
# Tests for resolve-concurrency-group.sh.
# Run from repo root.
set -euo pipefail

SCRIPT="$(dirname "$0")/resolve-concurrency-group.sh"
PASS=0
FAIL=0

assert_eq() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name"
    echo "  expected: $expected"
    echo "  actual:   $actual"
    FAIL=$((FAIL + 1))
  fi
}

# 1. Empty array → deploy-none
out=$(printf '[]\n' | bash "$SCRIPT")
assert_eq "empty array" "deploy-none" "$out"

# 2. Single ocean playbook → deploy-ocean
out=$(printf '["playbooks/individual/ocean/services/homepage.yaml"]\n' | bash "$SCRIPT")
assert_eq "homepage → ocean" "deploy-ocean" "$out"

# 3. Single runner playbook → deploy-github_runners (matches playbook hosts: directive)
out=$(printf '["playbooks/individual/infrastructure/github_docker_runners.yaml"]\n' | bash "$SCRIPT")
assert_eq "github_docker_runners → github_runners" "deploy-github_runners" "$out"

# 4. Two ocean playbooks → deploy-ocean (deduped)
out=$(printf '["playbooks/individual/ocean/services/homepage.yaml","playbooks/individual/ocean/network/nginx_compose.yaml"]\n' | bash "$SCRIPT")
assert_eq "two ocean playbooks deduped" "deploy-ocean" "$out"

# 5. Mixed ocean + runners → deploy-github_runners-ocean (sorted)
out=$(printf '["playbooks/individual/ocean/services/homepage.yaml","playbooks/individual/infrastructure/github_docker_runners.yaml"]\n' | bash "$SCRIPT")
assert_eq "ocean + runners sorted" "deploy-github_runners-ocean" "$out"

# 6. Orchestrator (which uses 'all' or a group like 'ocean') → derived from hosts: line
#    01_base_system targets the 'all' group; expect deploy-all.
out=$(printf '["playbooks/01_base_system.yaml"]\n' | bash "$SCRIPT")
assert_eq "orchestrator 01 → all" "deploy-all" "$out"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
```

- [ ] **Step 2: Stub the script**

Create `.github/workflows/lib/resolve-concurrency-group.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
echo "deploy-none"
```

- [ ] **Step 3: Run tests and observe failures**

Run: `chmod +x .github/workflows/lib/resolve-concurrency-group.sh .github/workflows/lib/resolve-concurrency-group.test.sh && bash .github/workflows/lib/resolve-concurrency-group.test.sh`

Expected: 1 passes (empty array → deploy-none). Tests 2–6 fail. Exit non-zero.

- [ ] **Step 4: Verify expected `hosts:` values by reading the actual playbooks**

Run these to confirm test fixtures match reality:

```bash
grep -E '^[[:space:]]*-?[[:space:]]*hosts:' playbooks/individual/ocean/services/homepage.yaml
grep -E '^[[:space:]]*-?[[:space:]]*hosts:' playbooks/individual/infrastructure/github_docker_runners.yaml
grep -E '^[[:space:]]*-?[[:space:]]*hosts:' playbooks/01_base_system.yaml
```

Expected output:
- `homepage.yaml` → `hosts: ocean`
- `github_docker_runners.yaml` → `hosts: github_runners`
- `01_base_system.yaml` → `hosts: all` (or similar)

If the actual value differs (e.g., the orchestrator uses a different group), update the corresponding `assert_eq` in the test file and re-run Step 3.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/lib/resolve-concurrency-group.test.sh .github/workflows/lib/resolve-concurrency-group.sh
git commit -m "test(ci): fixtures for concurrency-group resolver (failing)"
```

---

## Task 4: Implement resolve-concurrency-group.sh

**Files:**
- Modify: `.github/workflows/lib/resolve-concurrency-group.sh`

- [ ] **Step 1: Write the implementation**

Replace the contents of `.github/workflows/lib/resolve-concurrency-group.sh` with:

```bash
#!/usr/bin/env bash
# resolve-concurrency-group.sh
#
# Reads a JSON array of playbook paths from stdin.
# For each playbook, extracts the value of the first `hosts:` directive.
# Emits a single concurrency-group key on stdout: "deploy-<sorted-hosts-joined-by-dash>"
# Empty input → "deploy-none".
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

input="$(cat)"
[[ -z "$input" ]] && input='[]'

count=$(echo "$input" | jq 'length')
if [[ "$count" -eq 0 ]]; then
  echo "deploy-none"
  exit 0
fi

declare -A HOSTS=()
while IFS= read -r pb; do
  [[ -z "$pb" ]] && continue
  pb_path="$REPO_ROOT/$pb"
  if [[ ! -f "$pb_path" ]]; then
    # Playbook doesn't exist on disk (renamed/deleted) — skip
    continue
  fi
  # Match the first `hosts: <value>` line at any indentation
  host=$(grep -E '^[[:space:]]*-?[[:space:]]*hosts:' "$pb_path" \
         | head -1 \
         | sed -E 's/^[[:space:]]*-?[[:space:]]*hosts:[[:space:]]*//' \
         | sed -E 's/[[:space:]]*(#.*)?$//' \
         | tr -d '"' \
         | tr -d "'")
  if [[ -n "$host" ]]; then
    HOSTS["$host"]=1
  fi
done < <(echo "$input" | jq -r '.[]')

if [[ ${#HOSTS[@]} -eq 0 ]]; then
  echo "deploy-none"
  exit 0
fi

joined=$(printf '%s\n' "${!HOSTS[@]}" | sort -u | paste -sd '-' -)
echo "deploy-$joined"
```

- [ ] **Step 2: Run tests**

Run: `bash .github/workflows/lib/resolve-concurrency-group.test.sh`

Expected: `Results: 6 passed, 0 failed`.

If a test fails, the most likely cause is the actual `hosts:` value differing from the fixture. Re-run the verification commands from Task 3 Step 4 and update the test fixtures (not the implementation) to match reality.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/lib/resolve-concurrency-group.sh
git commit -m "feat(ci): resolve per-host concurrency group from playbook list"
```

---

## Task 5: Wire scripts into main-apply.yml + add job concurrency

**Files:**
- Modify: `.github/workflows/main-apply.yml`

The current `detect-changes` job has its detection logic inline (`git diff HEAD~1 HEAD` → grep + jq). Replace the detection block with a call to the new script, and add a new output for the concurrency-group key. Add a `concurrency:` block on the `apply-playbooks` job that references the new output.

- [ ] **Step 1: Read the current main-apply.yml to confirm structure**

Run: `wc -l .github/workflows/main-apply.yml && grep -n "detect-changes\|apply-playbooks\|verify-deployment\|outputs:\|jobs:" .github/workflows/main-apply.yml`

Confirm the file has three jobs (`detect-changes`, `apply-playbooks`, `verify-deployment`) and the outputs structure matches what's referenced in Step 2 below.

- [ ] **Step 2: Edit `detect-changes` job's "Find changed playbooks" step**

In `.github/workflows/main-apply.yml`, replace the body of the `Find changed playbooks` step (the `run: |` block that currently does inline grep + jq) with:

```yaml
      - name: Find changed playbooks
        id: find-playbooks
        run: |
          set -euo pipefail
          echo "Detecting changed files in last commit..."
          CHANGED_FILES=$(git diff --name-only HEAD~1 HEAD)
          echo "Changed files:"
          echo "$CHANGED_FILES"
          echo ""

          PLAYBOOK_JSON=$(echo "$CHANGED_FILES" \
            | bash .github/workflows/lib/detect-impacted-playbooks.sh)

          COUNT=$(echo "$PLAYBOOK_JSON" | jq 'length')

          CONCURRENCY_GROUP=$(echo "$PLAYBOOK_JSON" \
            | bash .github/workflows/lib/resolve-concurrency-group.sh)

          # Health-check target derivation (preserves the existing targets output)
          TARGETS="[]"
          if echo "$PLAYBOOK_JSON" | jq -e '.[] | select(test("ocean|03_ocean"))' >/dev/null; then
            TARGETS=$(echo "$TARGETS" | jq -c '. + ["ocean"]')
          fi
          if echo "$PLAYBOOK_JSON" | jq -e '.[] | select(test("dns|bind9|dhcp|02_core"))' >/dev/null; then
            TARGETS=$(echo "$TARGETS" | jq -c '. + ["dns"]')
          fi
          if echo "$PLAYBOOK_JSON" | jq -e '.[] | select(test("github.*runner"))' >/dev/null; then
            TARGETS=$(echo "$TARGETS" | jq -c '. + ["runners"]')
          fi

          echo "Playbooks to apply: $PLAYBOOK_JSON"
          echo "Concurrency group: $CONCURRENCY_GROUP"
          echo "Health check targets: $TARGETS"

          echo "playbooks=$PLAYBOOK_JSON" >> "$GITHUB_OUTPUT"
          echo "count=$COUNT" >> "$GITHUB_OUTPUT"
          echo "concurrency-group=$CONCURRENCY_GROUP" >> "$GITHUB_OUTPUT"
          echo "targets=$TARGETS" >> "$GITHUB_OUTPUT"
          if [[ "$COUNT" -gt 0 ]]; then
            echo "has-changes=true" >> "$GITHUB_OUTPUT"
          else
            echo "has-changes=false" >> "$GITHUB_OUTPUT"
          fi
```

- [ ] **Step 3: Add `concurrency-group` to detect-changes outputs**

In `.github/workflows/main-apply.yml`, locate the `outputs:` block under `detect-changes` job (currently has `playbooks`, `has-changes`, `playbook-count`, `targets`). Add a new line:

```yaml
      concurrency-group: ${{ steps.find-playbooks.outputs.concurrency-group }}
```

- [ ] **Step 4: Add `concurrency:` block to apply-playbooks job**

In `.github/workflows/main-apply.yml`, under the `apply-playbooks:` job, immediately after the `runs-on:` line, add:

```yaml
    concurrency:
      group: ${{ needs.detect-changes.outputs.concurrency-group }}
      cancel-in-progress: false
```

- [ ] **Step 5: Validate the workflow YAML locally**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/main-apply.yml'))"` — exits 0 if valid YAML.

Then run: `gh workflow view main-apply.yml --yaml 2>&1 | head -3` — confirms `gh` can parse it (you may need to commit and push for the latest version to be queryable, but local YAML validity is enough here).

- [ ] **Step 6: Commit (do NOT push yet — Task 8 pushes the tracer)**

```bash
git add .github/workflows/main-apply.yml
git commit -m "feat(ci): wire narrow-detection + per-host concurrency into main-apply"
```

---

## Task 6: Add static concurrency to dispatched deploy workflows

**Files:**
- Modify: `.github/workflows/deploy-ocean-service.yml`
- Modify: `.github/workflows/deploy-terrac-com.yml`
- Modify: `.github/workflows/deploy-critical-service.yml`

These three workflows already know their target host upfront (input-driven), so concurrency can be added statically without needing a resolver step.

- [ ] **Step 1: Edit deploy-ocean-service.yml**

In `.github/workflows/deploy-ocean-service.yml`, add at the workflow top level (after `name:` and before `on:`):

```yaml
concurrency:
  group: deploy-ocean
  cancel-in-progress: false
```

- [ ] **Step 2: Edit deploy-terrac-com.yml**

Same insertion as Step 1, in `.github/workflows/deploy-terrac-com.yml`:

```yaml
concurrency:
  group: deploy-ocean
  cancel-in-progress: false
```

- [ ] **Step 3: Edit deploy-critical-service.yml**

In `.github/workflows/deploy-critical-service.yml`, this workflow has a `service` input (DNS or Plex). Add at the workflow top level:

```yaml
concurrency:
  group: deploy-${{ inputs.service }}
  cancel-in-progress: false
```

- [ ] **Step 4: Validate all three workflows parse**

```bash
for f in .github/workflows/deploy-ocean-service.yml .github/workflows/deploy-terrac-com.yml .github/workflows/deploy-critical-service.yml; do
  python3 -c "import yaml; yaml.safe_load(open('$f'))" && echo "OK: $f"
done
```

Expected: three `OK:` lines.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/deploy-ocean-service.yml .github/workflows/deploy-terrac-com.yml .github/workflows/deploy-critical-service.yml
git commit -m "feat(ci): add static per-host concurrency to dispatched deploy workflows"
```

---

## Task 7: Write the deploy-tracer runbook

**Files:**
- Create: `docs/operations/deploy-tracer.md`

This document is the user-facing record of what "validate the chain" looks like. It must contain the exact `gh` commands, expected output shape, and failure-mode interpretation.

- [ ] **Step 1: Write the runbook**

Create `docs/operations/deploy-tracer.md`:

```markdown
# Deploy Pipeline Tracer

Use this to validate the homelab CI/CD chain end-to-end after a change to the
detection or concurrency logic. Each tracer is a single-commit push with a
known small change that should trigger a known small set of playbooks.

## Prerequisites

- `gh` authenticated against `bluefishforsale/homelab`.
- On `master` and up to date with `origin/master`.
- Working tree clean.

## Tracer 1 — Service playbook change (ocean)

Goal: confirm a change to one service playbook triggers only that playbook.

```bash
# 1. Make a no-op edit to the homepage playbook (bump a comment)
sed -i '' '1a\
# tracer: see docs/operations/deploy-tracer.md
' playbooks/individual/ocean/services/homepage.yaml

# 2. Single commit, push
git add playbooks/individual/ocean/services/homepage.yaml
git commit -m "ci(tracer): homepage no-op edit"
git push origin master

# 3. Watch the run
RUN=$(gh run list --workflow=main-apply.yml --limit 1 --json databaseId -q '.[0].databaseId')
echo "Run: $RUN"
gh run watch "$RUN" --exit-status

# 4. Inspect what got applied
gh run view "$RUN" --log | grep -E '^Applying:|^✅ SUCCESS|^❌ FAILED'
```

**Expected:**
- One line: `Applying: playbooks/individual/ocean/services/homepage.yaml`
- One line: `✅ SUCCESS: playbooks/individual/ocean/services/homepage.yaml (...)`
- No other playbooks applied.

**Smoke-test the live site:**
```bash
curl -sI https://homepage.terrac.com   # expect HTTP/2 200
curl -sI http://homepage.home          # expect 200 OK (run from inside the home network)
```

**If the run touches more than the homepage playbook:** the detection rules are too broad. Inspect with:
```bash
gh run view "$RUN" --log | grep "Playbooks to apply"
```

## Tracer 2 — Non-ocean playbook change (runners)

Goal: confirm a change to a non-ocean playbook routes to the correct concurrency group.

```bash
sed -i '' '1a\
# tracer: see docs/operations/deploy-tracer.md
' playbooks/individual/infrastructure/github_docker_runners.yaml

git add playbooks/individual/infrastructure/github_docker_runners.yaml
git commit -m "ci(tracer): runners no-op edit"
git push origin master

RUN=$(gh run list --workflow=main-apply.yml --limit 1 --json databaseId -q '.[0].databaseId')
gh run watch "$RUN" --exit-status
```

**Expected:**
- The apply-playbooks job's concurrency group is `deploy-github_runners` (visible in the run's "Set up job" log).
- Only `playbooks/individual/infrastructure/github_docker_runners.yaml` is applied.

## Tracer 3 — Routing-file edit (nginx vhost)

Goal: confirm a change to `files/nginx-compose/*` triggers only the nginx playbook, not the full orchestrator.

```bash
# Edit the homepage nginx vhost block — bump a comment within the file
# (e.g., add `# tracer 3` near the homepage server block)
$EDITOR files/nginx-compose/proxy_hostname_web_proxy.conf

git add files/nginx-compose/proxy_hostname_web_proxy.conf
git commit -m "ci(tracer): nginx vhost no-op edit"
git push origin master

RUN=$(gh run list --workflow=main-apply.yml --limit 1 --json databaseId -q '.[0].databaseId')
gh run watch "$RUN" --exit-status
gh run view "$RUN" --log | grep -E '^Applying:|^✅ SUCCESS'
```

**Expected:**
- One line: `Applying: playbooks/individual/ocean/network/nginx_compose.yaml`
- Concurrency group: `deploy-ocean`.
- No `01_base_system`, `02_core_infrastructure`, or `03_ocean_services` orchestrator runs.

**Then confirm the live site is still up:**
```bash
curl -sI https://homepage.terrac.com
```

## Failure interpretation

- **`gh run watch` exits non-zero**: read `gh run view "$RUN" --log-failed`. Common causes: vault password file missing on runner, SSH key mounted read-only with wrong perms, target host unreachable.
- **Concurrency group is `deploy-none`**: the resolver couldn't extract `hosts:` from the playbook. Likely a playbook with a non-standard `hosts:` indentation. Check `playbooks/individual/.../that.yaml` for the `hosts:` line.
- **Multiple playbooks applied when only one was expected**: detection rule too broad. Add the regression to `.github/workflows/lib/detect-impacted-playbooks.test.sh` and tighten the script.
```

- [ ] **Step 2: Commit the runbook**

```bash
git add docs/operations/deploy-tracer.md
git commit -m "docs(ops): deploy-tracer runbook for validating CI/CD chain"
```

---

## Task 8: Push the foundation commits + run Tracer 1

**Files:** none modified in this task (push + observation only)

- [ ] **Step 1: Confirm local commits**

Run: `git log origin/master..HEAD --oneline`

Expected: 7 commits from Tasks 1, 2, 3, 4, 5, 6, 7.

- [ ] **Step 2: Push**

```bash
git push origin master
```

Note: the foundation commits touch only `.github/workflows/**` and `docs/**`. Neither path is in `main-apply.yml`'s `paths:` filter, so this push will **not** trigger `main-apply.yml`. Confirm:

```bash
sleep 5
gh run list --workflow=main-apply.yml --limit 3 --json status,headSha,event
```

Expected: no run with `headSha` matching the latest push commit. (The `ci-validate.yml` workflow may run; that's fine.)

- [ ] **Step 3: Run Tracer 1 per the runbook**

Follow `docs/operations/deploy-tracer.md` "Tracer 1" exactly. Do not skip the smoke-test curls.

- [ ] **Step 4: Verify success criteria**

- `gh run watch` exited 0.
- `gh run view "$RUN" --log | grep "Applying:"` shows exactly one line: the homepage playbook.
- `curl -sI https://homepage.terrac.com` returns 200.
- `curl -sI http://homepage.home` returns 200 (from inside the home network).

If any of the above fails, **stop**. Do not run Tracers 2 or 3. Diagnose with `gh run view "$RUN" --log-failed` and either fix the detection script (Task 2) or fix the workflow wiring (Task 5), commit the fix, and re-run Tracer 1.

---

## Task 9: Run Tracer 2 (non-ocean playbook)

**Files:** modifies `playbooks/individual/infrastructure/github_docker_runners.yaml` (no-op comment edit)

- [ ] **Step 1: Follow runbook Tracer 2 exactly**

- [ ] **Step 2: Verify success criteria**

- Concurrency group in the run log reads `deploy-github_runners`.
- Exactly one playbook applied: `playbooks/individual/infrastructure/github_docker_runners.yaml`.
- Run exits 0.

If concurrency group is wrong (e.g., `deploy-none` or `deploy-ocean`), inspect with:
```bash
gh run view "$RUN" --log | grep -A 2 "Concurrency"
```
Then check `resolve-concurrency-group.sh` against the actual `hosts:` value in `github_docker_runners.yaml`.

---

## Task 10: Run Tracer 3 (routing-file edit)

**Files:** modifies `files/nginx-compose/proxy_hostname_web_proxy.conf` (no-op comment in the homepage server block)

- [ ] **Step 1: Follow runbook Tracer 3 exactly**

- [ ] **Step 2: Verify success criteria — the critical Phase 1 gate**

- Exactly one playbook applied: `playbooks/individual/ocean/network/nginx_compose.yaml`.
- **No orchestrator playbook runs** (`01_base_system.yaml`, `02_core_infrastructure.yaml`, `03_ocean_services.yaml` must not appear in the Applying lines).
- `curl -sI https://homepage.terrac.com` still returns 200.

This is the criterion that proves the sledgehammer was removed. If an orchestrator runs, the fallback logic in `detect-impacted-playbooks.sh` is too broad (probably matching `files/nginx-compose/*` as a generic `files/*` fallback). Inspect:
```bash
echo 'files/nginx-compose/proxy_hostname_web_proxy.conf' | bash .github/workflows/lib/detect-impacted-playbooks.sh
```
Should print exactly `["playbooks/individual/ocean/network/nginx_compose.yaml"]`. If it doesn't, the rule ordering in the script is wrong — the nginx-compose case is being shadowed by the catch-all fallback. Fix Task 2's script (move the specific rules above the fallback), add a regression test in Task 1's test runner, commit, push, and re-run Tracer 3.

---

## Task 11: Phase 1 sign-off

**Files:** none modified

- [ ] **Step 1: Confirm all three tracers landed green**

```bash
gh run list --workflow=main-apply.yml --limit 5 --json conclusion,headBranch,event,createdAt,displayTitle
```

Expected: the three most recent `main-apply` runs (one per tracer) all show `conclusion: success`.

- [ ] **Step 2: Confirm Phase 1 "done" criteria from the spec are met**

From `docs/superpowers/specs/2026-05-10-deploy-pipeline-shape-up-design.md`:

> **Phase 1 done when:** both tracer commits land green, the runbook is checked in, and a deliberate "edit homepage routing" tracer (touching only `files/nginx-compose/proxy_hostname_web_proxy.conf` for the homepage block) runs *only* the nginx playbook — not the full orchestrator chain.

Tick off:
- [x] All three tracer commits land green (Tasks 8–10).
- [x] `docs/operations/deploy-tracer.md` checked in on master (Task 7).
- [x] Tracer 3 runs only `nginx_compose.yaml` — no orchestrator (Task 10).

- [ ] **Step 3: Tag the foundation**

Tag the master commit that lands Tracer 3 as the Phase 1 baseline (this becomes the reference point for Phase 2's `render_diff.sh` migrations):

```bash
git tag -a phase-1-tracer -m "Phase 1 baseline: narrow-detection + per-host concurrency validated end-to-end"
git push origin phase-1-tracer
```

Phase 1 is complete. Phase 2 (service registry) can begin from this commit.
