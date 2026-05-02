# Terminalbench On-Demand Evaluation Playbook

**Date:** 2026-04-29  
**Status:** Approved

## Overview

An on-demand Ansible playbook that evaluates LLM models on the RTX 3090 using terminalbench. It cycles through each eligible model one at a time — swapping the running llama.cpp service, running the benchmark container, and collecting results — then restores the production llama.cpp configuration when done.

## Goals

- Evaluate all `benchmark_eligible` models sequentially without manual intervention
- Produce per-model result files on disk for comparison across runs
- Leave the system in its normal production state after completion
- Reuse existing llama.cpp infrastructure (same service, same port, same API key)

## Non-Goals

- Parallel evaluation (GPU can only load one model at a time)
- Persistent terminalbench service (runs on-demand only, container exits after each model)
- Modifying open-webui or any other service during evaluation

---

## Architecture

### Playbook Location

```
playbooks/individual/ocean/ai/terminalbench.yaml
```

Targets the `ocean` host. Runs as `become: true` to manage systemd and Docker.

### Flow

```
for each model where benchmark_eligible == true:
  1. Write benchmark docker-compose.yml  →  swap model + benchmark_ctx_size
  2. systemctl restart llamacpp
  3. Poll http://localhost:8080/health  (timeout: 3 min, retry every 10s)
  4. harbor run --agent terminus-2 --model openai/<model>  →  write results files
  5. Capture stdout + JSON results to benchmarks dir

after loop:
  6. import_playbook: llamacpp.yaml  →  restore production profile
```

### Harbor CLI (terminal-bench)

Terminal-bench is the [Harbor framework's](https://github.com/harbor-framework/terminal-bench) agent evaluation benchmark — not a simple HTTP client. It runs LLM agents against real terminal tasks in Docker sandbox containers, scoring whether the agent solves each task.

- **Installation**: `uv tool install harbor` on the ocean VM (one-time, idempotent)
- **Agent**: `terminus-2` — Harbor's reference agent for terminal tasks
- **Model routing**: LiteLLM `openai/<model-name>` prefix with `OPENAI_API_BASE` env var pointing at localhost:8080
- **Run command**:
  ```bash
  OPENAI_API_BASE=http://localhost:8080/v1 \
  OPENAI_API_KEY=llamacpp-homelab-key \
  harbor run --dataset terminal-bench@2.0 --agent terminus-2 \
             --model openai/<model-name> -n 1
  ```
- **Output**: JSON result files written to a temp working directory; Ansible copies them to the benchmarks dir with dated model-named filenames

---

## Configuration

### `vars/vars_terminalbench.yaml` (new)

```yaml
terminalbench_dataset: "terminal-bench@2.0"
terminalbench_agent: "terminus-2"
terminalbench_benchmark_ctx_size: 8192        # standardized context for all models
terminalbench_output_dir: /data01/services/llamacpp/benchmarks
terminalbench_api_base: http://localhost:8080/v1
terminalbench_api_key: llamacpp-homelab-key
terminalbench_health_url: http://localhost:8080/health
terminalbench_health_timeout: 180             # seconds
terminalbench_health_interval: 10             # seconds between polls
terminalbench_n_concurrent: 1
```

### `vars/vars_llamacpp_models.yaml` additions

Each eligible model entry gains:

```yaml
benchmark_eligible: true
```

All three current models qualify (Qwen3-14B ~9GB, Qwen3-8B ~6GB, DeepSeek-R1-7B ~5GB — all fit within the 24GB VRAM budget at the standardized 8192 context size).

---

## Results Layout

Output directory on ocean: `/data01/services/llamacpp/benchmarks/`

Files accumulate across runs:

```
/data01/services/llamacpp/benchmarks/
  2026-04-29_Qwen3-14B-Q4_K_M.json
  2026-04-29_Qwen3-8B-Q4_K_M.json
  2026-04-29_DeepSeek-R1-Distill-Qwen-7B-Q4_K_M.json
```

Naming pattern: `{{ ansible_date_time.date }}_{{ model.name | replace('.gguf', '') }}.json`

---

## New Files

| Path | Purpose |
|---|---|
| `playbooks/individual/ocean/ai/terminalbench_run.yaml` | Entry point: benchmark then restore |
| `playbooks/individual/ocean/ai/terminalbench.yaml` | Benchmark play: setup + per-model loop |
| `playbooks/individual/ocean/ai/terminalbench_model.yaml` | Per-model included tasks: swap, restart, health, run, collect |
| `vars/vars_terminalbench.yaml` | Benchmark-specific config (agent, ctx size, output dir) |
| `files/ocean-terminalbench/docker-compose.yml.j2` | Benchmark docker-compose template (parameterized model + ctx) |

## Modified Files

| Path | Change |
|---|---|
| `vars/vars_llamacpp_models.yaml` | Add `benchmark_eligible: true` to each qualifying model |

---

## Restoration

After the model loop completes, the playbook calls:

```yaml
- import_playbook: llamacpp.yaml
```

This runs the full production llamacpp playbook, restoring the production model and context configuration. The restoration is idempotent — safe to re-run.

---

## Invocation

```bash
ansible-playbook playbooks/individual/ocean/ai/terminalbench_run.yaml
```

No extra vars required. The eligible model list is fully driven by `benchmark_eligible: true` in `vars_llamacpp_models.yaml`.

---

## Out of Scope / Future

- Results dashboard in Grafana
- Scheduled/recurring benchmark runs
- Automatic model addition based on VRAM budget calculation
