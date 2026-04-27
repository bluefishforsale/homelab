# Ocean VM CPU Type Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change ocean VM (VMID 5000) CPU type from `kvm64` to `host`, restart it, verify all containers recover, and update the creation docs so future rebuilds include the correct CPU type.

**Architecture:** An Ansible playbook (modeled after `dns01_vm.yaml`) gracefully shuts down ocean, applies `qm set 5000 --cpu host` on node006, restarts ocean, waits for SSH, then a second verification step confirms all expected Docker containers are back up. Finally the two doc files that contain the `qm` creation commands get the `--cpu host` flag added.

**Tech Stack:** Ansible (`hosts: node006`, `become: true`), Proxmox `qm` CLI, SSH

**Context:**
- `kvm64` CPU type omits x86-v2 features (SSE4.2, POPCNT). NumPy 2.x (shipped in the April 24 2026 `open-webui:main` pull) requires them. `--cpu host` passes through the real Xeon flags.
- node006 SSH: `root@192.168.1.106` ✓ (key auth works)
- ocean SSH: `terrac@192.168.1.143` ✓
- All other 37 containers are healthy; only `open-webui` is crash-looping.
- `homepage` shows `(unhealthy)` — pre-existing, not our problem.

---

### Task 1: Create the CPU-fix Ansible playbook

**Files:**
- Create: `playbooks/individual/infrastructure/ocean_cpu_host.yaml`

- [ ] **Step 1: Write the playbook**

```yaml
---
# Fix ocean VM CPU type from kvm64 → host so NumPy 2.x (and any future
# package requiring x86-v2 CPU features) works inside Docker containers.
#
# Usage:
#   ansible-playbook -i inventories/production/hosts.ini \
#     playbooks/individual/infrastructure/ocean_cpu_host.yaml
#
- name: Set ocean VM CPU type to host and restart
  hosts: node006
  become: true
  gather_facts: false

  vars:
    vmid: 5000
    ocean_ip: "192.168.1.143"

  tasks:
    - name: Show current CPU type
      ansible.builtin.command: "qm config {{ vmid }}"
      register: vm_config
      changed_when: false

    - name: Print current cpu line
      ansible.builtin.debug:
        msg: "{{ vm_config.stdout_lines | select('match', '^cpu') | list }}"

    - name: Gracefully shut down ocean VM
      ansible.builtin.command: "qm shutdown {{ vmid }}"
      register: shutdown_result
      changed_when: true

    - name: Wait for VM to reach stopped state
      ansible.builtin.command: "qm status {{ vmid }}"
      register: vm_status
      until: "'stopped' in vm_status.stdout"
      retries: 30
      delay: 10
      changed_when: false

    - name: Set CPU type to host
      ansible.builtin.command: "qm set {{ vmid }} --cpu host"
      changed_when: true

    - name: Confirm new CPU config
      ansible.builtin.command: "qm config {{ vmid }}"
      register: new_config
      changed_when: false

    - name: Print new cpu line
      ansible.builtin.debug:
        msg: "{{ new_config.stdout_lines | select('match', '^cpu') | list }}"

    - name: Start ocean VM
      ansible.builtin.command: "qm start {{ vmid }}"
      changed_when: true

    - name: Wait for SSH to become available on ocean
      ansible.builtin.wait_for:
        host: "{{ ocean_ip }}"
        port: 22
        delay: 15
        timeout: 180
        state: started
      delegate_to: localhost
      become: false
```

- [ ] **Step 2: Commit the playbook**

```bash
git add playbooks/individual/infrastructure/ocean_cpu_host.yaml
git commit -m "feat(infra): add playbook to set ocean VM CPU type to host

Fixes open-webui crash loop caused by NumPy 2.x requiring x86-v2
CPU features (SSE4.2/POPCNT) absent from the default kvm64 type."
```

---

### Task 2: Run the playbook and verify open-webui recovers

**Files:**
- No changes — run only

- [ ] **Step 1: Run the playbook**

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/ocean_cpu_host.yaml
```

Expected output: playbook completes with no failures; the "Print new cpu line" debug task should show `cpu: host`.

- [ ] **Step 2: Verify open-webui is no longer crash-looping (give it ~2 min to start)**

```bash
ssh ocean "for i in 1 2 3 4 5 6; do \
  echo \"=== attempt \$i ===\"; \
  docker ps --filter name=open-webui --format '{{.Names}}\t{{.Status}}'; \
  sleep 20; \
done"
```

Expected: after 1-2 minutes status changes from `Restarting` to `Up X seconds`.

- [ ] **Step 3: Verify CPU flags are now visible in ocean**

```bash
ssh ocean "grep -m1 'flags' /proc/cpuinfo | tr ' ' '\n' | grep -E 'sse4_2|popcnt|avx' | sort"
```

Expected: `avx`, `popcnt`, `sse4_2` (and many more) are printed.

- [ ] **Step 4: Check all containers are healthy**

```bash
ssh ocean "docker ps --format 'table {{.Names}}\t{{.Status}}' | sort"
```

Expected: `open-webui` shows `Up X minutes` (not Restarting). All previously healthy containers remain healthy. `homepage` may still show `(unhealthy)` — pre-existing, ignore.

- [ ] **Step 5: Smoke-test open-webui HTTP endpoint**

```bash
ssh ocean "curl -sf -o /dev/null -w '%{http_code}' http://localhost:3000/ && echo ' OK'"
```

Expected: `200 OK`

---

### Task 3: Update VM creation docs to include `--cpu host`

**Files:**
- Modify: `docs/operations/ocean-migration-plan.md` (lines ~184-186, CPU/memory section)
- Modify: `docs/operations/proxmox.md` (lines ~387, ocean creation block)

- [ ] **Step 1: Add `--cpu host` to ocean-migration-plan.md**

Find the block (around line 184):
```
# Set CPU and Memory (ocean specs: 30 cores, 256GB RAM)
qm set 5000 --cores 30
qm set 5000 --memory 262144  # 256GB in MB
```

Replace with:
```
# Set CPU and Memory (ocean specs: 30 cores, 256GB RAM)
qm set 5000 --cores 30
qm set 5000 --memory 262144  # 256GB in MB
# Set CPU type to host so x86-v2 features (SSE4.2, POPCNT) are visible
# inside the VM. Required for NumPy 2.x and other modern packages.
qm set 5000 --cpu host
```

- [ ] **Step 2: Add `--cpu host` to proxmox.md quick-reference block**

Find the ocean block (around line 387):
```
qm set 5000 --cores 30 --memory 262144
```

Replace with:
```
qm set 5000 --cores 30 --memory 262144
qm set 5000 --cpu host          # expose x86-v2 features for NumPy 2.x+
```

- [ ] **Step 3: Commit the doc updates**

```bash
git add docs/operations/ocean-migration-plan.md docs/operations/proxmox.md
git commit -m "docs(proxmox): add --cpu host to ocean VM creation commands

NumPy 2.x (adopted by open-webui April 2026) requires x86-v2 CPU
features absent from kvm64. Document the fix so future rebuilds don't
hit the same crash loop."
```
