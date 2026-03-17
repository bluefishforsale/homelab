# GitHub Actions Environment Configuration

## Required Environments

The following GitHub environments must be configured in repository settings for workflows to function correctly.

### Setup Location
Settings → Environments → New environment

---

## critical-services

**Purpose:** Manual approval gate for critical infrastructure deployments (DNS, Plex)

**Required For:** `.github/workflows/deploy-critical-service.yml`

**Configuration:**
- **Protection Rules:** 
  - ✅ Required reviewers (1+ approvers recommended)
  - ✅ Wait timer: 0 minutes (approval required immediately)
- **Deployment branches:** All branches
- **Environment secrets:** None required (uses repository secrets)

**Usage:**
When deploying critical services via workflow_dispatch, this environment gate ensures:
1. Dry-run completes successfully
2. Manual review of dry-run output
3. Explicit approval before production deployment

**Services Protected:**
- `dns` - ISC BIND9 on dns01
- `plex` - Plex Media Server on ocean

**Note:** DHCP service removed from critical services list (deprecated March 2026, replaced by Kea DHCP on dns02)

---

## Troubleshooting

### Error: "Value 'critical-services' is not valid"

**Cause:** Environment not configured in repository settings

**Fix:**
1. Go to Settings → Environments
2. Click "New environment"
3. Name: `critical-services`
4. Add required reviewers
5. Save environment

### Workflow Fails at Approval Step

**Cause:** No reviewers configured or reviewers not available

**Fix:**
- Add multiple reviewers to environment
- Ensure at least one reviewer has repository access
- Check reviewer notifications are enabled
