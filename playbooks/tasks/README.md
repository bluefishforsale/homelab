# Shared Ansible Tasks

This directory contains reusable Ansible task files that are included by multiple playbooks.

## Available Tasks

### `dpkg_lock.yaml`
**Purpose**: Handle dpkg lock issues and repair broken package installations  
**Used by**: Base system playbooks, core services  
**Usage**:
```yaml
- name: Handle dpkg locks and repair
  include_tasks: ../../tasks/dpkg_lock.yaml
```

### `gcloud.yaml`
**Purpose**: Google Cloud SDK authentication and setup  
**Used by**: Infrastructure playbooks requiring GCloud access  
**Usage**:
```yaml
- name: Include GCloud authentication
  ansible.builtin.include_tasks: ../../tasks/gcloud.yaml
```

### `kind.yaml`
**Purpose**: Kubernetes in Docker (kind) cluster setup  
**Used by**: Kubernetes deployment playbooks  
**Usage**:
```yaml
- name: Setup kind cluster
  ansible.builtin.include_tasks: ../../tasks/kind.yaml
```

### `pki_gcloud_kms.yaml`
**Purpose**: Manage PKI resources in Google Cloud KMS  
**Used by**: Infrastructure playbooks with PKI requirements  
**Usage**:
```yaml
- name: Manage KMS resources
  ansible.builtin.include_tasks: ../../tasks/pki_gcloud_kms.yaml
```

### `pki_tools_apt.yaml`
**Purpose**: Install PKI tools on Debian/Ubuntu systems  
**Used by**: PKI infrastructure playbooks  
**Usage**:
```yaml
- name: Install PKI tools
  ansible.builtin.include_tasks: ../../tasks/pki_tools_apt.yaml
```

### `pki_tools_macos.yaml`
**Purpose**: Install PKI tools on macOS systems  
**Used by**: PKI infrastructure playbooks (local development)  
**Usage**:
```yaml
- name: Install PKI tools
  ansible.builtin.include_tasks: ../../tasks/pki_tools_macos.yaml
```

### `user.yaml`
**Purpose**: User account management and configuration  
**Used by**: Base system playbooks  
**Usage**:
```yaml
- name: Configure users
  ansible.builtin.include_tasks: ../../tasks/user.yaml
```

## Best Practices

1. **Idempotency**: All tasks should be idempotent and safe to run multiple times
2. **Error Handling**: Use `ignore_errors` or `failed_when` appropriately
3. **Documentation**: Document required variables and expected behavior
4. **Testing**: Test task files independently when possible

## Path Reference

From individual playbooks, use relative paths:
```yaml
# From playbooks/individual/base/*.yaml
include_tasks: ../../tasks/dpkg_lock.yaml

# From playbooks/individual/core/services/*.yaml
include_tasks: ../../../tasks/dpkg_lock.yaml

# From playbooks/individual/infrastructure/*.yaml
include_tasks: ../../tasks/gcloud.yaml
```

Or use absolute paths from playbook directory:
```yaml
include_tasks: "{{ playbook_dir }}/../../tasks/dpkg_lock.yaml"
```
