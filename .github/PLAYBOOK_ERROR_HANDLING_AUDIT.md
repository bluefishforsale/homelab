# Playbook Error Handling Audit

## Overview

This document tracks error handling patterns in playbooks that bypass strict fail-fast behavior.

## ⚠️ Current State

The following playbooks contain error suppression (`ignore_errors`, `failed_when: false`) that should be reviewed:

### Critical Issues (Must Fix)

#### `individual/base/packages.yaml`
- **Lines 71, 76, 85**: Package installation with `ignore_errors: true`
- **Risk**: Failed package installations will be silent
- **Recommendation**: Remove `ignore_errors`, rely on retry logic only

#### `individual/ocean/ai/comfyui.yaml`
- **Lines 145-146**: Model downloads with both `failed_when: false` AND `ignore_errors: true`
- **Risk**: Failed model downloads will not block deployment
- **Recommendation**: Remove error suppression OR add explicit validation after downloads

### Moderate Issues (Should Review)

#### `individual/base/users.yaml`
- **Lines 8, 23, 90, 100, 107, 117, 127, 139, 150, 160, 170, 179**: User/shell setup with `ignore_errors: yes`
- **Risk**: User configuration failures will be silent
- **Recommendation**: Only suppress errors for cosmetic/optional features (themes, oh-my-zsh)

#### `individual/core/services/dhcp_ddns.yaml`
- **Lines 59, 67**: File removal and rsyslog config with `ignore_errors`
- **Risk**: Service may be misconfigured without notification
- **Recommendation**: Use `failed_when` with explicit conditions instead

### Acceptable Cases (Conditional Checks)

These are legitimate uses where failure is expected behavior:

#### Health Checks & Detection
- `node_exporter.yaml:107,260` - GPU detection (expected to fail on non-GPU hosts)
- `io_cpu_ups.yaml:16,253` - ZFS detection (expected to fail on non-ZFS systems)
- `qdisc.yaml:15` - Network qdisc check (checking for existing config)
- `cloudflared.yaml:31,108,158` - Tunnel connectivity tests
- `gcloud_sdk.yaml:33,53` - GCloud auth checks

#### Cleanup Operations
- `tasks/dpkg_lock.yaml` - Lock file cleanup (may not exist)
- `docker_ce.yaml:185` - Log truncation (files may not exist)
- `create_runner_vm.yaml:40` - VM status check (may not exist yet)

## ✅ Workflow Gates Added

The following gates now ensure failures are caught:

### 1. Validation Gate
- YAML syntax validation
- Ansible playbook syntax check
- Ansible-lint (style issues)

### 2. Dry-Run Gate
- Full playbook execution in check mode
- **Blocks deployment if dry-run fails**
- Only runs before live deployment (skipped if already in check mode)

### 3. Fail-Fast Deployment
- `set -e` - Exit on first error
- `set -o pipefail` - Catch errors in pipes
- Explicit exit code checking

## 📋 Recommended Actions

### Immediate (High Priority)

1. **Fix `comfyui.yaml` model downloads**
   ```yaml
   # REMOVE:
   failed_when: false
   ignore_errors: true
   
   # ADD after download loop:
   - name: Verify critical models downloaded
     ansible.builtin.stat:
       path: "{{ item }}"
     loop:
       - "{{ home }}/ComfyUI/models/checkpoints/flux1-dev-fp8.safetensors"
       - "{{ home }}/ComfyUI/models/vae/ae.safetensors"
     register: model_check
     failed_when: not model_check.stat.exists
   ```

2. **Fix `packages.yaml` installations**
   ```yaml
   # REMOVE ignore_errors from apt tasks
   # Keep only the retry logic
   ```

### Medium Priority

3. **Review user setup errors**
   - Keep `ignore_errors` only for themes/cosmetic features
   - Remove from critical steps (user creation, group membership)

4. **Add explicit failure conditions**
   - Replace `ignore_errors: yes` with `failed_when: false` only when truly optional
   - Add comments explaining WHY errors are ignored

### Low Priority (Nice to Have)

5. **Add validation after error-suppressed tasks**
   - Check that expected outcomes occurred
   - Log warnings for suppressed errors

6. **Document expected failures**
   - Add comments explaining when/why tasks may fail
   - Use `changed_when` with `failed_when` for detection tasks

## 🔒 Enforcement

### Workflow Level
- ✅ Syntax validation required
- ✅ Lint checking required  
- ✅ Dry-run required before deployment
- ✅ Fail-fast mode enabled (`set -e`)

### Playbook Level
**Still Required:** Manual review and fixes as listed above

### Testing Checklist
Before deploying a playbook with error suppression:

1. [ ] Does the dry-run pass?
2. [ ] Are suppressed errors documented with comments?
3. [ ] Are critical failures still caught?
4. [ ] Is there validation after optional steps?
5. [ ] Could this playbook fail silently in production?

## 📊 Statistics

- **Total playbooks scanned**: ~65
- **Files with `ignore_errors`**: 16
- **Files with `failed_when: false`**: 11
- **Critical issues**: 2 (packages.yaml, comfyui.yaml)
- **Moderate issues**: 3
- **Acceptable usage**: 11

## Next Steps

1. ✅ Workflow gates implemented
2. ⏳ Fix critical issues (packages.yaml, comfyui.yaml)
3. ⏳ Review moderate issues
4. ⏳ Add validation checks after error suppression
5. ⏳ Document all legitimate uses of error suppression

---

**Last Updated**: 2024-11-17  
**Status**: Workflows secured, playbook fixes pending
