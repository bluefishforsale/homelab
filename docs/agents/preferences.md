# Choice preferences (how terrac wants homelab work done)

Distilled from prior decisions. Defaults, not laws, but don't override without a reason.

- **Live through CI/CD.** Push, watch the pipeline, iterate. Keep responses terse and
  evidence-backed; terrac is a strong critic of his own ideas, so push back rather than agree.
- **PRD before code.** For non-trivial work, write a zero-ambiguity PRD with explicit
  goals AND anti-goals first, then implement only against that scope. Grill the
  requirements before writing code.
- **New repos inherit the CI/CD deploy pattern** ([`docs/operations/deploy-pattern.md`](../operations/deploy-pattern.md)) automatically.
- **Every service is monitored — no exceptions.** Part of shipping any new service:
  (1) it has at least a **health check** (compose `healthcheck:` / systemd health / a
  blackbox probe) so failure is visible, and (2) if it exposes a **Prometheus metrics
  endpoint, add a scrape job** (`files/ocean-prometheus/prometheus.yml.j2`) so the metrics
  land in Prometheus and a Grafana panel, and (3) it runs as a **compose project**, which
  is what enables its `docker-refresh@<project>` timer and keeps a floating tag current
  (a bare `docker run` service never gets a new image again — see
  [`docs/operations/deploy-pattern.md`](../operations/deploy-pattern.md)).
  A service that ships unmonitored is unfinished.
  Verify with [`scripts/exporter-scrape-check.sh`](../../scripts/exporter-scrape-check.sh) (targets down)
  and [`scripts/prom.sh`](../../scripts/prom.sh) `targets`.
- **Fix through code, never manual SSH.** A playbook / workflow / runner-config change
  is the fix; manual SSH to a host is the escape hatch, not the default. Manual edits
  drift from IaC and get overwritten by the next apply.
- **No `Co-Authored-By: Claude` trailers** on commits. Considered weak; never add them.
- **Ansible house style:** FQCN modules (`ansible.builtin.*`), bare `true`/`false` (not
  `yes`/`no`), quoted octal file modes, and pass package lists to `apt`'s `name` rather
  than looping per item.
- **Vault is a nested tree** (`ai_services.openai.api_key`, `network.unifi.monitoring_user`).
  Reuse existing keys; never duplicate a secret across two locations.
- **Local-first for sensitive data.** PII / FERPA / HIPAA-adjacent work defaults to
  on-prem inference; cloud LLMs are explicit opt-in phase 2, never suggested unprompted.
