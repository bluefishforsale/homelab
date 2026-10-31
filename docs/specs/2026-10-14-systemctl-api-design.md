# systemctl-api design

Date: 2026-10-14
Status: approved for implementation
Phase: 2 of 3. Depends on phase 1, `2026-10-14-homelab-ca-design.md`.

## Purpose

A small HTTP API on every fleet host that performs a fixed set of systemd unit
operations against a reviewed allowlist, so routine unit management stops
requiring an interactive SSH session with the fleet key.

The capability already exists in this repo as `scripts/fleet-restart.sh` over
SSH. This re-plumbs that one verb set onto an audited API. It is not a new power.

## The security argument this design rests on

An SSH session is arbitrary code execution as root. That is the baseline being
measured against, and it is why a narrow API can be a genuine reduction in
standing privilege rather than an expansion.

The API is only an improvement if it can do strictly **less** than the SSH session
it replaces. Concretely: six verbs against an enumerated list, no free-text unit
name ever reaching a shell, exact-match allowlist comparison, no unit-file writes,
no influence over `ExecStart`, and authorization enforced outside the daemon's own
code. Built that way it also beats SSH on auditability, since SSH gives you an
auth log but not a record of what was restarted.

Built as root with a shared token and glob matching, it would be a worse SSH. The
design is the entire difference, which is why the decisions below are not
negotiable details.

## Goals

1. Perform `start`, `stop`, `restart`, `reload`, `daemon-reload` and `reset-failed`
   on permitted units, from any authorized caller on the LAN.
2. Read-only `status`, `is-active` and journal access for **any** unit, not just
   allowlisted ones, so diagnosis does not require shell.
3. Per-caller identity in every audit record, so the log names agentbox, the CI
   runner, or a phone, and one caller can be revoked without rotating all.
4. Authorization that survives a bug in the daemon's request parsing.
5. Prometheus metrics and structured audit logs into the existing Loki pipeline.
6. No capability to brick a host, i.e. no path by which a single call removes the
   operator's ability to repair the fleet.

## Anti-goals

1. **Not a general remote-execution API.** No arbitrary commands, no shell, no
   unit-file creation or modification, no `systemd-run`, no enable/disable/mask.
2. **Does not replace `scripts/fleet-restart.sh`.** The SSH path is retained
   deliberately as break-glass (D9).
3. **Does not touch DNS, sshd, `authorized_keys`, the fleet key, or firewall
   rules.** (`agents.md` hard limits. See D7 for why firewalling is in-process.)
4. **No self-management.** The API cannot start, stop or restart its own unit.
5. **Not internet-exposed.** No Cloudflare tunnel, no ingress, no public DNS
   record. LAN only.
6. **No client certificates in this phase.** Phase 3.
7. **Not a compose project**, in deliberate deviation from the house rule. See D3.

## Decisions

### D1: Per-host agent on every host

A listener on each inventory host, deployed by one playbook with `hosts: all`,
following the `node_exporter.yaml` pattern.

Rationale: operator requirement. It survives agentbox being down, and it is the
only shape that genuinely retires the fleet key for this task.

Rejected: a single broker on agentbox holding the fleet key and shelling out over
SSH. That would have been one privileged surface instead of eight and reused
working auth, at the cost of an agentbox dependency and of keeping SSH as the
transport. The eight-surface cost was raised explicitly and accepted.

### D2: Unprivileged daemon, polkit is the authorization boundary

The daemon runs as a dedicated unprivileged system user `systemctl-api` and calls
systemd over D-Bus. A polkit rule at
`/etc/polkit-1/rules.d/50-systemctl-api.rules`, rendered per host from the
resolved allowlist, declares which units that identity may manage and with which
verbs.

Rationale: this is the single most important decision in the design. With it, a
parsing bug in the daemon is an unprivileged process that can restart Plex, not a
root compromise. Two independent things must both be wrong before anyone gets
root, and the privilege decision lives in a declarative policy file reviewable on
its own rather than in the daemon's control flow.

This repo already carries a scar from exactly the class of bug this contains:
`scripts/fleet-restart.sh` documents `sonarr` substring-matching `sonarr-exporter`
and bouncing the wrong container while reporting success. The same bug under root
systemd is a different magnitude of incident.

The relevant polkit actions are `org.freedesktop.systemd1.manage-units` (which
exposes `unit` and `verb` lookups to the rule) and
`org.freedesktop.systemd1.reload-daemon`.

**Verified on the fleet, 2026-10-14:** all nine hosts run polkit 122 on Debian 12
with `/etc/polkit-1/rules.d` present, and both action IDs exist in
`/usr/share/polkit-1/actions/org.freedesktop.systemd1.policy`. JS rules are
therefore supported everywhere and no `.pkla` fallback is needed. This mattered
enough to check rather than assume: polkit below 0.106 ignores JS rules silently,
which would have left the authorization boundary absent while appearing
configured.

**Refinement to the mechanism:** the daemon reaches systemd by exec'ing
`/usr/bin/systemctl` with an argv array (plus `--no-ask-password`), not through a
D-Bus library. `systemctl` run by an unprivileged user already routes through
D-Bus and is subject to the same polkit decision, so the authorization model is
identical, while the binary keeps **zero third-party dependencies**. For a
privileged binary that is worth more than typed D-Bus returns. Note this is argv
exec, not a shell: there is no interpolation and no metacharacter surface, and the
unit name is validated before it reaches argv. Prometheus exposition is
hand-rolled text for the same reason.

Rejected: running as root with systemd sandboxing (the in-process allowlist would
be the only authorization boundary, so the entire security of the system becomes
the correctness of the daemon's string handling, on eight hosts, indefinitely);
unprivileged plus an enumerated NOPASSWD sudoers allowlist (simpler, but sudoers
wildcards are a classic footgun and it is a weaker boundary than polkit).

A static system user is required rather than `DynamicUser=yes`, because polkit
needs a stable identity to match against.

### D3: CI-built Go static binary, native systemd unit, not a container

Source in this repo at `src/systemctl-api/`. A tag-triggered workflow builds a
static binary (`CGO_ENABLED=0`) and publishes it as a GitHub Release asset with a
SHA256. The playbook downloads the pinned version and verifies the checksum,
refusing to install on mismatch.

Rationale for Go: strong typing on the request parsing that stands in front of
privileged operations, and one static binary with no interpreter or apt
dependencies across Debian and Proxmox hosts.

Rationale for *not* a compose project, deviating from the house rule: a container
would need the host's D-Bus or systemd private socket mounted in, which hands any
container escape the exact privilege polkit was introduced to contain. And
`docker-refresh@` auto-pulls a floating tag weekly, which is a property you
specifically do not want on the one service that can restart units as root. Image
changes to a privileged service should arrive only through a reviewed PR. This is
the stricter answer, not the lazier one.

Consequence: the service does not get `docker-refresh@`, so its update path is an
ansible apply with a bumped `systemctl_api_version`. That is intentional.

Rejected: GHCR OCI artifact (unusual use of a registry for a bare binary);
building on the runner at deploy time (weakest provenance, toolchain becomes a
deploy dependency); vendoring the binary in git (unreviewable, bloats history).

### D4: Allowlist is explicit in vars, enforced by a CI check

`vars/vars_systemctl_api.yaml` declares permitted units per host, with a
`stoppable` flag per entry.

Maintenance is handled by a CI check, not by discipline: a test enumerates the
compose-wrapping unit templates in `files/*/*.service.j2` (detectable by their
`Environment=COMPOSE_PROJECT_NAME=` plus `ExecStart=/usr/bin/docker compose ... up`
signature) and asserts each unit name appears either somewhere in the allowlist or
in a declared `systemctl_api_exempt:` block in the same vars file, each entry
carrying a reason string. Adding a service without an entry fails CI at PR time
with the line to add.

The check deliberately asserts presence by unit name only, not per-host mapping.
Resolving which host deploys which unit would mean reparsing playbook-to-host
relationships, which is a second source of truth and a new failure mode. Name-only
presence is enough to catch the thing that actually goes wrong, which is adding a
service and forgetting the allowlist entirely.

Rationale: this gives a reviewed diff for every widening of what the API can
touch, with no recurring chore, since CI surfaces the requirement instead of the
operator remembering it. It is the same pattern `ownership-coverage.test.sh`
already uses with its `is_known_unowned()` exception ledger.

Rejected: deriving the allowlist from what the host runs at apply time. This was
initially chosen and then reconsidered. Two problems. It widens silently, so
nobody reviewed what the API can touch; and more seriously, "everything enabled
on the host" includes `networking`, `dbus`, `polkit`, `docker` and
`systemd-resolved`. Restarting `networking` or `dbus` on a remote host is an
instant self-inflicted outage with no route back, and `dbus` is what the daemon
uses to do its own job. A hard denylist does not save you there, because the
dangerous set is the system units, which cannot be enumerated safely in advance.

The onboarding requirement belongs alongside the existing monitoring requirement
in `docs/operations/deploy-pattern.md`.

### D5: Verbs are tiered by risk

| Verb | Gate |
|---|---|
| `status`, `is-active`, journal read | Any unit. No allowlist needed. |
| `start`, `restart`, `reload`, `reset-failed` | Any allowlisted unit. |
| `stop` | Allowlisted unit **with** `stoppable: true` in vars. |
| `daemon-reload` | Separate endpoint, separate token permission. Not unit-scoped. |

Rationale: `start`, `restart` and `reload` all end with the service trying to run.
`stop` is the only verb with no self-recovery: it leaves a service down
indefinitely and silently. `daemon-reload` is a global manager call, so there is
no unit to allowlist it against and it needs its own permission.

### D6: Hard denylist, compiled into the binary

Not configurable, not overridable by vars, and it overrides the allowlist. Applies
to mutations only; reads remain permitted for everything.

- `sshd.service`, `ssh.service`
- `dns-stack.service`, `named.service`, and any `pdns*`, `kea*`, `pihole*` unit
- `systemctl-api.service` (self)
- `docker.service`, `containerd.service`
- `dbus.service`, `polkit.service`
- `networking.service`, `systemd-networkd.service`, `systemd-resolved.service`
- `data01.mount` and any `zfs*` unit

Rationale: `agents.md` treats DNS and SSH as absolute, not judgement calls, and
these are the units whose loss removes the route back in. `dns-stack.service` is
specifically why this list must override the allowlist: it is a legitimate compose
service on the resolver pair. `data01.mount` and `zfs*` are here to honour the
repo's data-safety invariant.

### D7: Per-caller bearer tokens over TLS, source-CIDR checked in-process

- Tokens are 32 bytes of randomness, one per caller, stored in the vault tree at
  `infrastructure.systemctl_api.tokens.<caller>`.
- The rendered host config holds **SHA-256 hashes, not plaintext**, compared in
  constant time. A slow KDF is unnecessary for high-entropy random tokens.
- TLS server certificate and key come from phase 1 at
  `/etc/homelab-pki/<host>.{crt,key}`.
- Listener binds the host's LAN address, never `0.0.0.0`.
- The daemon enforces a source-CIDR allowlist **in process**, rather than via host
  firewall rules. Default is `192.168.1.0/24`, narrowable per host in
  `vars/vars_systemctl_api.yaml`. An empty list is a configuration error and
  refuses to start, rather than defaulting to permit-all.

Rationale for in-process CIDR filtering: `agents.md` forbids touching firewall
rules, which sit adjacent to the port-22 hard limit. Doing the check in the daemon
keeps the whole design clear of that limit.

Note on what the token is and is not. With polkit as the real boundary (D2), the
token is an identity and rate-limiting mechanism, not the thing standing between a
stranger and root. That is deliberate, because a bearer token is the weakest part
of this design: it travels in HTTP headers through logs, CI environments and error
messages, which is a genuine downgrade in secret hygiene versus a private key file
nobody accidentally echoes. Per-caller tokens and hashed storage reduce but do not
eliminate that.

### D8: SSRF hardening

Mutations require `POST` **and** a custom `X-Systemctl-Api: 1` header. Tokens are
never accepted in a query string.

Rationale: this is the one genuinely new exposure class the API introduces that
SSH does not have. Anything on the fleet that can be induced to make an HTTP
request (a webhook, Home Assistant, an exporter, an LLM with tool access) becomes
a potential path to this API. You cannot SSRF your way into an SSH session.
Requiring a non-simple header plus POST means a trivially-triggered GET cannot
reach a mutating endpoint.

### D9: `scripts/fleet-restart.sh` is retained

A new `scripts/systemctl-api.sh` becomes the API client. The SSH-based tool stays
in place as break-glass.

Rationale: if the API, the CA, or a host certificate is broken, the old tool still
works. Two tools to maintain is the price of not putting a single point of failure
into admin tooling. Retaining it also means the phase-1 certificate design cannot
lock the operator out of their own fleet.

**The client must construct `<host>.home` from an inventory short name**, never
pass the short name through as a TLS peer name. Phase 1 D6 establishes that no
valid fleet certificate can carry a bare short name, because it violates the
root's `nameConstraints`. So `https://ocean.home:9443` works and
`https://ocean:9443` cannot ever work. The same applies to the Prometheus scrape
targets below.

### D10: No self-management

The API's own unit is in the hard denylist for all verbs, including restart.

Rationale: restarting itself kills the process mid-request, and a safe
implementation would need a detached privileged helper, which is a new privileged
path for marginal benefit. Updating the agent is an ansible apply.

## API contract

Port `9443`, registered as `service_ports.systemctl_api` in
`vars/vars_service_ports.yaml`. TLS only.

| Method | Path | Auth | Notes |
|---|---|---|---|
| `GET` | `/healthz` | none | For a blackbox probe. |
| `GET` | `/metrics` | none | Prometheus. Consistent with the fleet's other exporters. Must leak no token material. |
| `GET` | `/v1/units` | token | The resolved allowlist and permitted verbs per unit. Makes the host's real scope inspectable. |
| `GET` | `/v1/units/{unit}/status` | token | Any unit. |
| `GET` | `/v1/units/{unit}/journal?lines=N` | token | Any unit. `N` defaults to 50, capped at 1000. |
| `POST` | `/v1/units/{unit}/{verb}` | token | `verb` in `start\|stop\|restart\|reload\|reset-failed`. Gated per D5, D6. |
| `POST` | `/v1/daemon-reload` | token with the `daemon-reload` permission | Not unit-scoped. |

Mutation response includes before and after state, mirroring what
`fleet-restart.sh` already reports, because "it returned 200" is not evidence the
service came back:

```json
{
  "unit": "plex.service",
  "verb": "restart",
  "result": "ok",
  "before": {"active": "active", "sub": "running"},
  "after":  {"active": "active", "sub": "running"},
  "caller": "agentbox",
  "duration_ms": 3412
}
```

### Input validation

Unit names are validated against a strict regex and a length cap, then compared
against the allowlist by **exact string match**, never prefix or substring. No
unit name is ever interpolated into a shell string; there is no shell. Operations
go over D-Bus with typed arguments.

## Rate limiting

A 30 second minimum interval between mutations on the same unit, and a single
in-flight mutation per host (a second concurrent mutation is rejected, not
queued). Both are overridable in `vars/vars_systemctl_api.yaml`. Over limit
returns `429` with the seconds remaining.

Rationale: operator error is the most likely incident, well ahead of any attacker.
On this fleet `restart` is expensive, because units run `compose down -> pull ->
up`, so a restart recreates the container and pulls an image. A restart loop is a
self-inflicted outage plus registry hammering. Rate limiting addresses this more
effectively than any authentication does.

## Audit and metrics

Structured JSON to stdout, so journald collects it and the existing promtail
pipeline carries it to Loki. One record per request: timestamp, caller, source IP,
verb, unit, decision (allowed or denied, with reason), result, duration.

This is the capability that makes the API better than SSH rather than merely
equivalent: SSH gives an auth log, not a queryable history of what was restarted.

Metrics:

- `systemctl_api_requests_total{verb,unit,result}`
- `systemctl_api_denied_total{reason}`
- `systemctl_api_allowlist_size`
- `systemctl_api_build_info{version}`

Plus a scrape job in `files/ocean-prometheus/prometheus.yml.j2` with
`scheme: https` and the phase-1 CA as `ca_file`, and a blackbox probe against
`/healthz`.

## systemd hardening

Beyond running unprivileged: `NoNewPrivileges=yes`, `ProtectSystem=strict`,
`ProtectHome=yes`, `PrivateTmp=yes`, `CapabilityBoundingSet=` (empty),
`SystemCallFilter=@system-service`, `ReadOnlyPaths=/etc/homelab-pki`, and a
`MemoryMax`. Defence in depth behind D2, not a substitute for it.

## Deploy mapping

- `playbooks/individual/infrastructure/systemctl_api.yaml`, `hosts: all`, plus
  `files/systemctl-api/` maps to one owner via the `files/<svc>` convention.
- `vars/vars_systemctl_api.yaml` maps by name to that playbook.
- **Not** imported into `01_base_system.yaml`, keeping within the
  one-service-per-PR limit despite `hosts: all`.
- `src/systemctl-api/` sits outside the deploy path filter, so source changes do
  not trigger an apply. The release workflow is tag-triggered. Note that
  `.github/workflows/**` *is* in the deploy path filter, so the workflow-adding PR
  needs its impact checked rather than assumed.
- The Prometheus scrape job touches the prometheus service and therefore belongs
  in its own PR, per the one-service limit.

## Failure modes

- **Certificate or CA broken** leaves the API unusable. Mitigated by D9: the SSH
  path remains.
- **Token leak** gives a caller's permitted verbs on that caller's permitted
  units, bounded by polkit (D2) rather than by the token alone. Remedy is rotating
  one token, not all.
- **Lateral movement.** Any compromised container on the LAN gains a path to this
  API. Today such an attacker needs the fleet key. Bounded by D5, D6, D7 and the
  rate limits, and this is the cost accepted in exchange for retiring interactive
  SSH for this task.
- **Allowlist drift** between vars and reality. Caught by the D4 CI check.
- **polkit rule and vars disagree.** The rule is rendered from the same vars, so
  they cannot diverge by construction. Verification below asserts it.

## Verification

CI is lint and syntax only, so these are manual from agentbox:

1. Go unit tests on validation, allowlist matching, denylist precedence and verb
   tiering. These are the tests that matter most, since they guard privilege.
2. **Denylist precedence test:** put `dns-stack.service` in the allowlist and
   confirm a mutation is still refused.
3. **Stop gating test:** confirm `stop` is refused on an allowlisted unit lacking
   `stoppable: true`.
4. **polkit test:** with the daemon's allowlist bypassed, confirm polkit still
   refuses a non-permitted unit. This proves the second boundary is real rather
   than decorative.
5. **Exact-match test:** confirm `sonarr` does not match `sonarr-exporter`, the
   known historical bug.
6. **SSRF test:** confirm a plain `GET` and a request without the custom header are
   both refused.
7. **TLS test:** `curl --cacert` succeeds, and a connection ignoring the CA fails.
8. End-to-end restart of one real allowlisted unit on one host, with before and
   after state and the audit record in Loki.

## Implementation order and PR decomposition

Each PR is one service, one branch, independently reviewable, and respects the
`agents.md` limits. None may be self-merged if it affects production.

**Wave 1, fully parallel, no interdependencies:**

- **PR-A `homelab-ca`.** Phase 1 playbook, `files/homelab-ca/`, vars, trust
  distribution, issuance, textfile metrics. Blocked on the manual CA key creation
  per phase 1 D3.
- **PR-B `systemctl-api` source and release pipeline.** `src/systemctl-api/`, the
  tag-triggered build workflow, Go unit tests. No deploy effect from `src/`; the
  workflow addition needs its impact checked.
- **PR-C client tooling.** `scripts/systemctl-api.sh`. Outside the deploy path
  filter, so no apply. Can be written against the contract above before the server
  exists.
- **PR-D docs.** The decommissioning step (`docs/architecture/decommissioning.md`) (phase 1 F2) and the
  `deploy-pattern.md` onboarding requirement (D4).

**Wave 2, after PR-A merges and PR-B has a release:**

- **PR-E `systemctl-api` deployment.** Playbook, systemd unit, polkit rule
  template, `vars/vars_systemctl_api.yaml`, the D4 CI check, port registration.

**Wave 3, after PR-E:**

- **PR-F monitoring.** Prometheus scrape job and blackbox probe. Separate because
  it touches the prometheus service.

Dependency summary: PR-E needs PR-A for certificates and PR-B for a binary. PR-F
needs PR-E for something to scrape. Everything in wave 1 is independent.
