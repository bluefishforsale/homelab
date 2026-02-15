#!/bin/bash
# GitHub Issues Setup Script for homelab-gitops
# Sets up labels and creates initial issues for task management

set -e

REPO="bluefishforsale/homelab"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"

if [ -z "$GITHUB_TOKEN" ]; then
  echo "❌ Error: GITHUB_TOKEN environment variable not set"
  echo ""
  echo "To use this script:"
  echo "1. Create a Personal Access Token at: https://github.com/settings/tokens"
  echo "2. Grant 'repo' scope"
  echo "3. Export it: export GITHUB_TOKEN='your_token_here'"
  echo "4. Run this script again"
  exit 1
fi

API_BASE="https://api.github.com"

echo "════════════════════════════════════════════════════════════"
echo "  Setting up GitHub Issues for $REPO"
echo "════════════════════════════════════════════════════════════"
echo ""

# Function to create a label
create_label() {
  local name="$1"
  local color="$2"
  local description="$3"
  
  echo "Creating label: $name"
  
  curl -s -X POST \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    "$API_BASE/repos/$REPO/labels" \
    -d "{\"name\":\"$name\",\"color\":\"$color\",\"description\":\"$description\"}" \
    > /dev/null 2>&1 || echo "  (may already exist)"
}

# Function to create an issue
create_issue() {
  local title="$1"
  local body="$2"
  local labels="$3"
  
  echo ""
  echo "Creating issue: $title"
  
  RESPONSE=$(curl -s -X POST \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    "$API_BASE/repos/$REPO/issues" \
    -d "{\"title\":\"$title\",\"body\":\"$body\",\"labels\":[$labels]}")
  
  ISSUE_NUMBER=$(echo "$RESPONSE" | grep -o '"number":[0-9]*' | head -1 | cut -d: -f2)
  ISSUE_URL=$(echo "$RESPONSE" | grep -o '"html_url":"[^"]*"' | head -1 | cut -d'"' -f4)
  
  if [ -n "$ISSUE_NUMBER" ]; then
    echo "  ✅ Created: Issue #$ISSUE_NUMBER"
    echo "  🔗 $ISSUE_URL"
  else
    echo "  ❌ Failed to create issue"
    echo "  Response: $RESPONSE"
  fi
}

# ═══════════════════════════════════════════════════════════════
# STEP 1: Create Labels
# ═══════════════════════════════════════════════════════════════

echo "Step 1: Creating labels..."
echo ""

create_label "agent:homelab-gitops" "0E8A16" "Tasks for Homelab GitOps agent"
create_label "priority:critical" "D73A4A" "Critical priority"
create_label "priority:high" "FF6B6B" "High priority"
create_label "priority:medium" "FFA94D" "Medium priority"
create_label "priority:low" "94D82D" "Low priority"
create_label "status:blocked" "FBCA04" "Blocked - waiting on external dependency"
create_label "status:in-progress" "0075CA" "Currently being worked on"
create_label "status:ready" "7057FF" "Ready to be worked on"

echo ""
echo "✅ Labels setup complete!"

# ═══════════════════════════════════════════════════════════════
# STEP 2: Create Initial Issues
# ═══════════════════════════════════════════════════════════════

echo ""
echo "════════════════════════════════════════════════════════════"
echo "Step 2: Creating initial issues..."
echo "════════════════════════════════════════════════════════════"

# Issue 1: Complete CI/CD health check integration
ISSUE1_BODY=$(cat <<'EOF'
## Objective
Complete the CI/CD health check integration by reviewing and merging the `ci/improve-deploy-verification` branch.

## Background
Branch `ci/improve-deploy-verification` is ready for PR with:
- Health check workflow (manual and callable)
- Rollback workflow with safety guards
- Reusable SSH action
- Enhanced main-apply.yml with post-deployment verification
- Comprehensive documentation

## Tasks
- [ ] Create pull request from `ci/improve-deploy-verification` to `master`
- [ ] Review workflow files for best practices
- [ ] Verify SSH key handling is secure
- [ ] Ensure health checks cover critical services
- [ ] Approve and merge PR
- [ ] Verify workflows are available after merge

## PR Details
- **Branch:** `ci/improve-deploy-verification`
- **Commits:** 2 commits (5fb89c0, 706b7e1)
- **URL:** https://github.com/bluefishforsale/homelab/compare/master...ci/improve-deploy-verification

## Documentation
- See `COMPLETION_REPORT.md` in the branch for full details
- See `PR_SUMMARY.md` for PR template

## Success Criteria
- PR created and merged to master
- All workflows available in GitHub Actions UI
- No breaking changes to existing workflows

## Priority
High - This is blocking integration testing of the new health check system.
EOF
)

create_issue \
  "Complete CI/CD health check integration" \
  "$ISSUE1_BODY" \
  '"agent:homelab-gitops","priority:high","status:ready"'

# Issue 2: Test full GitOps deployment cycle
ISSUE2_BODY=$(cat <<'EOF'
## Objective
Perform end-to-end validation of the GitOps deployment cycle with the new health check integration.

## Prerequisites
- Issue #1 must be completed (PR merged)
- New workflows available in GitHub Actions

## Test Scenarios

### 1. PR Validation
- [ ] Create test branch with small change (e.g., update README)
- [ ] Push branch and create PR
- [ ] Verify `ci-validate.yml` runs and passes
- [ ] Verify YAML syntax check passes
- [ ] Verify Ansible validation passes
- [ ] Verify security scan completes

### 2. Deployment Flow
- [ ] Merge test PR to master
- [ ] Verify `main-apply.yml` triggers automatically
- [ ] Verify Ansible playbooks apply successfully
- [ ] Verify health check job runs post-deployment
- [ ] Verify health check validates service availability
- [ ] Check deployment marked as successful

### 3. Manual Health Check
- [ ] Navigate to Actions → Health Check workflow
- [ ] Run with "all" target
- [ ] Verify comprehensive service checks execute
- [ ] Review health check results summary
- [ ] Confirm no false positives

### 4. Service Coverage
Verify health checks cover:
- [ ] Ocean services (nginx, Plex, Sonarr, Radarr, Prowlarr, Grafana, Prometheus)
- [ ] DNS services (internal and external resolution)
- [ ] DHCP server status
- [ ] GitHub runner containers

## Expected Results
- All workflows execute without errors
- Health checks accurately reflect service status
- Deployment process completes in < 5 minutes
- Clear success/failure indicators in Actions UI

## Documentation Updates
- [ ] Document any issues encountered
- [ ] Update troubleshooting guide if needed
- [ ] Record baseline deployment time
- [ ] Note any false positives for tuning

## Success Criteria
- Complete deployment cycle works end-to-end
- Health checks provide accurate service status
- No manual intervention needed for standard deployment
- Team confident in automated deployment process

## Priority
High - Validates the entire CI/CD enhancement is working correctly.
EOF
)

create_issue \
  "Test full GitOps deployment cycle" \
  "$ISSUE2_BODY" \
  '"agent:homelab-gitops","priority:high","status:blocked"'

# Issue 3: Document rollback procedures with real-world test
ISSUE3_BODY=$(cat <<'EOF'
## Objective
Test and document the rollback workflow with a real-world scenario in a safe environment.

## Prerequisites
- Issues #1 and #2 must be completed
- Rollback workflow available in GitHub Actions
- Safe test environment or willingness to rollback production

## Safety First
⚠️ **Important:** Always test with dry-run mode first!

## Test Scenarios

### 1. Dry-Run Rollback
- [ ] Navigate to Actions → Rollback workflow
- [ ] Select target commit (default HEAD~1)
- [ ] Enable dry-run mode
- [ ] Leave confirmation empty
- [ ] Run workflow
- [ ] Verify it shows what would be reverted
- [ ] Confirm no actual changes made

### 2. Commit Selection
- [ ] Test with specific commit SHA
- [ ] Test with auto-detect (HEAD~1)
- [ ] Verify commit validation works
- [ ] Confirm playbook auto-detection works

### 3. Safety Mechanisms
- [ ] Verify rollback fails without confirmation when dry-run is false
- [ ] Test that "ROLLBACK" confirmation is required
- [ ] Confirm dry-run is enabled by default
- [ ] Verify rollback won't run with invalid commit

### 4. Live Rollback (Optional - Use Caution!)
⚠️ **Only if you're comfortable rolling back production:**
- [ ] Make a safe, reversible change
- [ ] Deploy the change
- [ ] Trigger rollback with dry-run=false
- [ ] Type "ROLLBACK" to confirm
- [ ] Monitor rollback execution
- [ ] Verify services return to previous state
- [ ] Document time to complete rollback

## Documentation Tasks
- [ ] Create rollback runbook in `.github/ROLLBACK_RUNBOOK.md`
- [ ] Document when to use rollback
- [ ] Document rollback time estimates
- [ ] Include troubleshooting section
- [ ] Add examples of safe vs. risky rollbacks
- [ ] Document recovery if rollback fails

## Rollback Runbook Contents
Should include:
1. When to use rollback vs. forward fix
2. Pre-rollback checklist
3. Step-by-step rollback procedure
4. Post-rollback verification steps
5. Common issues and solutions
6. Rollback limitations
7. Emergency contact/escalation procedures

## Success Criteria
- Rollback tested in dry-run mode successfully
- Documentation covers all rollback scenarios
- Team knows when and how to use rollback
- Rollback time is < 2 minutes
- Clear decision matrix for rollback vs. forward fix

## Priority
Medium - Important for operational readiness, but not blocking current work.

## Notes
- Rollback should be last resort, not first response
- Always consider forward fix for simple issues
- Document rollback in incident reports
- Review rollback logs after any usage
EOF
)

create_issue \
  "Document rollback procedures with real-world test" \
  "$ISSUE3_BODY" \
  '"agent:homelab-gitops","priority:medium","status:blocked"'

echo ""
echo "════════════════════════════════════════════════════════════"
echo "✅ GitHub Issues Setup Complete!"
echo "════════════════════════════════════════════════════════════"
echo ""
echo "Next steps:"
echo "1. Visit: https://github.com/$REPO/issues"
echo "2. Review the created issues"
echo "3. Start working on Issue #1 (Complete CI/CD health check integration)"
echo ""
echo "Labels created: 8"
echo "Issues created: 3"
echo ""
