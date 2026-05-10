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
' playbooks/individual/ocean/services/terrac_com.yaml

# 2. Single commit, push
git add playbooks/individual/ocean/services/terrac_com.yaml
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
- One line: `Applying: playbooks/individual/ocean/services/terrac_com.yaml`
- One line: `✅ SUCCESS: playbooks/individual/ocean/services/terrac_com.yaml (...)`
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
