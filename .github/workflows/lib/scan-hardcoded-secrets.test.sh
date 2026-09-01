#!/usr/bin/env bash
# Does the secret scanner actually catch a secret?
# Run from repo root: bash .github/workflows/lib/scan-hardcoded-secrets.test.sh
#
# The previous scanner could not fail — it ended in `|| echo "✓ no obvious
# credentials"`, so it exited 0 whether it found something or not, and nobody
# noticed for months while three live API keys and a third-party account
# password sat in files/ocean-bazarr/config.ini.j2.
#
# So the first assertion here is the one that matters: plant a secret, and
# require a NON-ZERO exit. A scanner that cannot go red is decoration.
set -uo pipefail
cd "$(dirname "$0")/../../.." || exit 1
SCAN="$PWD/.github/workflows/lib/scan-hardcoded-secrets.sh"

PASS=0; FAIL=0
ok()  { echo "PASS: $1"; PASS=$((PASS + 1)); }
bad() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

plant() {  # $1 = filename, $2 = line content
  mkdir -p "$TMP/vars"
  printf '%s\n' "$2" > "$TMP/vars/$1"
}
scan() { ( cd "$TMP" && bash "$SCAN" vars >/dev/null 2>&1 ); }

# --- must CATCH these -------------------------------------------------------
while IFS='|' read -r name content; do
  [ -z "$name" ] && continue
  plant "planted.yaml" "$content"
  if scan; then
    bad "missed a planted secret: $name"
  else
    ok "catches $name"
  fi
done <<'CASES'
a quoted api key|api_key: "eb727e9d30f540d59677e7cc08161da4"
an unquoted ini apikey|apikey = eb727e9d30f540d59677e7cc08161da4
a plaintext password|password = dvW0z8Mech&co^&L
a token literal|github_token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345
a secret literal|client_secret: 9f8e7d6c5b4a39281706
CASES

# --- must NOT flag these ----------------------------------------------------
while IFS='|' read -r name content; do
  [ -z "$name" ] && continue
  plant "planted.yaml" "$content"
  if scan; then
    ok "allows $name"
  else
    bad "false positive on: $name"
  fi
done <<'CASES'
a vault reference|api_key: "{{ media_services.radarr.api_key }}"
an env reference|api_key: "${RADARR_API_KEY}"
a commented example|# password = hunter2someexample
an ini-commented default|;client_secret = some_secret
an empty value|password =
a boolean|auth: false
a variable reference|github_token: token_data.token
a documented placeholder|api_key: "your-key-here"
CASES

# The scanner must look where secrets actually turned up: vars/ and *.j2 were
# both outside the old scan's scope, and both held live credentials.
for tok in "vars" "\*\.j2" "api_?key"; do
  if grep -qE "$tok" "$SCAN"; then
    ok "scanner covers $tok"
  else
    bad "scanner does not cover $tok"
  fi
done

# And it must never end in a swallow that forces exit 0.
if grep -qE '\|\|[[:space:]]*echo.*(clean|no obvious)' "$SCAN"; then
  bad "scanner swallows its own failure with || echo"
else
  ok "scanner does not swallow its exit code"
fi

echo
echo "passed: $PASS  failed: $FAIL"
[ "$FAIL" -eq 0 ]
