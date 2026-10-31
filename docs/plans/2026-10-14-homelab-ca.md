# homelab-ca Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up an internal never-expiring X.509 CA, distribute its trust anchor to all nine fleet hosts, and issue one per-host server certificate, so internal services can speak verified TLS.

**Architecture:** One ansible playbook with `hosts: all`. It distributes the CA certificate into each host's system trust store, generates a private key and CSR on the host (the key never leaves), slurps the CSR to the controller, signs it there with the vault-held CA key, and copies the certificate back. A standalone shell validator plus its test guards the SAN configuration in CI. Certificate creation of the root itself is deliberately NOT automated.

**Tech Stack:** Ansible (`community.crypto` 2.14.0), OpenSSL 3.0.22, Python `cryptography` 38.0.4, bash for the CI validator.

**Spec:** `docs/specs/2026-10-14-homelab-ca-design.md`

## Global Constraints

Every task's requirements implicitly include these. Values are copied verbatim from the spec.

- `notAfter` is **`99991231235959Z`**, passed as an absolute ASN.1 TIME string. **Never** use a relative value (`-days`, `+Nw`): `openssl req -x509 -days <n>` overflows past year 9999 and silently produces a wrong date.
- Root key: **RSA-4096**. Leaf keys: **ECDSA P-256** (`type: ECC`, `curve: secp256r1`).
- Root extensions: `basicConstraints = CA:TRUE, pathlen:0` (critical), `keyUsage = keyCertSign` (critical), `nameConstraints` (critical) permitting DNS `home`, DNS `cluster.local`, IP `192.168.1.0/24`. Permitted DNS subtrees carry **no leading dot**.
- Leaf SANs: `DNS:<host>.home` plus `IP:<inventory ip>` plus per-host extras. **Never a bare short name**: it violates `nameConstraints` and fails verification with `error 47 ... permitted subtree violation`.
- Leaf extensions: `extendedKeyUsage = serverAuth`.
- Paths: `/etc/homelab-pki/<host>.key` mode `"0600"`, `/etc/homelab-pki/<host>.crt` mode `"0644"`, directory mode `"0755"`, all `owner: root group: root`.
- Trust anchor: `/usr/local/share/ca-certificates/homelab-ca.crt`.
- CA certificate in git at `files/homelab-ca/ca.crt`. CA private key in vault at `pki.homelab_ca.key`.
- Vault loads as `vars_files: - "{{ playbook_dir }}/../../../vault/secrets.yaml"`.
- **No task may add a play, task or script that creates or overwrites the CA key or certificate.** Per spec D3 that is a manual human procedure. A playbook able to regenerate the CA can invalidate every certificate on the fleet in one accidental apply, with no revocation to soften it.
- Ansible house style: FQCN modules (`ansible.builtin.*`), bare `true`/`false` not `yes`/`no`, quoted octal modes, package lists passed to `apt`'s `name` rather than looped.
- Variables live in `vars/vars_homelab_ca.yaml`. **Never** `group_vars/all*` or `inventories/**`: those are the only inputs that replay the site-wide orchestrators.
- The playbook is **not** imported into `01_base_system.yaml`.
- The play must not restart any service. It runs against `dns01` and `dns02`, and the DNS pair is an absolute hard limit in `agents.md`. Adding a trust anchor is additive and safe; restarting anything there is not.

## Review Focus

Failure modes the spec implies that no task's happy path exercises. Each has its test assigned to the task that owns the code.

1. **An extra SAN outside the permitted subtrees** (`foo.example.com`, or a bare label like `grafana`). The certificate issues fine and then fails verification at use time, surfacing as an unrelated TLS error in whatever service consumes it. Test in Task 1.
2. **The same extra SAN claimed by two hosts.** Both get a valid certificate for the same name, so either can impersonate the other inside the fleet, silently and permanently given no revocation. Test in Task 1.
3. **A second consecutive apply reissues certificates.** With no expiry there is no other source of churn, so unnecessary reissue means every TLS consumer takes an unexpected reload on every apply. Test in Task 4.
4. **Adding an extra SAN does not reissue.** The new name silently has no certificate covering it; the failure appears much later as a name mismatch far from the change. Test in Task 4.
5. **Partial state: key present but certificate absent, or certificate present whose public key does not match the host key.** A consuming service then starts with a key and certificate that disagree. Test in Task 3 and Task 4.

---

## File structure

**Create:**
- `docs/operations/homelab-ca.md` — the runbook. Covers the manual one-time root creation (the only way the CA is ever created), how to add an extra SAN, and the CA-rotation flag-day procedure.
- `files/homelab-ca/ca.crt` — the CA certificate, committed in plain text so a change to fleet trust is a readable diff. Placeholder until the human runs the Task 0 procedure.
- `files/homelab-ca/ca-metrics.sh.j2` — node-exporter textfile collector reporting trust-anchor presence, certificate verification, and key file mode.
- `files/homelab-ca/ca-metrics.service.j2`, `files/homelab-ca/ca-metrics.timer.j2` — units driving that collector.
- `playbooks/individual/infrastructure/homelab_ca.yaml` — the single playbook, `hosts: all`.
- `vars/vars_homelab_ca.yaml` — per-host extra SANs and the collector's schedule.
- `.github/workflows/lib/validate-ca-sans.sh` — CI validator for the SAN configuration.
- `.github/workflows/lib/validate-ca-sans.test.sh` — its test, fixture-driven, following the existing `detect-impacted-playbooks.test.sh` pattern.

**Modify:**
- `docs/architecture/decommissioning.md` — add the mandatory step wiping `/etc/homelab-pki`. Spec F2. Without it a retired host's disk is a permanent fleet credential, because nothing expires and nothing revokes.
- `.github/workflows/ci-validate.yml` — run `validate-ca-sans.test.sh` and `validate-ca-sans.sh` in the existing **Validate Deploy Mapping** job.

**Out of scope for this plan:**
- Client certificates (phase 3).
- Any consumer of these certificates. Phase 2 is a separate plan.
- Replacing the Let's Encrypt cert for `home.terrac.com`. It stays.
- Any CRL or OCSP. Excluded by the no-expiration decision.

---

## Task 0: The runbook, and the human step it documents

This task writes documentation. The procedure inside it is executed by a human, not by this plan, and the issuance half of Task 4 cannot be verified against the real CA until they have run it.

**Files:**
- Create: `docs/operations/homelab-ca.md`

- [ ] **Step 1: Write the runbook**

Create `docs/operations/homelab-ca.md`. It must contain this root-creation procedure verbatim, because `openssl req -x509` cannot express a year-9999 expiry and a reader reaching for `-days` will get a silently wrong certificate.

````markdown
# homelab-ca runbook

The internal CA for the fleet. Certificates never expire; see
`docs/specs/2026-10-14-homelab-ca-design.md` for why, and for the
revocation consequences that follow.

## Creating the CA (one time, by hand, never by playbook)

No playbook creates or overwrites the CA. Regenerating it invalidates every
certificate on the fleet at once and there is no revocation to soften it, so it is
excluded from automation entirely rather than guarded within it.

Run on agentbox:

```bash
python3 - <<'PY'
import datetime, ipaddress
from cryptography import x509
from cryptography.x509.oid import NameOID
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa

key = rsa.generate_private_key(public_exponent=65537, key_size=4096)
name = x509.Name([
    x509.NameAttribute(NameOID.COMMON_NAME, u'homelab internal CA'),
    x509.NameAttribute(NameOID.ORGANIZATION_NAME, u'homelab'),
])
cert = (x509.CertificateBuilder()
    .subject_name(name).issuer_name(name)
    .public_key(key.public_key())
    .serial_number(x509.random_serial_number())
    .not_valid_before(datetime.datetime.utcnow() - datetime.timedelta(days=1))
    .not_valid_after(datetime.datetime(9999, 12, 31, 23, 59, 59))
    .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
    .add_extension(x509.KeyUsage(
        digital_signature=False, content_commitment=False,
        key_encipherment=False, data_encipherment=False,
        key_agreement=False, key_cert_sign=True, crl_sign=False,
        encipher_only=False, decipher_only=False), critical=True)
    .add_extension(x509.NameConstraints(
        permitted_subtrees=[
            x509.DNSName('home'),
            x509.DNSName('cluster.local'),
            x509.IPAddress(ipaddress.IPv4Network('192.168.1.0/24')),
        ],
        excluded_subtrees=None), critical=True)
    .sign(key, hashes.SHA256()))

open('ca.crt', 'wb').write(cert.public_bytes(serialization.Encoding.PEM))
open('ca.key', 'wb').write(key.private_bytes(
    serialization.Encoding.PEM,
    serialization.PrivateFormat.TraditionalOpenSSL,
    serialization.NoEncryption()))
print('wrote ca.crt and ca.key')
PY
```

Then:

1. Confirm what you built, before it goes anywhere:

   ```bash
   openssl x509 -in ca.crt -noout -enddate      # expect notAfter=Dec 31 23:59:59 9999 GMT
   openssl x509 -in ca.crt -noout -text | grep -A4 'X509v3 Name Constraints'
   openssl x509 -in ca.crt -noout -text | grep -A1 'X509v3 Basic Constraints'
   ```

2. Put `ca.crt` in the repo at `files/homelab-ca/ca.crt` and commit it. It is public.

3. Put the key into vault at `pki.homelab_ca.key` using `scripts/vault.py`. Never
   commit it unencrypted, never paste it into an issue or a PR, and never copy it
   to a fleet host.

4. Shred the local copies:

   ```bash
   shred -u ca.key
   ```

## Adding an extra SAN to a host

Edit `vars/vars_homelab_ca.yaml`, add the name under that host in
`homelab_ca_extra_sans`, open a PR. CI rejects a name outside the permitted
subtrees (`home`, `cluster.local`) and rejects a name claimed by two hosts.

The next apply reissues only that host's certificate. Services holding the old
certificate need a reload to pick up the new one.

## Rotating the CA (flag day, last resort)

There is no revocation. If the CA key leaks, the only remedy is a new CA, which
invalidates every fleet certificate simultaneously. Order matters: distribute the
new trust anchor to every host **first**, with the old one still present, then
reissue, then remove the old anchor. Doing it in any other order breaks every TLS
consumer on the fleet at once.

## Connect by FQDN or IP, never by short name

`https://ocean.home` and `https://192.168.1.143` validate. `https://ocean` cannot
ever validate, because a bare short name violates the CA's name constraints, so no
valid certificate can carry it.
````

- [ ] **Step 2: Commit**

```bash
git add docs/operations/homelab-ca.md
git commit -m "docs(homelab-ca): runbook for CA creation, SAN changes and rotation"
```

- [ ] **Step 3: Flag the human prerequisite**

State plainly in the PR description that `files/homelab-ca/ca.crt` is a placeholder
until a human runs the Step 1 procedure, and that Task 4's issuance cannot be
verified against the real CA until then. Do not fake it, and do not generate a real
CA to "unblock" yourself: that is exactly the operation the design excludes from
automation.

---

## Task 1: SAN configuration and its CI validator

The validator exists because a bad SAN entry produces a certificate that issues
successfully and fails much later, somewhere else. This is Review Focus 1 and 2.

**Files:**
- Create: `vars/vars_homelab_ca.yaml`
- Create: `.github/workflows/lib/validate-ca-sans.sh`
- Create: `.github/workflows/lib/validate-ca-sans.test.sh`
- Modify: `.github/workflows/ci-validate.yml`

**Interfaces:**
- Produces: `vars/vars_homelab_ca.yaml` defining `homelab_ca_extra_sans` (a mapping of inventory hostname to a list of bare DNS names) and `homelab_ca_metrics_schedule` (a systemd `OnCalendar` string). Task 3 and Task 5 consume both.
- Produces: `.github/workflows/lib/validate-ca-sans.sh`, taking a vars file path as `$1`, exit 0 on valid, exit 1 with a message naming the offending host and name.

- [ ] **Step 1: Write the failing test**

Create `.github/workflows/lib/validate-ca-sans.test.sh`:

```bash
#!/usr/bin/env bash
# Tests for validate-ca-sans.sh.
# Run from repo root: bash .github/workflows/lib/validate-ca-sans.test.sh
set -uo pipefail

SCRIPT="$(dirname "$0")/validate-ca-sans.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
PASS=0
FAIL=0

# $1 = name, $2 = expected exit code, $3 = vars file body
assert_exit() {
  local name="$1" expected="$2" body="$3" actual
  printf '%s\n' "$body" > "$TMP/vars.yaml"
  bash "$SCRIPT" "$TMP/vars.yaml" >"$TMP/out" 2>&1
  actual=$?
  if [ "$expected" = "$actual" ]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name (expected exit $expected, got $actual)"
    sed 's/^/    /' "$TMP/out"
    FAIL=$((FAIL + 1))
  fi
}

assert_exit "no extra SANs at all" 0 'homelab_ca_extra_sans: {}'

assert_exit "valid .home names" 0 'homelab_ca_extra_sans:
  ocean:
    - grafana.home
    - llama.home
  dns01: []'

assert_exit "valid cluster.local name" 0 'homelab_ca_extra_sans:
  ocean:
    - registry.cluster.local'

# Review Focus 1: a name outside the permitted subtrees issues fine and fails later.
assert_exit "rejects name outside permitted subtrees" 1 'homelab_ca_extra_sans:
  ocean:
    - grafana.example.com'

# Review Focus 1: a bare label violates nameConstraints (error 47).
assert_exit "rejects bare short name" 1 'homelab_ca_extra_sans:
  ocean:
    - grafana'

# Review Focus 2: two hosts claiming one name means mutual impersonation.
assert_exit "rejects duplicate SAN across hosts" 1 'homelab_ca_extra_sans:
  ocean:
    - grafana.home
  node005:
    - grafana.home'

assert_exit "rejects a DNS: prefix (bare names only)" 1 'homelab_ca_extra_sans:
  ocean:
    - DNS:grafana.home'

assert_exit "rejects an IP (extras are DNS names only)" 1 'homelab_ca_extra_sans:
  ocean:
    - 192.168.1.50'

echo
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -eq 0 ]
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bash .github/workflows/lib/validate-ca-sans.test.sh`
Expected: every case FAILs, because `validate-ca-sans.sh` does not exist yet.

- [ ] **Step 3: Write the validator**

Create `.github/workflows/lib/validate-ca-sans.sh`:

```bash
#!/usr/bin/env bash
# validate-ca-sans.sh — guard the homelab-ca extra-SAN configuration.
#
# Two failure modes this catches, both of which otherwise surface far from the
# change that caused them:
#   1. A name outside the CA's permitted subtrees. The certificate issues fine and
#      then fails verification at use time with "error 47 permitted subtree
#      violation", which reads like an unrelated TLS fault.
#   2. One name claimed by two hosts. Both get a valid certificate for it, so
#      either can impersonate the other. There is no revocation, so that is
#      permanent.
#
# Usage: validate-ca-sans.sh <path to vars_homelab_ca.yaml>
set -uo pipefail

VARS="${1:-vars/vars_homelab_ca.yaml}"
[ -f "$VARS" ] || { echo "validate-ca-sans: no such file: $VARS" >&2; exit 1; }

python3 - "$VARS" <<'PY'
import sys, re, yaml

path = sys.argv[1]
with open(path) as fh:
    doc = yaml.safe_load(fh) or {}

extras = doc.get('homelab_ca_extra_sans') or {}
if not isinstance(extras, dict):
    print("validate-ca-sans: homelab_ca_extra_sans must be a mapping of host to list",
          file=sys.stderr)
    sys.exit(1)

# Must match the nameConstraints permitted subtrees on the root. Keep in sync with
# docs/specs/2026-10-14-homelab-ca-design.md D5.
PERMITTED = ('.home', '.cluster.local')
LABEL = re.compile(r'^[a-z0-9]([a-z0-9-]*[a-z0-9])?$')

errors = []
seen = {}

for host, names in sorted(extras.items()):
    if names is None:
        continue
    if not isinstance(names, list):
        errors.append(f"{host}: extra SANs must be a list, got {type(names).__name__}")
        continue
    for name in names:
        if not isinstance(name, str):
            errors.append(f"{host}: extra SAN must be a string, got {name!r}")
            continue
        if ':' in name:
            errors.append(
                f"{host}: {name!r} must be a bare DNS name with no 'DNS:' or 'IP:' "
                f"prefix; the playbook adds the prefix")
            continue
        if re.match(r'^[0-9.]+$', name):
            errors.append(
                f"{host}: {name!r} looks like an IP address; extra SANs are DNS "
                f"names only, and the host's own IP is added automatically")
            continue
        if not name.endswith(PERMITTED):
            errors.append(
                f"{host}: {name!r} is outside the CA's permitted subtrees "
                f"{PERMITTED}. A certificate carrying it would fail verification "
                f"with 'error 47 permitted subtree violation'")
            continue
        if not all(LABEL.match(part) for part in name.split('.')):
            errors.append(f"{host}: {name!r} is not a valid DNS name")
            continue
        if name in seen:
            errors.append(
                f"{name!r} is claimed by both {seen[name]!r} and {host!r}. Two hosts "
                f"holding a valid certificate for one name can impersonate each "
                f"other, and there is no revocation")
            continue
        seen[name] = host

if errors:
    for e in errors:
        print(f"validate-ca-sans: {e}", file=sys.stderr)
    sys.exit(1)

print(f"validate-ca-sans: OK ({len(seen)} extra SANs across {len(extras)} hosts)")
PY
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `bash .github/workflows/lib/validate-ca-sans.test.sh`
Expected: `passed: 8  failed: 0`

- [ ] **Step 5: Create the vars file**

Create `vars/vars_homelab_ca.yaml`:

```yaml
---
# homelab-ca: per-host certificate configuration.
#
# Each host automatically gets DNS:<host>.home and IP:<its inventory ip>. This file
# is only for ADDITIONAL names a host serves, such as nginx vhosts on ocean.
#
# Names must sit inside the CA's permitted subtrees (.home or .cluster.local) and no
# name may be claimed by two hosts. Both rules are enforced by
# .github/workflows/lib/validate-ca-sans.sh in CI, because a violation issues
# successfully and only fails later, somewhere else.
#
# Bare short names are impossible by design: they violate the root's
# nameConstraints. Connect by FQDN or IP.

homelab_ca_extra_sans:
  ocean: []
  node005: []
  node006: []
  dns01: []
  dns02: []
  pihole: []
  gh-runner-01: []
  registry-cache-01: []
  agentbox: []

# Textfile collector cadence. Nothing expires, so this watches for a missing trust
# anchor or a certificate that stopped verifying, not for impending expiry.
homelab_ca_metrics_schedule: "hourly"
```

- [ ] **Step 6: Run the validator against the real vars file**

Run: `bash .github/workflows/lib/validate-ca-sans.sh vars/vars_homelab_ca.yaml`
Expected: `validate-ca-sans: OK (0 extra SANs across 9 hosts)`

- [ ] **Step 7: Wire both into CI**

In `.github/workflows/ci-validate.yml`, inside the existing **Validate Deploy Mapping** job, add two steps alongside the other `*.test.sh` invocations:

```yaml
      - name: Test CA SAN validator
        run: bash .github/workflows/lib/validate-ca-sans.test.sh

      - name: Validate CA SAN configuration
        run: bash .github/workflows/lib/validate-ca-sans.sh vars/vars_homelab_ca.yaml
```

Read the job's existing steps first and match their style. Do not restructure the job.

- [ ] **Step 8: Commit**

```bash
git add vars/vars_homelab_ca.yaml \
        .github/workflows/lib/validate-ca-sans.sh \
        .github/workflows/lib/validate-ca-sans.test.sh \
        .github/workflows/ci-validate.yml
git commit -m "feat(homelab-ca): SAN configuration with CI validation"
```

---

## Task 2: Trust distribution

The first half of the playbook, and the half that must land before anything consumes
a certificate. A host holding the CA but no certificate is fine; a client without
the CA rejects a perfectly valid certificate.

**Files:**
- Create: `playbooks/individual/infrastructure/homelab_ca.yaml`
- Create: `files/homelab-ca/ca.crt` (placeholder, see Task 0)

**Interfaces:**
- Produces: the playbook's single play, which Tasks 3, 4 and 5 append tasks to. Play-level `vars` defined here are consumed by all later tasks.

- [ ] **Step 1: Add the placeholder CA certificate**

Until a human runs Task 0, create `files/homelab-ca/ca.crt` containing exactly:

```
PLACEHOLDER: replace with the real CA certificate.
See docs/operations/homelab-ca.md. No playbook may generate this file.
```

A placeholder that cannot be mistaken for a certificate is correct here. Generating
a real CA to make the playbook run is the one thing this design forbids.

- [ ] **Step 2: Write the playbook with trust distribution only**

Create `playbooks/individual/infrastructure/homelab_ca.yaml`:

```yaml
---
# homelab-ca: distribute the internal CA trust anchor and issue per-host
# certificates.
#
# Spec: docs/specs/2026-10-14-homelab-ca-design.md
# Runbook: docs/operations/homelab-ca.md
#
# Certificates never expire (notAfter 9999-12-31), so there is no renewal and no
# expiry alerting. The only reissue trigger is a change to a host's SAN set.
#
# This play creates nothing in the CA itself. The CA key and certificate are made
# once by hand, per spec D3: a playbook able to regenerate them could invalidate
# every certificate on the fleet in a single accidental apply, and there is no
# revocation to soften that.
#
# Runs against dns01 and dns02. Adding a trust anchor is additive and restarts
# nothing. Do not add a task here that restarts a service.

- name: Distribute homelab CA trust and issue host certificates
  hosts: all
  become: true
  gather_facts: true

  vars_files:
    - "{{ playbook_dir }}/../../../vault/secrets.yaml"
    - "{{ playbook_dir }}/../../../vars/vars_homelab_ca.yaml"

  vars:
    ca_files: "{{ playbook_dir }}/../../../files/homelab-ca"
    ca_cert_src: "{{ ca_files }}/ca.crt"
    pki_dir: /etc/homelab-pki
    trust_anchor: /usr/local/share/ca-certificates/homelab-ca.crt
    host_key: "{{ pki_dir }}/{{ inventory_hostname }}.key"
    host_csr: "{{ pki_dir }}/{{ inventory_hostname }}.csr"
    host_crt: "{{ pki_dir }}/{{ inventory_hostname }}.crt"
    ca_not_after: "99991231235959Z"
    # DNS:<host>.home and IP:<inventory ip>, plus any reviewed extras. A bare short
    # name is deliberately absent: it violates the root's nameConstraints and makes
    # the certificate fail verification against our own CA (error 47).
    host_sans: >-
      {{ ['DNS:' + inventory_hostname + '.home',
          'IP:' + hostvars[inventory_hostname].ansible_ssh_host]
         + (homelab_ca_extra_sans[inventory_hostname] | default([])
            | map('regex_replace', '^', 'DNS:') | list) }}

  tasks:
    - name: Refuse to run against a placeholder CA certificate
      ansible.builtin.assert:
        that:
          - "'PLACEHOLDER' not in lookup('file', ca_cert_src)"
        fail_msg: >-
          files/homelab-ca/ca.crt is still the placeholder. Create the CA by hand
          first, per docs/operations/homelab-ca.md. Do not generate one from a
          playbook.
      run_once: true
      delegate_to: localhost
      become: false

    - name: Install the homelab CA trust anchor
      ansible.builtin.copy:
        src: "{{ ca_cert_src }}"
        dest: "{{ trust_anchor }}"
        owner: root
        group: root
        mode: "0644"
      register: ca_anchor

    - name: Refresh the system trust store
      ansible.builtin.command:
        cmd: update-ca-certificates
      register: trust_refresh
      changed_when: "'added' in trust_refresh.stdout or 'removed' in trust_refresh.stdout"
      when: ca_anchor.changed
```

- [ ] **Step 3: Syntax-check**

Run:
```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/homelab_ca.yaml --syntax-check
```
Expected: no errors.

- [ ] **Step 4: Confirm the placeholder guard actually fires**

Run:
```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/homelab_ca.yaml --check --limit agentbox
```
Expected: FAILS at "Refuse to run against a placeholder CA certificate", with the
runbook message. This is the correct outcome before the human has created the CA,
and it proves the guard works. Record this output in the PR.

- [ ] **Step 5: Commit**

```bash
git add playbooks/individual/infrastructure/homelab_ca.yaml files/homelab-ca/ca.crt
git commit -m "feat(homelab-ca): distribute CA trust anchor to all hosts"
```

---

## Task 3: Host key and CSR, generated on the host

The private key never leaves the host. With no expiry and no revocation, that is the
primary containment for a leak, not a nicety.

**Files:**
- Modify: `playbooks/individual/infrastructure/homelab_ca.yaml`

**Interfaces:**
- Consumes: play vars `pki_dir`, `host_key`, `host_csr`, `host_sans` from Task 2.
- Produces: a CSR at `{{ host_csr }}` on each host, which Task 4 slurps and signs. Also `host_key_stat`, registered, which Task 4 reads for the partial-state assertion.

- [ ] **Step 1: Append the key and CSR tasks**

Append to the `tasks:` list in `playbooks/individual/infrastructure/homelab_ca.yaml`:

```yaml
    - name: Ensure the PKI directory exists
      ansible.builtin.file:
        path: "{{ pki_dir }}"
        state: directory
        owner: root
        group: root
        mode: "0755"

    - name: Generate the host private key
      community.crypto.openssl_privatekey:
        path: "{{ host_key }}"
        type: ECC
        curve: secp256r1
        owner: root
        group: root
        mode: "0600"
        # Default regenerate: full_idempotence. The key is never recreated once it
        # exists: services hold it, and replacing it breaks TLS until the matching
        # certificate is reissued and every consumer reloads.
        regenerate: never

    - name: Generate the host CSR
      community.crypto.openssl_csr:
        path: "{{ host_csr }}"
        privatekey_path: "{{ host_key }}"
        common_name: "{{ inventory_hostname }}.home"
        subject_alt_name: "{{ host_sans }}"
        extended_key_usage:
          - serverAuth
        owner: root
        group: root
        mode: "0644"
      register: host_csr_result

    - name: Record the host key state
      ansible.builtin.stat:
        path: "{{ host_key }}"
      register: host_key_stat
```

- [ ] **Step 2: Assert the key file mode, which is Review Focus 5**

Append:

```yaml
    - name: Assert the host key is not readable beyond root
      ansible.builtin.assert:
        that:
          - host_key_stat.stat.exists
          - host_key_stat.stat.mode == '0600'
        fail_msg: >-
          {{ host_key }} has mode {{ host_key_stat.stat.mode | default('absent') }},
          expected 0600. A fleet private key readable by anything other than root is
          a leak, and nothing here expires or can be revoked.
```

- [ ] **Step 3: Syntax-check**

Run:
```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/homelab_ca.yaml --syntax-check
```
Expected: no errors.

- [ ] **Step 4: Verify the SAN expression renders correctly, with no bare short name**

This is the single most dangerous expression in the playbook, so check its output
directly rather than trusting it:

```bash
ansible -i inventories/production/hosts.ini all -m debug \
  -e @vars/vars_homelab_ca.yaml \
  -a "msg={{ ['DNS:' + inventory_hostname + '.home', 'IP:' + hostvars[inventory_hostname].ansible_ssh_host] + (homelab_ca_extra_sans[inventory_hostname] | default([]) | map('regex_replace', '^', 'DNS:') | list) }}" \
  --connection=local
```

Expected, for all nine hosts: exactly `DNS:<host>.home` and `IP:<its inventory ip>`.

Then confirm the negative explicitly:

```bash
ansible -i inventories/production/hosts.ini all -m debug \
  -e @vars/vars_homelab_ca.yaml \
  -a "msg={{ ['DNS:' + inventory_hostname + '.home'] }}" --connection=local | grep -c '"DNS:[a-z0-9-]*"$' || true
```

Expected: `0` matches, i.e. no SAN is a bare label. A bare label would make the
certificate fail against our own CA with `error 47`.

- [ ] **Step 5: Commit**

```bash
git add playbooks/individual/infrastructure/homelab_ca.yaml
git commit -m "feat(homelab-ca): generate host key and CSR on each host"
```

---

## Task 4: Sign on the controller and place the certificate

The CA key is used only on the controller. It is never transmitted to a fleet host,
which is why signing is not done in place.

**Files:**
- Modify: `playbooks/individual/infrastructure/homelab_ca.yaml`

**Interfaces:**
- Consumes: `host_csr`, `host_crt`, `ca_cert_src`, `ca_not_after`, `host_key_stat` from Tasks 2 and 3, and `pki.homelab_ca.key` from vault.
- Produces: `{{ host_crt }}` on each host. Task 5's collector verifies it.

- [ ] **Step 1: Append the signing tasks**

Append to the `tasks:` list:

```yaml
    - name: Read the host CSR
      ansible.builtin.slurp:
        src: "{{ host_csr }}"
      register: csr_slurp

    - name: Check for an existing host certificate
      ansible.builtin.stat:
        path: "{{ host_crt }}"
      register: host_crt_stat

    - name: Create the controller work directory
      ansible.builtin.file:
        path: "{{ ca_workdir }}"
        state: directory
        mode: "0700"
      delegate_to: localhost
      become: false
      run_once: true

    # Stage the existing certificate on the controller so x509_certificate compares
    # against it and only reissues on a real mismatch. Without this the module has
    # nothing to compare to and mints a fresh certificate on every apply, which with
    # no expiry would be the only source of churn on the fleet (Review Focus 3).
    - name: Stage the existing certificate for comparison
      ansible.builtin.copy:
        content: "{{ lookup('ansible.builtin.file', '/dev/null') }}"
        dest: "{{ ca_workdir }}/{{ inventory_hostname }}.crt"
        mode: "0600"
        force: false
      delegate_to: localhost
      become: false

    - name: Stage the existing certificate content
      ansible.builtin.slurp:
        src: "{{ host_crt }}"
      register: existing_crt
      when: host_crt_stat.stat.exists

    - name: Write the existing certificate to the controller
      ansible.builtin.copy:
        content: "{{ existing_crt.content | b64decode }}"
        dest: "{{ ca_workdir }}/{{ inventory_hostname }}.crt"
        mode: "0600"
      delegate_to: localhost
      become: false
      when: host_crt_stat.stat.exists

    - name: Sign the host certificate with the homelab CA
      community.crypto.x509_certificate:
        path: "{{ ca_workdir }}/{{ inventory_hostname }}.crt"
        csr_content: "{{ csr_slurp.content | b64decode }}"
        provider: ownca
        ownca_content: "{{ lookup('ansible.builtin.file', ca_cert_src) }}"
        ownca_privatekey_content: "{{ pki.homelab_ca.key }}"
        ownca_not_after: "{{ ca_not_after }}"
        ownca_not_before: "-1d"
        mode: "0600"
      delegate_to: localhost
      become: false
      register: signed_cert
      no_log: true

    - name: Install the host certificate
      ansible.builtin.copy:
        src: "{{ ca_workdir }}/{{ inventory_hostname }}.crt"
        dest: "{{ host_crt }}"
        owner: root
        group: root
        mode: "0644"
```

Add `ca_workdir` to the play's `vars` block in Task 2:

```yaml
    ca_workdir: "{{ lookup('env', 'HOME') }}/.cache/homelab-ca"
```

`no_log: true` on the signing task is required: its arguments contain the CA private
key, and ansible logs task arguments on failure.

- [ ] **Step 2: Assert the certificate and key actually match (Review Focus 5)**

Append:

```yaml
    - name: Assert the certificate matches the host key
      ansible.builtin.command:
        cmd: >-
          sh -c 'test "$(openssl x509 -in {{ host_crt }} -noout -pubkey)"
                    = "$(openssl pkey -in {{ host_key }} -pubout)"'
      changed_when: false
      register: key_match
      failed_when: key_match.rc != 0

    - name: Assert the certificate verifies against the homelab CA
      ansible.builtin.command:
        cmd: "openssl verify -CAfile {{ trust_anchor }} {{ host_crt }}"
      changed_when: false
      register: chain_verify
      failed_when: chain_verify.rc != 0
```

The second assertion is what catches a SAN that violates `nameConstraints`: it fails
here, during the apply, rather than later inside whichever service consumes the
certificate.

- [ ] **Step 3: Syntax-check**

Run:
```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/homelab_ca.yaml --syntax-check
```
Expected: no errors.

- [ ] **Step 4: Idempotency test (Review Focus 3)**

Blocked until a human has created the real CA. Once they have, run the playbook
twice against one host:

```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/homelab_ca.yaml --limit agentbox
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/homelab_ca.yaml --limit agentbox
```

Expected: the second run reports `changed=0`. If it reports a changed certificate,
the staging logic in Step 1 is not working and **must** be fixed before this goes
fleet-wide. Paste both summary lines into the PR.

- [ ] **Step 5: SAN-change test (Review Focus 4)**

Add a temporary extra SAN for one host:

```bash
# in vars/vars_homelab_ca.yaml, set:  agentbox: [test-reissue.home]
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/homelab_ca.yaml --limit agentbox
ssh agentbox "sudo openssl x509 -in /etc/homelab-pki/agentbox.crt -noout -text" \
  | grep -A2 'Subject Alternative Name'
```

Expected: the certificate is reissued and now carries `DNS:test-reissue.home`.
Then revert the vars change, re-run, and confirm it is reissued back without it.
Revert before committing: do not leave a test SAN in the vars file.

- [ ] **Step 6: Negative test, proving nameConstraints is enforced**

This one does not need the real CA and should be run regardless, because it is the
test that proves the constraint is doing work rather than merely being configured:

```bash
cd "$(mktemp -d)" && python3 - <<'PY'
import datetime, ipaddress
from cryptography import x509
from cryptography.x509.oid import NameOID
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa, ec
far = datetime.datetime(9999,12,31,23,59,59); nb = datetime.datetime(2026,1,1)
cak = rsa.generate_private_key(public_exponent=65537, key_size=2048)
can = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, u'test-ca')])
ca = (x509.CertificateBuilder().subject_name(can).issuer_name(can)
    .public_key(cak.public_key()).serial_number(1)
    .not_valid_before(nb).not_valid_after(far)
    .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
    .add_extension(x509.NameConstraints(
        permitted_subtrees=[x509.DNSName('home'), x509.DNSName('cluster.local'),
                            x509.IPAddress(ipaddress.IPv4Network('192.168.1.0/24'))],
        excluded_subtrees=None), critical=True).sign(cak, hashes.SHA256()))
open('ca.crt','wb').write(ca.public_bytes(serialization.Encoding.PEM))
def leaf(f, sans):
    lk = ec.generate_private_key(ec.SECP256R1())
    c = (x509.CertificateBuilder()
        .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, u'ocean')]))
        .issuer_name(can).public_key(lk.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(nb).not_valid_after(far)
        .add_extension(x509.SubjectAlternativeName(sans), critical=False)
        .sign(cak, hashes.SHA256()))
    open(f,'wb').write(c.public_bytes(serialization.Encoding.PEM))
leaf('good.crt', [x509.DNSName('ocean.home'),
                  x509.IPAddress(ipaddress.IPv4Address('192.168.1.143'))])
leaf('bad.crt',  [x509.DNSName('ocean.home'), x509.DNSName('ocean'),
                  x509.IPAddress(ipaddress.IPv4Address('192.168.1.143'))])
PY
openssl verify -CAfile ca.crt good.crt
openssl verify -CAfile ca.crt bad.crt
```

Expected: `good.crt: OK`, and `bad.crt` fails with
`error 47 at 0 depth lookup: permitted subtree violation`. If `bad.crt` passes, the
name constraints are not being applied and the containment described in spec D5 does
not exist.

- [ ] **Step 7: Commit**

```bash
git add playbooks/individual/infrastructure/homelab_ca.yaml
git commit -m "feat(homelab-ca): sign host certificates on the controller"
```

---

## Task 5: Textfile metrics

Nothing listens on a port, so there is nothing to scrape directly, but the house rule
has no exceptions and the real failure modes are checkable. With no expiry, the
things that can break are a missing trust anchor, a certificate that stopped
verifying, and a key whose permissions regressed.

**Files:**
- Create: `files/homelab-ca/ca-metrics.sh.j2`
- Create: `files/homelab-ca/ca-metrics.service.j2`
- Create: `files/homelab-ca/ca-metrics.timer.j2`
- Modify: `playbooks/individual/infrastructure/homelab_ca.yaml`

**Interfaces:**
- Consumes: `pki_dir`, `trust_anchor`, `host_crt`, `host_key` from Task 2, and `homelab_ca_metrics_schedule` from Task 1.
- Produces: `/data01/services/node-exporter/text_files/homelab_ca.prom`, scraped by the existing node-exporter textfile collector.

- [ ] **Step 1: Write the collector**

Create `files/homelab-ca/ca-metrics.sh.j2`:

```bash
#!/usr/bin/env bash
# homelab-ca textfile metrics.
#
# Certificates never expire, so there is no days-to-expiry metric here by design.
# What can actually break: the trust anchor goes missing, the host certificate stops
# verifying against the CA (a bad SAN, or a CA rotation half-applied), or the private
# key's mode regresses. Those are the three below.
#
# Rendered by playbooks/individual/infrastructure/homelab_ca.yaml. Do not edit on the
# host: the next apply overwrites it.
set -uo pipefail

OUT="{{ node_exporter_textfiles }}/homelab_ca.prom"
TMP="${OUT}.$$"
ANCHOR="{{ trust_anchor }}"
CRT="{{ host_crt }}"
KEY="{{ host_key }}"

trust=0
[ -f "$ANCHOR" ] && trust=1

verifies=0
if [ -f "$CRT" ] && [ -f "$ANCHOR" ]; then
  openssl verify -CAfile "$ANCHOR" "$CRT" >/dev/null 2>&1 && verifies=1
fi

mode=0
[ -f "$KEY" ] && mode=$(stat -c '%a' "$KEY" 2>/dev/null || echo 0)

{
  echo '# HELP homelab_ca_trust_installed Whether the homelab CA is in the host trust store.'
  echo '# TYPE homelab_ca_trust_installed gauge'
  echo "homelab_ca_trust_installed ${trust}"
  echo '# HELP homelab_ca_cert_verifies Whether this host certificate verifies against the homelab CA.'
  echo '# TYPE homelab_ca_cert_verifies gauge'
  echo "homelab_ca_cert_verifies ${verifies}"
  echo '# HELP homelab_ca_key_mode Octal mode of the host private key, as a decimal number.'
  echo '# TYPE homelab_ca_key_mode gauge'
  echo "homelab_ca_key_mode ${mode}"
} > "$TMP"

mv -f "$TMP" "$OUT"
```

- [ ] **Step 2: Write the units**

Create `files/homelab-ca/ca-metrics.service.j2`:

```ini
[Unit]
Description=Publish homelab-ca certificate state to the node_exporter textfile collector

[Service]
Type=oneshot
ExecStart={{ pki_dir }}/ca-metrics.sh
TimeoutStartSec=60
```

Create `files/homelab-ca/ca-metrics.timer.j2`:

```ini
[Unit]
Description=Periodic homelab-ca certificate state collection

[Timer]
OnCalendar={{ homelab_ca_metrics_schedule }}
Persistent=true
RandomizedDelaySec=300

[Install]
WantedBy=timers.target
```

- [ ] **Step 3: Append the deployment tasks**

Append to the `tasks:` list, and add `node_exporter_textfiles: /data01/services/node-exporter/text_files` to the play's `vars`:

```yaml
    - name: Install the CA metrics collector
      ansible.builtin.template:
        src: "{{ ca_files }}/ca-metrics.sh.j2"
        dest: "{{ pki_dir }}/ca-metrics.sh"
        owner: root
        group: root
        mode: "0755"

    - name: Install the CA metrics units
      ansible.builtin.template:
        src: "{{ ca_files }}/{{ item }}.j2"
        dest: "/etc/systemd/system/{{ item }}"
        owner: root
        group: root
        mode: "0644"
      loop:
        - ca-metrics.service
        - ca-metrics.timer
      register: ca_metrics_units

    - name: Reload systemd for the CA metrics units
      ansible.builtin.systemd:
        daemon_reload: true
      when: ca_metrics_units.changed

    - name: Enable the CA metrics timer
      ansible.builtin.systemd:
        name: ca-metrics.timer
        enabled: true
        state: started

    - name: Run the collector once so metrics exist immediately
      ansible.builtin.command:
        cmd: "{{ pki_dir }}/ca-metrics.sh"
      changed_when: false
```

Starting a timer is not restarting a service, so this stays inside the Task 2
constraint about `dns01` and `dns02`.

- [ ] **Step 4: Syntax-check and lint**

Run:
```bash
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/infrastructure/homelab_ca.yaml --syntax-check
make validate
```
Expected: both clean.

- [ ] **Step 5: Verify the metrics output shape**

Blocked until the CA exists. Once it does:

```bash
ssh agentbox "cat /data01/services/node-exporter/text_files/homelab_ca.prom"
```

Expected: `homelab_ca_trust_installed 1`, `homelab_ca_cert_verifies 1`,
`homelab_ca_key_mode 600`.

- [ ] **Step 6: Commit**

```bash
git add files/homelab-ca/ca-metrics.sh.j2 \
        files/homelab-ca/ca-metrics.service.j2 \
        files/homelab-ca/ca-metrics.timer.j2 \
        playbooks/individual/infrastructure/homelab_ca.yaml
git commit -m "feat(homelab-ca): textfile metrics for trust and certificate state"
```

---

## Task 6: The decommissioning step

Spec F2. Without this, a retired host's disk stays a valid fleet credential forever,
because nothing expires and nothing revokes.

**Files:**
- Modify: `docs/architecture/decommissioning.md`

- [ ] **Step 1: Read the ADR**

Read `docs/architecture/decommissioning.md` in full and find where host-level
teardown steps are listed. Match the document's existing structure and voice rather
than appending a differently-shaped section.

- [ ] **Step 2: Add the step**

Add a step requiring, as part of decommissioning any host:

```markdown
- **Destroy the host's PKI material.** `shred -u /etc/homelab-pki/<host>.key` and
  remove `/etc/homelab-pki/`. The homelab CA issues certificates that never expire
  and it has no revocation mechanism (see
  `docs/specs/2026-10-14-homelab-ca-design.md` D2), so a decommissioned
  host's key remains a valid fleet credential indefinitely. A recovered disk is a
  working identity on the internal network. If the disk cannot be wiped, the CA must
  be rotated; the flag-day procedure is in `docs/operations/homelab-ca.md`.
```

- [ ] **Step 3: Commit**

```bash
git add docs/architecture/decommissioning.md
git commit -m "docs(adr-0002): wipe PKI material when decommissioning a host"
```

---

## Deploy mapping check before opening the PR

- [ ] **Confirm ownership resolves.** `files/homelab-ca/` must map to
  `playbooks/individual/infrastructure/homelab_ca.yaml` via the `files/<svc>`
  convention, and `vars/vars_homelab_ca.yaml` by name. Run the repo's own checks:

```bash
bash .github/workflows/lib/ownership-coverage.test.sh
bash .github/workflows/lib/detect-impacted-playbooks.test.sh
```

Expected: both pass. An unmapped `files/` or `vars/` input is a hard error (exit 3)
in the deploy detector, so this is caught here or it fails the push.

- [ ] **Confirm the impact set is this playbook alone.** Not the site-wide
  orchestrators. If `01_base_system.yaml`, `02_core_infrastructure.yaml` or
  `03_ocean_services.yaml` appear in the detected set, something was added to
  `inventories/**` or `group_vars/all*`. Fix it rather than shipping it.

- [ ] **Confirm `make validate` is clean.**

## PR

One PR, one service, branch `agent/homelab-ca`.

The PR description must state plainly:

1. `files/homelab-ca/ca.crt` is a placeholder; a human must run the Task 0 procedure
   before this does anything.
2. Which verification steps were run and which were blocked on that, with the actual
   command output rather than a claim. The negative `nameConstraints` test (Task 4
   Step 6) is not blocked and must be included.
3. That `hosts: all` includes `dns01` and `dns02`, and that the play adds a trust
   anchor and starts a timer without restarting anything on them.

**Do not merge.** Merging to master runs a real apply against production. This
playbook touches all nine hosts including the DNS pair, and it is a new capability
rather than a repair, so it falls outside what an agent may merge under `agents.md`.
Open it, label it, stop.
