# terminalbench

An on-demand benchmark that runs Harbor's `terminal-bench@2.0` dataset with the
`terminus-2` agent against each llama.cpp model flagged `benchmark_eligible`, on
ocean. It is disabled in code and has not run since 2026-05-29 (per the comment
in `terminalbench.yaml`); it is kept so a run can be revived by dispatch. It
never touches the production llamacpp service: the models are served by a
separate CPU-only container.

## Playbooks and files

| Path | What it owns |
|---|---|
| `playbooks/individual/ocean/ai/terminalbench_run.yaml` | Entry point. Imports `terminalbench.yaml`, then `llamacpp.yaml` (production re-apply). |
| `playbooks/individual/ocean/ai/terminalbench.yaml` | Kill switch, PAT assert, dirs, `uv` + `uv tool install harbor` into `/root/.local/bin`, the per-model loop, the results push to master, then `docker compose down` of the bench instance. |
| `playbooks/individual/ocean/ai/terminalbench_model.yaml` | Per model: render compose, `docker compose up -d --force-recreate`, poll health, `harbor run` in a tempdir, copy stdout and result JSON, write a `.prom` textfile, add a summary row. |
| `files/ocean-terminalbench/docker-compose.yml.j2` | `terminalbench-llamacpp` container: `ghcr.io/ggerganov/llama.cpp:server-cuda`, `--n-gpu-layers 0`, 8 threads, `cpus: '8.0'`, `mem_limit: 24g`, models mounted `:ro` from `/data01/services/llamacpp/models`, host port 8082. |
| `vars/vars_terminalbench.yaml` | `terminalbench_enabled: false`, dataset/agent, ctx 8192, home `/data01/services/terminalbench-llamacpp`, port 8082, health URL, `terminalbench_n_tasks: 0` (whole dataset), textfile dir, repo results paths. |
| `vars/vars_llamacpp_models.yaml` | `benchmark_eligible: true` on all five current models. |
| `files/grafana-compose/dashboards/Model_Benchmarks.json` | "Model Benchmarks" dashboard (uid `model-benchmarks`). |
| `docs/benchmarks/results.md`, `docs/benchmarks/runs/` | Where results get committed back. Today `results.md` is header only and `runs/` is empty. |

## Running it

Never on push. `emit()` in `.github/workflows/lib/detect-impacted-playbooks.sh`
drops `playbooks/individual/ocean/ai/terminalbench*.yaml`, and the input loop
skips `vars/vars_terminalbench.yaml` and `files/ocean-terminalbench/*`, so
editing any of them deploys nothing. The only path is `main-apply.yml`
`workflow_dispatch`, which takes the playbook list verbatim:

```
playbooks: playbooks/individual/ocean/ai/terminalbench_run.yaml
extra_vars: terminalbench_enabled=true terminalbench_n_tasks=1
```

`terminalbench_n_tasks>0` passes harbor `-l N` for a smoke run.

## Gotchas

- The run fails at the first task until a new PAT exists. The results push
  needs `terminalbench.pat` (contents:write on this repo), which was deleted
  from the vault in #476. The assert in `terminalbench.yaml` fails fast instead
  of hours into the run. Mint one or drop the push block.
- NOTE: dispatching `terminalbench_run.yaml` without `terminalbench_enabled=true`
  still re-applies `llamacpp.yaml`, because `meta: end_play` only ends the
  bench play. The import is a leftover from the spec's swap-the-production-model
  design and is now redundant.
- The bench container is `restart: unless-stopped` and is only torn down at the
  end of `terminalbench.yaml`. A run that fails mid-loop leaves it up, holding 8
  CPUs and up to 24 GiB until someone runs `docker compose down` in
  `/data01/services/terminalbench-llamacpp`.
- The health poll has `failed_when: false`, so a container that never goes
  healthy still gets a `harbor run` against it.
- Result files are named `<date>_<model>.json`, so a same-day rerun overwrites.
  The copy takes the first `*.json` harbor wrote (`files[0]` of a recursive
  find); if none is found the later `slurp` fails the run.
- Port 8082 is not in `vars/vars_service_ports.yaml`.
- `harbor` and `uv` are unpinned (`curl ... install.sh | sh`, `uv tool install harbor`).
- Comments in `terminalbench.yaml` and `detect-impacted-playbooks.sh` still say
  "GPU benchmark". The compose is CPU-only by design (agents.md rule 5: the GPU
  stays with production).

## Results and metrics

Per model, on ocean in `/data01/services/terminalbench-llamacpp/benchmarks/`:
`<date>_<model>.json` and `<date>_<model>_stdout.txt`. Metrics go to
`/data01/services/node-exporter/text_files/terminalbench_<model>.prom`, picked up
by the existing node_exporter scrape (no Prometheus job of its own). Gauges, all
labelled `model`, `dataset`, `agent`:

`terminalbench_pass_rate`, `terminalbench_trials_total`,
`terminalbench_trials_resolved`, `terminalbench_trials_errored`,
`terminalbench_run_duration_seconds`, `terminalbench_run_timestamp_seconds`.

`count(terminalbench_pass_rate)` returned nothing on 2026-10-30, so the
dashboard is empty. No alerts reference these metrics.

The repo push runs on the runner (`delegate_to: localhost`): it fetches the
JSONs into `docs/benchmarks/runs/`, appends rows to `docs/benchmarks/results.md`,
and commits straight to master as `terminalbench-bot`. That path is outside the
`main-apply` filter, so it deploys nothing.

## Divergences from the spec

The spec swapped the production model on port 8080 (GPU) and restored it after.
The code runs a separate CPU container on 8082 and never touches production. The
output dir moved from `/data01/services/llamacpp/benchmarks` to the bench
instance's own dir. The kill switch, PAT gate, metrics, dashboard and repo
commit-back are all additions.

## Not built

- Scheduled or recurring runs: nothing in `.github/workflows/` or any timer
  invokes it.
- Model eligibility from a VRAM budget: eligibility is the hand-set
  `benchmark_eligible` flag.
- Alerts on benchmark results or on a stale run.

## Backout

It is already off. To remove it, delete the three playbooks,
`vars/vars_terminalbench.yaml`, `files/ocean-terminalbench/`, the dashboard, the
`benchmark_eligible` keys, and the terminalbench cases in
`detect-impacted-playbooks.sh`. On ocean, `docker compose down` in
`/data01/services/terminalbench-llamacpp` and delete any
`terminalbench_*.prom` from the textfile dir. Production llamacpp is unaffected
either way.
