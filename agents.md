# Homelab Infrastructure — Agent Reference

Ansible-driven homelab managing Docker services, GPU passthrough, and CI/CD across a multi-host Proxmox cluster.

---

## Quick Commands

### Validation & Testing
```bash
# Run all validation checks
make validate

# Individual checks
make validate-yaml          # YAML syntax validation
make validate-ansible       # Ansible syntax check
make validate-templates     # Jinja2 template validation
make security-scan          # Check for hardcoded secrets
make check-vault            # Verify vault encryption
make lint-ansible           # Ansible linting (optional)

# Test a single playbook
ansible-playbook --syntax-check playbooks/individual/ocean/media/plex.yaml
ansible-playbook --check playbooks/individual/ocean/media/plex.yaml  # Dry run
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/media/plex.yaml
```

### Deployment Commands
```bash
# Deploy full infrastructure
ansible-playbook -i inventories/production/hosts.ini playbooks/00_site.yaml

# Deploy ocean services only
ansible-playbook -i inventories/production/hosts.ini playbooks/03_ocean_services.yaml

# Deploy single service
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/media/plex.yaml

# Deploy with specific tags
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/ai/comfyui.yaml --tags models

# Deploy from specific branch (for git-based services)
ansible-playbook -i inventories/production/hosts.ini playbooks/individual/ocean/services/terrac_com_static.yaml -e "git_branch=develop"
```

---

## Code Style Guidelines

### Ansible Playbooks

**1. File Structure**
- Use YAML format with `.yaml` extension (not `.yml`)
- Start every playbook with `---`
- Include descriptive header comments explaining purpose, tags, and usage examples
- Use 2-space indentation consistently

**2. Naming Conventions**
```yaml
# Playbook names: Use descriptive, action-oriented names
- name: Deploy Plex Media Server
- name: Configure nginx reverse proxy
- name: Sync built files to web root

# Variables: Use snake_case
service: plex
home_directory: /data01/services/plex
docker_image_version: latest

# Hosts: Use inventory names exactly
hosts: ocean
hosts: dns01
```

**3. Module Usage**
```yaml
# REQUIRED: Use fully qualified collection names (FQCN)
ansible.builtin.file:
ansible.builtin.template:
ansible.builtin.systemd:
community.docker.docker_compose:

# BAD - Don't use short names
file:
template:
```

**4. Variable References**
```yaml
# Use Jinja2 templating for all variables
path: "{{ home }}/config"
port: "{{ service_ports.plex.port }}"

# Access vault secrets
api_key: "{{ cloudflare.api_token }}"
password: "{{ media_services.plex.api_key }}"

# Use lookup for environment variables with defaults
git_branch: "{{ lookup('env', 'GIT_BRANCH') | default('main', true) }}"
```

**5. Task Organization**
```yaml
tasks:
  # 1. Display information
  - name: Display deployment information
    ansible.builtin.debug:
      msg: "Deploying {{ service }}"
    tags: [always]

  # 2. Create directories
  - name: Ensure base directory exists
    ansible.builtin.file:
      path: "{{ home }}"
      state: directory
      mode: '0755'
    tags: [setup]

  # 3. Deploy templates
  - name: Deploy docker-compose template
    ansible.builtin.template:
      src: "{{ files }}/docker-compose.yml.j2"
      dest: "{{ home }}/docker-compose.yml"
      mode: '0644'
    notify: Restart service

  # 4. Enable services
  - name: Enable systemd service
    ansible.builtin.systemd:
      name: "{{ service }}.service"
      enabled: yes
      state: started

handlers:
  - name: Reload systemd daemon
    ansible.builtin.systemd:
      daemon_reload: yes

  - name: Restart service
    ansible.builtin.systemd:
      name: "{{ service }}.service"
      state: restarted
```

**6. Error Handling**
```yaml
# AVOID: Don't suppress errors without justification
- name: Do something
  shell: command
  ignore_errors: true  # ❌ BAD

# GOOD: Check state explicitly
- name: Check if service exists
  ansible.builtin.shell: systemctl status servicename
  register: service_check
  failed_when: false      # Expected to fail if not exists
  changed_when: false     # Never reports as changed
  tags: [check]

- name: Start service only if it exists
  ansible.builtin.systemd:
    name: servicename
    state: started
  when: service_check.rc == 0

# ACCEPTABLE: Ignore errors for build steps with pre-built fallback
- name: Build the application
  ansible.builtin.command:
    cmd: npm run build
  register: build_result
  ignore_errors: true  # OK - Uses pre-built dist/ if build fails
  tags: [build]
```

**7. Idempotency**
- Every playbook must be safe to run multiple times
- Use `state:` parameters correctly (present/absent/started/stopped)
- Use `changed_when:` to prevent false positive changes
- Check state before making destructive changes

---

## File Organization

### Standard Service Structure
```
playbooks/individual/ocean/<category>/<service>.yaml
files/<service>/
  ├── docker-compose.yml.j2
  ├── <service>.service.j2
  └── <service>.env.j2
```

### Standard Playbook Template
```yaml
---
# <Service Name> - Brief description
#
# Available tags:
#   --tags setup    Description
#   --tags deploy   Description
#
# Usage:
#   ansible-playbook -i inventories/production/hosts.ini playbooks/path/to/playbook.yaml
#
- name: Configure <Service>
  hosts: ocean
  become: true
  gather_facts: true

  vars_files:
    - ../../../../vault/secrets.yaml
    - ../../../../vars/vars_service_ports.yaml

  vars:
    service: <name>
    port: "{{ service_ports.<service>.port }}"
    home: "/data01/services/{{ service }}"
    user: media
    uid: 1001
    gid: 1001

  tasks:
    # Tasks here

  handlers:
    # Handlers here
```

---

## Important Conventions

**Paths & Storage**
- All services: `/data01/services/<service>/`
- Systemd units: `/etc/systemd/system/<service>.service`
- Templates: `files/<service>/<template>.j2`

**Users & Permissions**
- Media services: `media:1001`
- System user: `terrac:1002`
- Directory mode: `0755`
- File mode: `0644`
- Executable mode: `0755`

**Ports**
- Define in `vars/vars_service_ports.yaml`
- Access via `{{ service_ports.<service>.port }}`
- Never hardcode ports in playbooks

**Secrets**
- Store in `vault/secrets.yaml` (encrypted)
- Never commit unencrypted secrets
- Access via `{{ vault_var.sub_var }}`

**Docker**
- Use Docker Compose for all services
- Manage via systemd services (Type=oneshot)
- Use journald logging driver
- Always set resource limits
- Always include health checks
- Pin image versions (no `latest` tags in production)

---

## Testing Workflow

1. **Before committing:**
   ```bash
   make validate-yaml
   ansible-playbook --syntax-check playbooks/path/to/playbook.yaml
   ```

2. **After changes:**
   ```bash
   ansible-playbook --check playbooks/path/to/playbook.yaml  # Dry run
   ```

3. **Deploy:**
   ```bash
   ansible-playbook -i inventories/production/hosts.ini playbooks/path/to/playbook.yaml
   ```

4. **Verify idempotency:**
   ```bash
   # Run twice - second run should show 0 changed
   ansible-playbook -i inventories/production/hosts.ini playbooks/path/to/playbook.yaml
   ansible-playbook -i inventories/production/hosts.ini playbooks/path/to/playbook.yaml
   ```

---

## Common Patterns

### Clean Git Deployment Pattern
```yaml
- name: Always remove existing git repository for clean deployment
  ansible.builtin.file:
    path: "{{ git_clone_path }}"
    state: absent

- name: Recreate repo directory with correct ownership
  ansible.builtin.file:
    path: "{{ git_clone_path }}"
    state: directory
    owner: terrac
    group: terrac
    mode: '0755'

- name: Clone git repository
  ansible.builtin.git:
    repo: "{{ git_repo }}"
    dest: "{{ git_clone_path }}"
    version: "{{ git_branch }}"
    force: yes
  become_user: terrac
```

### Docker Compose with Systemd Pattern
```yaml
- name: Deploy docker-compose template
  ansible.builtin.template:
    src: "{{ files }}/docker-compose.yml.j2"
    dest: "{{ home }}/docker-compose.yml"
  notify: Restart service

- name: Deploy systemd service
  ansible.builtin.template:
    src: "{{ files }}/{{ service }}.service.j2"
    dest: "/etc/systemd/system/{{ service }}.service"
  notify:
    - Reload systemd daemon
    - Restart service

- name: Enable and start service
  ansible.builtin.systemd:
    name: "{{ service }}.service"
    enabled: yes
    state: started
```

---

## Troubleshooting

**Syntax errors:** Check YAML indentation (2 spaces, no tabs)  
**Undefined variables:** Verify vars_files are loaded and vault is decrypted  
**Permission denied:** Check user/group ownership and become_user  
**Service won't start:** Check systemd logs: `journalctl -u <service>.service`  
**Git ownership issues:** Use clean deployment pattern (remove + recreate)

---

## CI/CD Integration

All changes are validated via GitHub Actions:
- YAML syntax validation
- Ansible syntax check
- Template validation
- Security scanning
- Automated deployment on merge to main

Critical services (DNS, DHCP, Plex) require manual approval before deployment.
