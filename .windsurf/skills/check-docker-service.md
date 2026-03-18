---
description: SSH into a host and check docker service logs, then summarize and prioritize issues
---

# Check Docker Service

Retrieve container logs from a remote host via Ansible, then summarize and prioritize findings.

## Prerequisites

- SSH access configured for target hosts
- Vault password available via `ANSIBLE_VAULT_PASSWORD_FILE`
- Working directory: repository root

## SSH Users (from inventory)

Ansible picks the correct `ansible_user` automatically per host:

- **ocean**: `terrac`
- **node005, node006** (proxmox): `root`
- **All other VMs** (dns01, dns02, pihole, gitlab, gh-runner-01, openclaw): `debian`

No manual `-u` flag is needed — the inventory handles it.

## Steps

1. Ask the user for:
   - **Service name**: the Docker container/service name (required)
   - **Host**: target host (default: `ocean`)
   - **Tail lines**: number of log lines to retrieve (default: `200`)

2. Fetch the logs:

```bash
ansible <host> -m shell -a "docker logs --tail <tail_lines> <service> 2>&1" --become
```

3. After retrieving the logs, analyze the output and produce a concise report with:

   - **Summary**: One-line description of overall service health
   - **Priority issues**: Errors and warnings ranked by severity (critical → warning → info)
     - For each issue include: timestamp (if available), log line, and recommended action
   - **Patterns**: Repeated errors, crash loops, OOM kills, connection failures, or resource exhaustion
   - **Verdict**: Healthy / Degraded / Unhealthy with a one-sentence justification
