# How a change maps to playbooks (impact detection)

`main-apply` does NOT run the whole site. `.github/workflows/lib/detect-impacted-playbooks.sh`
maps each changed file to the fewest playbooks — ideally one — and `apply-playbooks`
runs only those. The model is **explicit ownership**, not best-effort guessing:

- **`files/<svc>/**`** → the playbook that references `files/<svc>` literally, else the
  one declaring `service: <svc>`, else a playbook named `<svc>.yaml`. So a new service
  maps cleanly if it follows any one of those conventions.
- **`vars/vars_<name>.yaml`** → the playbook(s) that load it by name.
- **`vars/vars_service_ports.yaml`** (the shared port registry) → mapped by **which
  `service_ports.<key>` changed**, not the filename: a single port bump deploys only the
  service(s) referencing that key; a comment/format-only edit deploys nothing; if the
  key-diff can't be computed it falls back to every consumer. Needs `pyyaml` on the
  runner (installed in the workflow) and the base commit (`fetch-depth: 0`).
- **`roles/<role>/**`** → playbooks importing that role.
- **`inventories/**`, `group_vars/all*`** → the ONLY inputs that replay the site-wide
  orchestrators (`01_base_system`, `02_core_infrastructure`, `03_ocean_services`).
- **Unmapped owned input → hard error (exit 3).** A `files/`/`vars/`/`roles/` change that
  resolves to no playbook FAILS the run with a message, instead of silently fanning out
  across the fleet. Wire it to an owner (or, if it is genuinely deployed by nothing, add
  it to `is_known_unowned()` — that allowlist is the dead-input cleanup ledger).
- **Enforced in CI.** `ci-validate.yml`'s **Validate Deploy Mapping** job runs
  `detect-impacted-playbooks.test.sh` (pinned mappings) and `ownership-coverage.test.sh`
  (asserts every `files/`, `vars/`, `roles/` input resolves to an owner or a declared
  no-op). A new unwired service is caught at PR time, not as a fleet-wide fan-out on push.
