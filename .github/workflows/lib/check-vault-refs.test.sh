#!/usr/bin/env bash
# Does the vault-reference check catch the two failures it exists for?
# Run from repo root: bash .github/workflows/lib/check-vault-refs.test.sh
#
# Both failures produce a green CI and a broken deploy, which is why a static
# check is worth having at all:
#   1. play renders a template reading vault vars but never loads the vault
#      -> apply dies with "'media_services' is undefined" (bazarr, 2026-09-01)
#   2. template references a vault path that does not exist
#      -> ansible renders "", the deploy succeeds, the service breaks silently
#
# Works on a real temp repo rather than mocks: the check reads playbooks and
# templates off disk, so faking that would test the fake.
set -uo pipefail
cd "$(dirname "$0")/../../.." || exit 1
CHECK="$PWD/.github/workflows/lib/check-vault-refs.py"

PASS=0; FAIL=0
ok()  { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# Minimal repo shaped like the real one. No vault password, so the script runs
# in its degraded mode -- which still has to catch the missing-vault case.
build() {  # $1 = "vault"|"novault" ; $2 = template body
  rm -rf "$TMP/repo"
  mkdir -p "$TMP/repo/playbooks/individual/x" "$TMP/repo/files/svc" \
           "$TMP/repo/.github/workflows/lib" "$TMP/repo/vault"
  cp "$CHECK" "$TMP/repo/.github/workflows/lib/"
  printf '%s\n' "$2" > "$TMP/repo/files/svc/app.env.j2"
  {
    echo '---'
    echo '- name: t'
    echo '  hosts: x'
    echo '  vars_files:'
    echo '    - "{{ playbook_dir }}/../../../vars/vars_service_ports.yaml"'
    [ "$1" = "vault" ] && echo '    - "{{ playbook_dir }}/../../../vault/secrets.yaml"'
    echo '  vars:'
    echo '    files: "{{ playbook_dir }}/../../../files/svc"'
    echo '  tasks:'
    echo '  - name: render'
    echo '    ansible.builtin.template:'
    echo '      src: "{{ files }}/app.env.j2"'
    echo '      dest: /tmp/app.env'
  } > "$TMP/repo/playbooks/individual/x/svc.yaml"
}
run() { ( cd "$TMP/repo" && python3 .github/workflows/lib/check-vault-refs.py /nonexistent-passfile >/dev/null 2>&1 ); }

# 1. THE BAZARR BUG: reads a vault var, play does not load the vault.
build novault 'KEY={{ media_services.bazarr.api_key }}'
run && bad "missed a template reading vault vars with no vault loaded" \
     || ok "catches a play that renders vault vars without loading the vault"

# 2. Same template, vault loaded -> fine.
build vault 'KEY={{ media_services.bazarr.api_key }}'
run && ok "allows the same template once the play loads the vault" \
     || bad "false positive when the vault IS loaded"

# 3. An optional reference is the author saying it may be absent.
build novault "KEY={{ media_services.bazarr.api_key | default('') }}"
run && ok "allows a reference guarded by | default()" \
     || bad "false positive on a default-guarded reference"

# 4. A quoted literal that merely looks like a vault path.
build vault "HOST={{ smtp.host | default('smtp.gmail.com') }}"
run && ok "allows a dotted string inside a quoted literal" \
     || bad "false positive on smtp.gmail.com inside quotes"

# 5. Nothing vault-ish at all.
build novault 'PORT={{ service_ports.bazarr }}'
run && ok "ignores templates that read no vault variables" \
     || bad "false positive on a non-vault variable"

# The real repo must stay clean, or the gate cannot be enabled.
if python3 "$CHECK" >/dev/null 2>&1; then
  ok "the repository itself passes"
else
  bad "the repository does not pass its own check"
fi

echo
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -eq 0 ]
