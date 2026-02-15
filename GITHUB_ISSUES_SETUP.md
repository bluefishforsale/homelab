# GitHub Issues Setup Guide

**Created:** 2026-02-14  
**Purpose:** Set up task management for homelab-gitops using GitHub Issues

---

## Quick Start - Two Options

### Option 1: Automated Setup (Recommended)

Run the provided script with a GitHub Personal Access Token:

```bash
cd /home/node/.openclaw/workspace/homelab-gitops

# 1. Create a GitHub PAT at: https://github.com/settings/tokens
#    - Click "Generate new token (classic)"
#    - Give it a name: "Homelab Issues Setup"
#    - Select scope: "repo" (full control of private repositories)
#    - Click "Generate token" and copy it

# 2. Export your token
export GITHUB_TOKEN='your_token_here'

# 3. Run the setup script
./setup-github-issues.sh
```

The script will:
- ✅ Create 8 labels (agent, priority levels, status indicators)
- ✅ Create 3 initial issues to track current work
- ✅ Output URLs to the created issues

### Option 2: Manual Setup via Web UI

If you prefer to set things up manually or don't want to create a token:

#### Step 1: Create Labels

Navigate to https://github.com/bluefishforsale/homelab/labels and create:

| Label Name | Color | Description |
|------------|-------|-------------|
| `agent:homelab-gitops` | `0E8A16` (green) | Tasks for Homelab GitOps agent |
| `priority:critical` | `D73A4A` (red) | Critical priority |
| `priority:high` | `FF6B6B` (light red) | High priority |
| `priority:medium` | `FFA94D` (orange) | Medium priority |
| `priority:low` | `94D82D` (light green) | Low priority |
| `status:blocked` | `FBCA04` (yellow) | Blocked - waiting on external dependency |
| `status:in-progress` | `0075CA` (blue) | Currently being worked on |
| `status:ready` | `7057FF` (purple) | Ready to be worked on |

#### Step 2: Create Issues

Navigate to https://github.com/bluefishforsale/homelab/issues/new and create 3 issues using the templates below.

---

## Issue Templates

### Issue #1: Complete CI/CD Health Check Integration

**Title:** `Complete CI/CD health check integration`

**Labels:** `agent:homelab-gitops`, `priority:high`, `status:ready`

**Body:**
```markdown
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
```

---

### Issue #2: Test Full GitOps Deployment Cycle

**Title:** `Test full GitOps deployment cycle`

**Labels:** `agent:homelab-gitops`, `priority:high`, `status:blocked`

**Body:**
```markdown
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
```

---

### Issue #3: Document Rollback Procedures with Real-World Test

**Title:** `Document rollback procedures with real-world test`

**Labels:** `agent:homelab-gitops`, `priority:medium`, `status:blocked`

**Body:**
```markdown
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
```

---

## Using GitHub Issues for Task Management

### Creating New Issues

When you identify new tasks:

```bash
# Example: Create a new issue
gh issue create \
  --title "Add monitoring for new service" \
  --body "Set up health checks for the new XYZ service" \
  --label "agent:homelab-gitops,priority:medium,status:ready"
```

Or use the web UI: https://github.com/bluefishforsale/homelab/issues/new

### Updating Issues

Comment on progress:
```bash
gh issue comment <issue-number> --body "Completed health check integration. Testing now."
```

Update labels:
```bash
# Mark as in progress
gh issue edit <issue-number> --add-label "status:in-progress" --remove-label "status:ready"

# Mark as blocked
gh issue edit <issue-number> --add-label "status:blocked"
```

### Closing Issues

When complete:
```bash
gh issue close <issue-number> --comment "Completed: All health checks passing. Documentation updated in commit abc123."
```

Or close via commit message:
```bash
git commit -m "feat: Add new service monitoring

Closes #<issue-number>"
```

---

## Issue Lifecycle

```
[Created]
    ↓
[status:ready] ─► [status:in-progress] ─► [Closed ✓]
                          ↓
                  [status:blocked]
                          ↓
                  [status:ready]
```

### Priority Guidelines

- **critical**: Service down, security vulnerability, blocking deployment
- **high**: Feature complete but needs review/merge, important bug
- **medium**: Improvements, documentation, non-blocking enhancements
- **low**: Nice-to-have, tech debt, future considerations

---

## Best Practices

1. **One Issue Per Task** - Keep issues focused and actionable
2. **Update Regularly** - Comment on progress, blockers, decisions
3. **Use Checklists** - Break down complex issues into subtasks
4. **Link Related Issues** - Reference other issues, PRs, commits
5. **Close with Summary** - Document what was done and any learnings
6. **Label Consistently** - Always include agent, priority, and status labels

---

## Integration with Git

### Reference Issues in Commits

```bash
git commit -m "fix: Correct health check timeout

Resolves timeout issues in ocean service checks.
Related to #<issue-number>"
```

### Close Issues from PRs

In PR description:
```markdown
Closes #1
Fixes #2
Resolves #3
```

When the PR merges, these issues will auto-close.

---

## Automation Ideas (Future)

- Auto-create issues from failed deployments
- Auto-update issue status based on PR status
- Weekly issue digest via notification
- Stale issue cleanup
- Automated priority adjustment based on labels

---

## Quick Reference

| Action | Command | Web UI |
|--------|---------|--------|
| List issues | `gh issue list` | https://github.com/bluefishforsale/homelab/issues |
| Create issue | `gh issue create` | https://github.com/bluefishforsale/homelab/issues/new |
| View issue | `gh issue view <number>` | https://github.com/bluefishforsale/homelab/issues/<number> |
| Comment | `gh issue comment <number>` | Click on issue → Add comment |
| Close | `gh issue close <number>` | Click "Close issue" button |
| Labels | `gh label list` | https://github.com/bluefishforsale/homelab/labels |

---

## Next Steps

1. ✅ Choose setup method (automated script or manual)
2. ✅ Create labels
3. ✅ Create the 3 initial issues
4. ✅ Start with Issue #1 (Create PR for ci/improve-deploy-verification)
5. ✅ Update issues as you make progress
6. ✅ Create new issues as you identify additional tasks

---

**Ready to go!** Start by setting up the labels and issues, then begin work on Issue #1. 🚀
