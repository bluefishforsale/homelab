#!/bin/sh
# The macOS twin of base/authorized_keys.yaml's refresh: revoking a key on
# GitHub reaches this host within 5 minutes. Writes a temp file and renames it,
# so a failed download never locks anyone out.
set -eu

user="$1"
url="$2"
ssh_dir="$(dscl . -read "/Users/$user" NFSHomeDirectory | awk '{print $2}')/.ssh"
install -d -o "$user" -g staff -m 0700 "$ssh_dir"
tmp="$(mktemp "$ssh_dir/.authorized_keys.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
if ! curl -fsS --max-time 15 "$url" -o "$tmp"; then
  echo "refresh-authorized-keys: fetch failed, keeping $ssh_dir/authorized_keys" >&2
  exit 1
fi
# An empty answer would lock everyone out; keep the old file instead.
[ -s "$tmp" ] || { echo "refresh-authorized-keys: empty key list, not applied" >&2; exit 1; }
chown "$user":staff "$tmp"
chmod 0600 "$tmp"
mv "$tmp" "$ssh_dir/authorized_keys"
trap - EXIT
