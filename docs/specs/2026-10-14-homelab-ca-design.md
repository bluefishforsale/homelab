# homelab-ca design

Date: 2026-10-14
Status: approved for implementation
Phase: 1 of 3 (see `2026-10-14-systemctl-api-design.md` for phase 2)

## Purpose

An internal X.509 certificate authority for the fleet, so internal services can
speak TLS with verifiable per-host identity instead of plaintext or self-signed
certs that nothing validates.

The immediate consumer is phase 2 (`systemctl-api`), which needs TLS to carry
per-caller bearer tokens. The CA stands alone regardless: anything internal that
currently cannot do verified TLS becomes able to.

## Goals

1. A single internal root CA whose certificate is distributed to the system trust
   store of every host in the inventory.
2. A repeatable, idempotent way to issue a server certificate per host covering
   that host's internal names and address.
3. Extensible per-host SANs, so a host serving additional names (nginx on `ocean`
   serving `grafana.home`, `llama.home`) can carry them without a wildcard.
4. No scheduled maintenance. Nothing expires, nothing needs renewing.
5. A path to client certificates for mTLS in phase 3, without redesign.

## Anti-goals

Explicitly out of scope. These are not deferred, they are excluded:

1. **Does not replace the public cert.** `home.terrac.com` continues to be issued
   by Let's Encrypt via certbot and Cloudflare DNS-01 on `ocean`. That is the
   external-facing cert and is untouched.
2. **Does not touch DNS.** Zone names are read to determine SAN content. No zone,
   record, resolver config or container is modified. (`agents.md` hard limit.)
3. **Does not touch sshd, `authorized_keys`, the fleet key, or firewall rules.**
   (`agents.md` hard limit.)
4. **No CRL or OCSP.** Follows directly from the no-expiration decision below.
   There is no revocation mechanism in this design.
5. **No client certificates issued in this phase.** The CA is built capable of it
   (no EKU constraint on the root); phase 3 uses it.
6. **No sub-CAs.** The root is `pathlen:0`.
7. **Not imported into `01_base_system.yaml`**, despite being `hosts: all`.

## Decisions

### D1: Ansible-managed CA, no CA service

Root key generated once, stored ansible-vault encrypted, certificates signed by
`community.crypto` during a normal apply.

Rationale: eight hosts does not justify a running CA. No new always-on service
becomes a dependency for internal TLS, nothing new to monitor or bootstrap, and
the CA key stays encrypted at rest in git rather than live on a host.

Rejected: `step-ca` with ACME (a new always-on service that internal TLS then
depends on, plus a bootstrap chicken-and-egg); HashiCorp Vault PKI (introduces a
dependency the fleet does not have; "vault" here means ansible-vault); two-tier
offline root plus online intermediate (correct PKI practice, materially more
ceremony than this fleet justifies).

### D2: No expiration

`notAfter` is `99991231235959Z` on the root and on every leaf, using RFC 5280's
convention for "no well-defined expiration date".

Rationale: the operator's requirement. A certificate that expires silently and
breaks the tooling used to repair the fleet is a real outage mode, and this
removes it absolutely.

Note that X.509 has no "never" field; `notAfter` is mandatory, so this is a
far-future date rather than an omission. The usual objection to long lifetimes
(the 398-day cap enforced by Chrome and Apple) does not apply here, because that
cap covers certificates chaining to preinstalled public roots. Privately
installed roots are exempt.

**Accepted tradeoff, recorded deliberately:** expiry is normally the only
revocation mechanism that requires no infrastructure. Removing it means a leaked
host key is a valid fleet credential forever, and the only remedies are running
CRL/OCSP (rejected per D1, it needs a service) or regenerating the CA and
redistributing trust fleet-wide (a flag day). This was raised and accepted. Two
mitigations below are promoted to load-bearing as a result (D4, D5), and one
follow-on is mandatory (F2).

### D3: No committed playbook may create or overwrite the CA key

Generation is a documented one-time manual procedure using `scripts/vault.py`.

Rationale: regenerating the CA invalidates every certificate on the fleet
simultaneously, and with no revocation there is nothing to soften it. This is
categorically not an operation an accidental apply should be able to perform.
Excluding it from automation entirely is both cheaper and stronger than gating it
behind a confirmation variable.

This mirrors the existing ZFS invariant in this repo, where no committed playbook
may mutate the pool, for the same reason: the catastrophic operation is excluded
from automation rather than guarded within it.

### D4: Host private keys are generated on the host and never leave it

Only the CSR travels to the controller.

Rationale: there is then no central collection of host private keys to lose. This
mattered less when expiry bounded the damage from a leak. With D2 it is the
primary containment.

### D5: `nameConstraints` on the root

The root permits issuance only within the DNS subtrees `home` and
`cluster.local`, plus the IP range `192.168.1.0/24`.

Note the subtree form is `home`, with **no leading dot**. Verified against
OpenSSL 3.0.22: this form matches `ocean.home` and the apex. See D6 for the
consequence, which is not optional.

Rationale: if the CA key itself leaks, it cannot mint a valid certificate for a
public name. With no revocation path, this is the difference between an internal
problem and an arbitrary-identity problem. `.home` is not an ICANN-delegated TLD,
so the constraint is both safe and tight.

### D6: Per-host certificates with extensible SANs

One certificate per host. SANs: `<host>.home`, the host's inventory IP, plus any
extra SANs from a per-host list in `vars/vars_homelab_ca.yaml`.

Rationale: each host has a distinct identity it cannot extend on its own, and
adding a service name is a reviewed one-line change.

**Bare short names are excluded, and this is a hard constraint, not a preference.**
A DNS SAN of `ocean` is not within the `home` permitted subtree, so including it
makes the certificate fail validation against our own CA. Verified against
OpenSSL 3.0.22:

```
$ openssl verify -CAfile ca.crt fqdn.crt     # SANs: ocean.home, 192.168.1.143
fqdn.crt: OK
$ openssl verify -CAfile ca.crt short.crt    # SANs: ocean.home, ocean, 192.168.1.143
CN = ocean
error 47 at 0 depth lookup: permitted subtree violation
error short.crt: verification failed
```

Consequence for every consumer: **connect by FQDN or IP, never by short name.**
`https://ocean.home:9443` and `https://192.168.1.143:9443` both validate;
`https://ocean:9443` cannot, because no valid certificate can carry that name.
The phase 2 client script must therefore construct `<host>.home` from an
inventory short name rather than passing it through. This is cheap to honour and
expensive to discover later, which is why it is recorded here rather than left to
implementation.

Rejected: wildcard `*.home` on every host (every host could then impersonate
every name in the zone, including `node005` presenting as `grafana.home`, which
gives up per-host identity entirely); separate per-service certificates (cleaner
separation, more certificates to track, deferred until nginx actually needs it);
names without IP SANs (the inventory addresses hosts by IP, so this breaks real
callers).

### D7: Key algorithms

Root: RSA-4096. Leaves: ECDSA P-256.

Rationale: the artifact that never expires gets the conservative, maximally
compatible option. Leaves are fast and reissuable, so they get the modern one.
Low-stakes and reversible for leaves.

## Architecture

| Artifact | Location | Secret |
|---|---|---|
| CA certificate | `files/homelab-ca/ca.crt` | No. Plain in git so a change to fleet trust is a readable diff. |
| CA private key | vault tree `pki.homelab_ca.key` | Yes. ansible-vault encrypted. |
| Per-host key | `/etc/homelab-pki/<host>.key` on the host, `0600` | Yes. Never leaves the host. |
| Per-host cert | `/etc/homelab-pki/<host>.crt` on the host, `0644` | No. |
| Trust anchor | `/usr/local/share/ca-certificates/homelab-ca.crt` | No. |

Root certificate extensions: `basicConstraints = CA:TRUE, pathlen:0`,
`keyUsage = keyCertSign`, `nameConstraints` per D5, `notAfter = 99991231235959Z`.
No `extendedKeyUsage` constraint, so phase 3 client certs need no CA change.

Leaf extensions: `extendedKeyUsage = serverAuth`, SANs per D6,
`notAfter = 99991231235959Z`.

## Issuance flow

One playbook, `playbooks/individual/infrastructure/homelab_ca.yaml`, `hosts: all`,
idempotent and safe to re-run:

1. **Distribute trust.** Copy `ca.crt` to
   `/usr/local/share/ca-certificates/homelab-ca.crt`; run `update-ca-certificates`
   on change only. This step alone makes a host able to verify fleet certificates.
2. **Generate the host key** at `/etc/homelab-pki/<host>.key`, mode `0600`, only if
   absent. Never overwritten: services depending on it would break until certs
   reissued.
3. **Build the CSR on the host** with SANs per D6.
4. **Sign on the controller** using the vault-held CA key,
   `community.crypto.x509_certificate` with `provider: ownca`.
5. **Place the certificate** at `/etc/homelab-pki/<host>.crt`, mode `0644`.

### The reissue trigger, which needs care

Expiry is normally what triggers reissue. With D2 there is no expiry, so **the
only reissue trigger is a change to the desired SAN set.** The play must compare
desired SANs against the existing certificate's SANs and sign only on mismatch or
absence.

Both error directions are real and neither is loud:

- Too lenient: certificates churn on every apply, and every consuming service
  needs a reload it did not expect.
- Too strict: adding `grafana.home` to a host's extra SANs silently does nothing,
  and the failure appears later as a TLS name mismatch.

This comparison is the single most important piece of logic in the playbook and
must be covered by the verification steps below.

## Consumption

Services read `/etc/homelab-pki/<host>.{crt,key}`.

Host-native binaries get CA trust from the system store automatically, so the Go
binary in phase 2 needs no special handling.

**Containers do not.** Docker images carry their own trust store, so any container
that needs to *verify* a fleet certificate must have `ca.crt` mounted explicitly.
Recorded here because it will otherwise present as an unexplained TLS failure in
phase 2 or later.

## Failure modes and follow-ons

- **F1. CA regeneration is a fleet-wide flag day.** Mitigated by D3.
- **F2. No revocation, ever. Mandatory follow-on:** decommissioning must wipe
  `/etc/homelab-pki`, and `docs/architecture/decommissioning.md` needs that
  step added. Without it, a retired host's disk is a permanent fleet credential.
- **F3. Trust must precede consumption.** A host with the CA but no certificate is
  fine. A client without CA trust rejects a perfectly valid certificate. This is
  why distribution is step 1.
- **F4. Signing requires the vault password**, so issuance runs from CI or
  agentbox, never the laptop. Consistent with `CONTEXT.md`.
- **F5. A host key must never be overwritten** once services depend on it.

## Monitoring

Nothing here listens on a port, so there is nothing to scrape directly, but the
house rule has no exceptions and the real failure modes are checkable.

A node-exporter textfile collector per host, following the existing
`files/node-exporter/zpool-metrics.sh` pattern, emitting:

- `homelab_ca_trust_installed` (1 if the CA is in the host trust store)
- `homelab_ca_cert_verifies` (1 if `<host>.crt` verifies against the CA)
- `homelab_ca_key_mode` (the key file's octal mode, so a permission regression is
  visible)

With expiry no longer a failure mode, these three are what remain. Alert on a host
missing CA trust, or holding a certificate that does not verify.

## Deploy mapping

Per `agents.md` impact detection:

- `playbooks/individual/infrastructure/homelab_ca.yaml` plus `files/homelab-ca/`
  maps cleanly to one owner via the `files/<svc>` convention, so
  `ownership-coverage.test.sh` passes.
- `vars/vars_homelab_ca.yaml` maps by name to that playbook.
- Variables go in that file specifically, **not** `group_vars/all*` or
  `inventories/**`, because those are the only inputs that replay the site-wide
  orchestrators.
- **Not** imported into `01_base_system.yaml`, which keeps this inside the
  one-service-per-PR limit despite `hosts: all`.

## Verification

CI here is lint and syntax only, so real verification is manual from agentbox:

1. `make validate` and `ansible-playbook --syntax-check`.
2. `--check` run from agentbox against the fleet.
3. `openssl verify -CAfile ca.crt <host>.crt` for each host.
4. `openssl x509 -in ca.crt -text` confirming `nameConstraints`, `pathlen:0`, and
   the 9999 `notAfter` are actually present, not merely configured.
5. **Negative test:** a certificate for a name outside `nameConstraints` must fail
   verification with `error 47 ... permitted subtree violation`. This is the test
   that proves the constraint is doing work. Already verified during design, see
   D6; the implementation must keep it passing.
6. **Idempotency test:** two consecutive applies must produce no certificate
   change (guards the reissue-trigger logic in the lenient direction).
7. **SAN-change test:** adding an extra SAN to one host must reissue exactly that
   host's certificate (guards it in the strict direction).

## Premises verified during design

Checked on agentbox, 2026-10-14, against the actual installed toolchain
(OpenSSL 3.0.22, `community.crypto` 2.14.0, `cryptography` 38.0.4), so no task
needs to rediscover them:

1. `notAfter = 9999-12-31 23:59:59` constructs, serializes and round-trips.
2. `nameConstraints` with DNS subtrees plus an `IPv4Network` constructs, and
   OpenSSL enforces it (`error 47`).
3. `basicConstraints CA:TRUE, pathlen:0` constructs.
4. A leaf with `<host>.home` plus an in-range IP SAN verifies against the
   constrained root.
5. `community.crypto.x509_certificate`'s `ownca_not_after` accepts an absolute
   "ASN.1 TIME" value, so `99991231235959Z` is the form to pass.

**Known trap:** `openssl req -x509 -days <n>` cannot express a year-9999 expiry;
large day counts overflow and silently produce the wrong date. Certificate
creation must pass the absolute timestamp, never a relative `-days` or `+Nw`
value.

## Prerequisite before any implementation

The CA key and certificate must exist. Per D3 this is a one-time manual human
procedure, not a playbook, and it blocks the issuance half of the work. The
trust-distribution half can be built and tested against a throwaway CA first.
