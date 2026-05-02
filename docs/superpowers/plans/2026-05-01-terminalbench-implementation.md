# Terminalbench On-Demand Evaluation Playbook — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an on-demand Ansible playbook that sequentially evaluates each benchmark-eligible llama.cpp model using Harbor terminal-bench, then restores the production llamacpp configuration.

**Architecture:** A master playbook (`terminalbench_run.yaml`) chains the benchmark play (which installs harbor via uv, loops over eligible models swapping docker-compose + restarting llamacpp + running `harbor run`, and collects JSON results) with a final `import_playbook` call to `llamacpp.yaml` for production restore.

**Tech Stack:** Ansible, harbor Python CLI (uv tool install), terminal-bench 2.0 dataset, terminus-2 agent, LiteLLM openai/ model prefix, llama.cpp Docker + systemd on ocean VM (RTX 3090).

---

## File Map

| Action | Path | Purpose |
|--------|------|---------|
| Modify | `vars/vars_llamacpp_models.yaml` | Add `benchmark_eligible: true` to each model |
| Create | `vars/vars_terminalbench.yaml` | Harbor config: agent, dataset, ctx size, output dir, API creds |
| Create | `files/ocean-terminalbench/docker-compose.yml.j2` | Benchmark docker-compose (parameterized model name + ctx size) |
| Create | `playbooks/individual/ocean/ai/terminalbench.yaml` | Main benchmark play: setup + model loop |
| Create | `playbooks/individual/ocean/ai/terminalbench_model.yaml` | Per-model included tasks: swap → restart → health → run → collect |
| Create | `playbooks/individual/ocean/ai/terminalbench_run.yaml` | Entry point: benchmark play then llamacpp.yaml restore |

---

### Task 1: Add benchmark_eligible flag to model vars

**Files:**
- Modify: `vars/vars_llamacpp_models.yaml`

- [ ] **Step 1: Add `benchmark_eligible: true` to each model entry**

In `vars/vars_llamacpp_models.yaml`, add `benchmark_eligible: true` to all three model entries. The complete file should look like:

```yaml
# llamacpp Model Configuration for RTX 3090 (24GB VRAM)
# Optimized for single-user reasoning with max context in 16GB VRAM budget

llamacpp_models:
  reasoning:
    - name: "Qwen3-14B-Q4_K_M.gguf"
      url: "https://huggingface.co/unsloth/Qwen3-14B-GGUF/resolve/main/Qwen3-14B-Q4_K_M.gguf"
      size: "8.5GB"
      parameters: "14B"
      vram_usage: "~9GB base + KV cache"
      capabilities: ["reasoning", "thinking", "step-by-step", "analysis", "math", "coding"]
      context_size: "40K"
      timeout: 3600
      priority: 1
      requires_auth: false
      benchmark_eligible: true

    - name: "Qwen3-8B-Q4_K_M.gguf"
      url: "https://huggingface.co/unsloth/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q4_K_M.gguf"
      size: "5.0GB"
      parameters: "8B"
      vram_usage: "~6GB base + KV cache"
      capabilities: ["reasoning", "thinking", "general", "analysis", "coding"]
      context_size: "64K"
      timeout: 2400
      priority: 2
      requires_auth: false
      benchmark_eligible: true

  backup:
    - name: "DeepSeek-R1-Distill-Qwen-7B-Q4_K_M.gguf"
      url: "https://huggingface.co/deepseek-ai/DeepSeek-R1-Distill-Qwen-7B-GGUF/resolve/main/DeepSeek-R1-Distill-Qwen-7B-Q4_K_M.gguf"
      size: "4GB"
      parameters: "7B"
      vram_usage: "~5GB"
      capabilities: ["reasoning", "basic", "fast"]
      context_size: "16K"
      timeout: 1800
      priority: 4
      requires_auth: false
      benchmark_eligible: true

# RTX 3090 Performance Profiles (16GB VRAM budget for headroom)
rtx_3090_profiles:
  single_user_reasoning:
    model: "Qwen3-14B-Q4_K_M.gguf"
    gpu_layers: -1
    context_size: 40960
    batch_size: 2048
    parallel_requests: 1
    flash_attention: true
    thinking_mode: true

  max_context:
    model: "Qwen3-8B-Q4_K_M.gguf"
    gpu_layers: -1
    context_size: 65536
    batch_size: 2048
    parallel_requests: 1
    flash_attention: true
    thinking_mode: true

  lightweight:
    model: "DeepSeek-R1-Distill-Qwen-7B-Q4_K_M.gguf"
    gpu_layers: -1
    context_size: 32768
    batch_size: 1024
    parallel_requests: 1
    flash_attention: true

# Model download priorities (1 = download first)
download_priority:
  1: "Qwen3-14B-Q4_K_M.gguf"
  2: "Qwen3-8B-Q4_K_M.gguf"
  3: "DeepSeek-R1-Distill-Qwen-7B-Q4_K_M.gguf"
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('vars/vars_llamacpp_models.yaml'))" && echo "OK"
```
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add vars/vars_llamacpp_models.yaml
git commit -m "feat(terminalbench): add benchmark_eligible flag to model vars"
```

---

### Task 2: Create vars_terminalbench.yaml

**Files:**
- Create: `vars/vars_terminalbench.yaml`

- [ ] **Step 1: Create the vars file**

```yaml
# Terminal-bench evaluation configuration
terminalbench_dataset: "terminal-bench@2.0"
terminalbench_agent: "terminus-2"
terminalbench_benchmark_ctx_size: 8192
terminalbench_output_dir: /data01/services/llamacpp/benchmarks
terminalbench_api_base: "http://localhost:8080/v1"
terminalbench_api_key: "llamacpp-homelab-key"
terminalbench_health_url: "http://localhost:8080/health"
terminalbench_health_timeout: 180
terminalbench_health_interval: 10
terminalbench_n_concurrent: 1
```

- [ ] **Step 2: Verify YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('vars/vars_terminalbench.yaml'))" && echo "OK"
```
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add vars/vars_terminalbench.yaml
git commit -m "feat(terminalbench): add terminalbench vars file"
```

---

### Task 3: Create benchmark docker-compose template

**Files:**
- Create: `files/ocean-terminalbench/docker-compose.yml.j2`

The production docker-compose hard-codes the model and context size in the `command:` array. The benchmark template parameterizes those two values while keeping everything else (GPU, port, volumes, security opts) identical to production.

- [ ] **Step 1: Create template directory and file**

```bash
mkdir -p files/ocean-terminalbench
```

Create `files/ocean-terminalbench/docker-compose.yml.j2`:

```jinja2
services:
  llamacpp:
    image: ghcr.io/ggerganov/llama.cpp:server-cuda
    container_name: llamacpp
    hostname: llamacpp
    restart: unless-stopped

    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu, compute, utility]

    environment:
      - TZ=America/Los_Angeles
      - NVIDIA_VISIBLE_DEVICES=all
      - NVIDIA_DRIVER_CAPABILITIES=all
      - CUDA_VISIBLE_DEVICES=0

    ports:
      - "8080:8080"

    volumes:
      - /data01/services/llamacpp/models:/models
      - /data01/services/llamacpp/config:/config
      - /data01/services/llamacpp/logs:/logs

    command:
      - "--host"
      - "0.0.0.0"
      - "--port"
      - "8080"
      - "--model"
      - "/models/{{ bench_model }}"
      - "--n-gpu-layers"
      - "-1"
      - "--ctx-size"
      - "{{ bench_ctx_size }}"
      - "--parallel"
      - "1"
      - "--batch-size"
      - "2048"
      - "--ubatch-size"
      - "512"
      - "--flash-attn"
      - "--api-key"
      - "llamacpp-homelab-key"
      - "--metrics"

    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:8080/v1/models || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s

    security_opt:
      - no-new-privileges:true

    network_mode: bridge
```

- [ ] **Step 2: Verify template renders correctly**

```bash
python3 -c "
from jinja2 import Template
t = Template(open('files/ocean-terminalbench/docker-compose.yml.j2').read())
out = t.render(bench_model='Qwen3-14B-Q4_K_M.gguf', bench_ctx_size=8192)
print(out)
import yaml; yaml.safe_load(out); print('YAML OK')
"
```
Expected: rendered YAML printed, then `YAML OK`. Confirm `/models/Qwen3-14B-Q4_K_M.gguf` and `8192` appear in the command list.

- [ ] **Step 3: Commit**

```bash
git add files/ocean-terminalbench/
git commit -m "feat(terminalbench): add benchmark docker-compose template"
```

---

### Task 4: Create terminalbench.yaml — skeleton with setup tasks

**Files:**
- Create: `playbooks/individual/ocean/ai/terminalbench.yaml`

- [ ] **Step 1: Create the playbook**

Create `playbooks/individual/ocean/ai/terminalbench.yaml`:

```yaml
---
- name: Run terminal-bench evaluation on benchmark-eligible llamacpp models
  hosts: ocean
  become: true
  gather_facts: true

  vars_files:
    - "{{ playbook_dir }}/../../../../vault/secrets.yaml"
    - "{{ playbook_dir }}/../../../../vars/vars_llamacpp_models.yaml"
    - "{{ playbook_dir }}/../../../../vars/vars_terminalbench.yaml"
    - "{{ playbook_dir }}/../../../../vars/vars_service_ports.yaml"

  vars:
    llamacpp_home: /data01/services/llamacpp
    benchmark_models: >-
      {{ llamacpp_models | dict2items
         | map(attribute='value') | flatten
         | selectattr('benchmark_eligible', 'equalto', true) | list }}

  tasks:

    - name: Ensure benchmark output directory exists
      ansible.builtin.file:
        path: "{{ terminalbench_output_dir }}"
        state: directory
        owner: media
        group: media
        mode: '0755'

    - name: Check if uv is installed
      ansible.builtin.command: which uv
      register: uv_check
      changed_when: false
      failed_when: false

    - name: Install uv
      ansible.builtin.shell: |
        curl -LsSf https://astral.sh/uv/install.sh | sh
      args:
        executable: /bin/bash
      environment:
        HOME: /root
      when: uv_check.rc != 0

    - name: Install harbor via uv
      ansible.builtin.command: /root/.local/bin/uv tool install harbor
      register: harbor_install
      changed_when: "'already installed' not in (harbor_install.stdout | default(''))"
      environment:
        HOME: /root
```

- [ ] **Step 2: Syntax check**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/ai/terminalbench.yaml
```
Expected: `playbook: playbooks/individual/ocean/ai/terminalbench.yaml` with no errors.

- [ ] **Step 3: Commit**

```bash
git add playbooks/individual/ocean/ai/terminalbench.yaml
git commit -m "feat(terminalbench): add playbook skeleton with uv/harbor setup"
```

---

### Task 5: Add per-model loop dispatch to terminalbench.yaml

**Files:**
- Modify: `playbooks/individual/ocean/ai/terminalbench.yaml`
- Create: `playbooks/individual/ocean/ai/terminalbench_model.yaml`

- [ ] **Step 1: Append loop dispatch to terminalbench.yaml**

After the `Install harbor via uv` task, add:

```yaml
    - name: Evaluate each benchmark-eligible model
      ansible.builtin.include_tasks: terminalbench_model.yaml
      loop: "{{ benchmark_models }}"
      loop_control:
        loop_var: bench_model_item
        label: "{{ bench_model_item.name }}"
```

- [ ] **Step 2: Create terminalbench_model.yaml with swap + restart + health tasks**

Create `playbooks/individual/ocean/ai/terminalbench_model.yaml`:

```yaml
---
# Per-model benchmark tasks — included from terminalbench.yaml
# loop_var: bench_model_item

- name: "Write benchmark docker-compose.yml for {{ bench_model_item.name }}"
  ansible.builtin.template:
    src: "{{ playbook_dir }}/../../../../files/ocean-terminalbench/docker-compose.yml.j2"
    dest: "{{ llamacpp_home }}/docker-compose.yml"
    owner: media
    group: media
    mode: '0644'
  vars:
    bench_model: "{{ bench_model_item.name }}"
    bench_ctx_size: "{{ terminalbench_benchmark_ctx_size }}"

- name: "Restart llamacpp for {{ bench_model_item.name }}"
  ansible.builtin.systemd:
    name: llamacpp.service
    state: restarted

- name: "Wait for llamacpp to be healthy ({{ bench_model_item.name }})"
  ansible.builtin.uri:
    url: "{{ terminalbench_health_url }}"
    method: GET
    status_code: 200
  register: health_check
  until: health_check.status == 200
  retries: "{{ (terminalbench_health_timeout / terminalbench_health_interval) | int }}"
  delay: "{{ terminalbench_health_interval }}"
```

- [ ] **Step 3: Syntax check**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/ai/terminalbench.yaml
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add playbooks/individual/ocean/ai/terminalbench.yaml \
        playbooks/individual/ocean/ai/terminalbench_model.yaml
git commit -m "feat(terminalbench): add per-model swap, restart, and health-check tasks"
```

---

### Task 6: Add harbor run and results collection to terminalbench_model.yaml

**Files:**
- Modify: `playbooks/individual/ocean/ai/terminalbench_model.yaml`

- [ ] **Step 1: Append harbor run and results tasks to terminalbench_model.yaml**

After the health-check task, add:

```yaml
- name: "Create temp working dir for {{ bench_model_item.name }}"
  ansible.builtin.tempfile:
    state: directory
    suffix: "_terminalbench"
  register: bench_tmpdir

- name: "Run harbor terminal-bench for {{ bench_model_item.name }}"
  ansible.builtin.command: >
    /root/.local/bin/harbor run
    --dataset {{ terminalbench_dataset }}
    --agent {{ terminalbench_agent }}
    --model openai/{{ bench_model_item.name }}
    -n {{ terminalbench_n_concurrent }}
  args:
    chdir: "{{ bench_tmpdir.path }}"
  environment:
    HOME: /root
    PATH: "/root/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    OPENAI_API_BASE: "{{ terminalbench_api_base }}"
    OPENAI_API_KEY: "{{ terminalbench_api_key }}"
  register: harbor_result

- name: "Save harbor stdout for {{ bench_model_item.name }}"
  ansible.builtin.copy:
    content: "{{ harbor_result.stdout }}"
    dest: >-
      {{ terminalbench_output_dir }}/{{ ansible_date_time.date }}_{{ bench_model_item.name | replace('.gguf', '') }}_stdout.txt
    owner: media
    group: media
    mode: '0644'

- name: "Find harbor result JSON files for {{ bench_model_item.name }}"
  ansible.builtin.find:
    paths: "{{ bench_tmpdir.path }}"
    patterns: "*.json"
    recurse: true
  register: harbor_result_files

- name: "Copy harbor result JSON for {{ bench_model_item.name }}"
  ansible.builtin.copy:
    src: "{{ harbor_result_files.files[0].path }}"
    dest: >-
      {{ terminalbench_output_dir }}/{{ ansible_date_time.date }}_{{ bench_model_item.name | replace('.gguf', '') }}.json
    owner: media
    group: media
    mode: '0644'
    remote_src: true
  when: harbor_result_files.files | length > 0

- name: "Clean up temp dir for {{ bench_model_item.name }}"
  ansible.builtin.file:
    path: "{{ bench_tmpdir.path }}"
    state: absent
```

- [ ] **Step 2: Syntax check**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/ai/terminalbench.yaml
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add playbooks/individual/ocean/ai/terminalbench_model.yaml
git commit -m "feat(terminalbench): add harbor run and results collection tasks"
```

---

### Task 7: Create master playbook with production restore

**Files:**
- Create: `playbooks/individual/ocean/ai/terminalbench_run.yaml`

`import_playbook` is a top-level directive and cannot be used inside a play. The master playbook chains the benchmark play with the production restore playbook at the top level.

- [ ] **Step 1: Create terminalbench_run.yaml**

Create `playbooks/individual/ocean/ai/terminalbench_run.yaml`:

```yaml
---
# On-demand terminal-bench evaluation entry point.
# Runs benchmarks for all benchmark_eligible models, then restores production llamacpp.
- import_playbook: terminalbench.yaml

- import_playbook: llamacpp.yaml
```

- [ ] **Step 2: Syntax check the master playbook**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/ai/terminalbench_run.yaml
```
Expected: no errors — both sub-playbooks are validated in the same pass.

- [ ] **Step 3: Commit**

```bash
git add playbooks/individual/ocean/ai/terminalbench_run.yaml
git commit -m "feat(terminalbench): add master playbook that chains benchmark + restore"
```

---

### Task 8: Lint and dry-run validation

**Files:**
- No new files — validation only.

- [ ] **Step 1: Run ansible-lint across all new playbooks**

```bash
ansible-lint \
  playbooks/individual/ocean/ai/terminalbench_run.yaml \
  playbooks/individual/ocean/ai/terminalbench.yaml \
  playbooks/individual/ocean/ai/terminalbench_model.yaml
```
Expected: exit code 0. Fix any `[warning]` or `[error]` lines before proceeding.

- [ ] **Step 2: Syntax check master playbook one final time**

```bash
ansible-playbook --syntax-check playbooks/individual/ocean/ai/terminalbench_run.yaml
```
Expected: no errors.

- [ ] **Step 3: Dry run against ocean**

```bash
ansible-playbook playbooks/individual/ocean/ai/terminalbench_run.yaml \
  --check --diff -l ocean
```
Expected: tasks show predicted changes. The `harbor run` command task will be skipped in check mode (Ansible skips `command`/`shell` tasks by default with `--check` unless `check_mode: false` is set — this is correct behavior).

- [ ] **Step 4: Verify model files exist on ocean before a real run**

```bash
ansible ocean -m ansible.builtin.find \
  -a "paths=/data01/services/llamacpp/models patterns=*.gguf" -b
```
Expected: JSON listing the three `.gguf` files. If a model is missing, run the llamacpp playbook first to download it.

- [ ] **Step 5: Commit any lint fixes**

If step 1 produced warnings that required edits:
```bash
git add -p
git commit -m "fix(terminalbench): address ansible-lint warnings"
```
Skip this step if step 1 was clean.
