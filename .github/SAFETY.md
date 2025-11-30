# Safety & Protection Guide

Safety measures for GitHub Actions deployments.

---

## Quick Reference

| Host | IP | Risk Level |
|------|----|------------|
| dns01 | 192.168.1.2 | Critical |
| ocean | 192.168.1.143 | Critical |
| node005 | 192.168.1.105 | High |
| node006 | 192.168.1.106 | High |

---

## Critical Services

These require the protected workflow with manual approval:

| Service | Host | Port |
|---------|------|------|
| DNS (BIND) | dns01 | 53 |
| Plex | ocean | 32400 |

Deploy via: `Deploy Critical Service (Protected)` workflow

---

## Workflows

| Workflow | Purpose | Approval |
|----------|---------|----------|
| `ci-validate.yml` | Validation on push/PR | None (read-only) |
| `deploy-ocean-service.yml` | Standard services | Manual trigger |
| `deploy-critical-service.yml` | DNS, Plex | Manual approval required |
| `deploy-changed-services.yml` | Auto-deploy on merge | None |
| `main-apply.yml` | Full deployment | Manual trigger |

---

## Deployment Flow

```text
Validate YAML
    │
    ▼
Ansible Lint
    │
    ▼
Dry-Run (--check)
    │
    ▼
[Manual Approval] ← Critical services only
    │
    ▼
Deploy
    │
    ▼
Health Check
```

If any step fails, deployment stops.

---

## Standard Service Deployment

1. Go to Actions → `Deploy Ocean Service`
2. Select service from dropdown
3. Click "Run workflow"
4. Monitor validation → dry-run → deploy

---

## Critical Service Deployment

1. Go to Actions → `Deploy Critical Service (Protected)`
2. Select service (DNS or Plex)
3. Click "Run workflow"
4. Review dry-run output carefully
5. Approve deployment
6. Monitor health checks

---

## Emergency Procedures

### Check Service Status

```bash
# DNS
dig @192.168.1.2 ocean.home

# Plex
curl -s http://192.168.1.143:32400/web/index.html | head -1

# Docker services
ssh terrac@192.168.1.143 "docker ps --format 'table {{.Names}}\t{{.Status}}'"
```

### View Logs

```bash
# GitHub Actions logs in web UI

# Service logs on ocean
ssh terrac@192.168.1.143 "docker logs plex --tail 50"

# DNS logs
ssh debian@192.168.1.2 "journalctl -u named --since '1 hour ago'"
```

### Rollback

```bash
# Restore from backup (if available)
ssh debian@192.168.1.2 "tar -xzf /tmp/bind9-backup-*.tar.gz -C /"
ssh debian@192.168.1.2 "systemctl restart named"
```

---

## Secrets

| Secret | Location |
|--------|----------|
| Vault password | GitHub Secrets: `ANSIBLE_VAULT_PASSWORD` |
| SSH key | Mounted in runner containers |
| Encrypted vault | `vault/secrets.yaml` |

---

## Best Practices

1. Review dry-run output before approving
2. Deploy critical services during maintenance windows
3. Test with `--check` first for new playbooks
4. One critical service at a time
5. Verify service health after deployment

---

## Risk Levels

### Low Risk

- Validation and linting
- Dry-runs
- Health checks

### Medium Risk

- nginx, media services (non-Plex)
- AI services
- Monitoring stack

### High Risk

- DNS deployment
- Plex deployment
- Any infrastructure changes

---

**When in doubt, don't deploy.**
