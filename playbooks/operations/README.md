# Operations Playbooks

One-shot, ad-hoc, administration, and operational playbooks for homelab management.

## Directory Structure

```text
operations/
├── backup/           # Backup operations (databases, configs, dashboards)
├── restore/          # Restore operations from backups
├── maintenance/      # Routine maintenance tasks (cleanup, updates, pruning)
├── troubleshooting/  # Diagnostic and troubleshooting playbooks
└── administration/   # Administrative tasks (user management, certificates, secrets rotation)
```

## Usage

These playbooks are designed to be run on-demand, not as part of the main deployment pipeline.

```bash
# Source environment first
source .envrc

# Run a specific operation
ansible-playbook -i inventories/production/hosts.ini playbooks/operations/<category>/<playbook>.yaml

# Dry run (check mode)
ansible-playbook -i inventories/production/hosts.ini playbooks/operations/<category>/<playbook>.yaml --check

# With extra variables
ansible-playbook -i inventories/production/hosts.ini playbooks/operations/<category>/<playbook>.yaml -e "var=value"
```

## Guidelines

- **Idempotent**: All playbooks must be safe to run multiple times
- **Non-destructive by default**: Use `--check` first, require confirmation for destructive operations
- **Tagged**: Use tags for granular control (e.g., `--tags verify`, `--tags execute`)
- **Documented**: Include header comments with purpose, usage examples, and required variables
- **Validated**: Test with `--syntax-check` and `--check` before running

## Examples

### Backup Operations

- Export Grafana dashboards to repo
- Backup PostgreSQL databases
- Export Prometheus rules and alerts
- Backup DNS zone files

### Restore Operations

- Restore database from backup
- Import Grafana dashboards
- Restore service configurations

### Maintenance

- Docker image pruning
- Log rotation and cleanup
- Certificate renewal
- Service health checks

### Troubleshooting

- Collect service logs
- Network connectivity tests
- DNS resolution verification
- Container status checks

### Administration

- Rotate API keys and secrets
- Update vault passwords
- User permission audits
- Certificate management
