# Playbook Error Handling Audit

Tracking error handling patterns that bypass fail-fast behavior.

---

## Summary

| Category | Count |
|----------|-------|
| Total playbooks | ~65 |
| Files with `ignore_errors` | 16 |
| Files with `failed_when: false` | 11 |
| Critical issues | 2 |
| Acceptable usage | 11 |

---

## Critical Issues

### packages.yaml

Location: `playbooks/individual/base/packages.yaml`

Issue: Package installation with `ignore_errors: true`

Recommendation: Remove `ignore_errors`, rely on retry logic only

### comfyui.yaml

Location: `playbooks/individual/ocean/ai/comfyui.yaml`

Issue: Model downloads with `failed_when: false` AND `ignore_errors: true`

Recommendation: Add validation after downloads or remove error suppression

---

## Moderate Issues

### users.yaml

Location: `playbooks/individual/base/users.yaml`

Issue: User/shell setup with `ignore_errors: yes`

Recommendation: Only suppress for cosmetic features (themes, oh-my-zsh)

---

## Acceptable Usage

Error suppression is acceptable for:

- **Detection tasks**: GPU presence, ZFS availability
- **Health checks**: Service connectivity tests
- **Cleanup operations**: Removing files that may not exist
- **Status checks**: VM existence before creation

Examples:

- `node_exporter.yaml` - GPU detection
- `cloudflared.yaml` - Tunnel connectivity tests
- `docker_ce.yaml` - Log truncation

---

## Workflow Gates

| Gate | Purpose |
|------|---------|
| YAML validation | Syntax check |
| Ansible syntax | Playbook structure |
| ansible-lint | Style and best practices |
| Dry-run | Check mode before deploy |
| Fail-fast | `set -e` in deployment |

---

## Best Practices

1. Avoid `ignore_errors` on critical tasks
2. Use `failed_when` with explicit conditions
3. Add validation after error-suppressed tasks
4. Document why errors are suppressed
5. Test with dry-run before deployment

---

## Related Documentation

- [SAFETY.md](SAFETY.md) - Safety procedures
- [workflows/README.md](workflows/README.md) - Workflow reference
