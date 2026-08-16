#!/usr/bin/env bash
# Tests for check-no-zfs-mutations.sh — the storage-safety guardrail.
# Run from repo root: bash .github/workflows/lib/check-no-zfs-mutations.test.sh
set -uo pipefail

GUARD="$(dirname "$0")/check-no-zfs-mutations.sh"
PASS=0
FAIL=0
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# A file is "bad" if the guardrail flags it (exit != 0), "good" if it passes.
expect() {
  local name="$1" want="$2" file="$3"
  if bash "$GUARD" "$file" >/dev/null 2>&1; then got=good; else got=bad; fi
  if [[ "$got" == "$want" ]]; then
    echo "PASS: $name"; PASS=$((PASS + 1))
  else
    echo "FAIL: $name (wanted $want, got $got)"; FAIL=$((FAIL + 1))
  fi
}

# Each mutating form must be flagged.
for verb in \
  "command: zpool destroy data01" \
  "command: zpool import data01" \
  "command: zpool export data01" \
  "command: zpool replace data01 sda sdb" \
  "command: zfs destroy data01/x@snap" \
  "command: zfs set mountpoint=/x data01" \
  "command: zfs mount data01" \
  "command: zfs unmount data01" \
  "command: zfs rollback data01/x@snap" \
  "shell: timeout 5 zfs create data01/new" \
  "community.general.zfs: { name: data01/x, state: absent }" \
  "ansible.posix.mount: { path: /data01, fstype: zfs, state: mounted }"; do
  printf -- '- hosts: x\n  tasks:\n    - %s\n' "$verb" > "$TMP/bad.yaml"
  expect "flags: $verb" bad "$TMP/bad.yaml"
done

# Reads and non-ZFS mounts must NOT be flagged.
cat > "$TMP/good.yaml" <<'Y'
- hosts: x
  tasks:
    - command: zpool list -H -o health data01
    - command: zpool status -v data01
    - command: zfs get -H -o value mounted data01
    - command: zfs list -H -o name,mounted -d 1 data01
    - command: systemctl is-enabled zfs-mount.service zfs.target
    - ansible.builtin.mount: { path: /mnt/ram, src: tmpfs, fstype: tmpfs, state: mounted }
    # even a comment mentioning zpool destroy must not trip it
Y
expect "allows reads + tmpfs + comment prose" good "$TMP/good.yaml"

# The real read-only ZFS play must pass.
expect "the /data01 assertion play is clean" good \
  "$(git rev-parse --show-toplevel)/playbooks/individual/ocean/data01_zfs.yaml"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
