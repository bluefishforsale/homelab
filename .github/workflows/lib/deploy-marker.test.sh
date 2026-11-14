#!/usr/bin/env bash
# Tests for deploy-base.sh and mark-deployed.sh, against a throwaway repo and
# bare remote. Each case names the deploy bug it pins.
# Run from repo root: bash .github/workflows/lib/deploy-marker.test.sh
set -euo pipefail

LIB="$(cd "$(dirname "$0")" && pwd)"
PASS=0
FAIL=0

assert_eq() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $name"
    echo "  expected: $expected"
    echo "  actual:   $actual"
    FAIL=$((FAIL + 1))
  fi
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
git init -q --bare "$TMP/remote.git"
git clone -q "$TMP/remote.git" "$TMP/work" 2>/dev/null
cd "$TMP/work"
git config user.email t@example.com
git config user.name t
commit() { git commit -q --allow-empty -m "$1"; git rev-parse HEAD; }
c1=$(commit one)
c2=$(commit two)
c3=$(commit three)
c4=$(commit four)
git push -q origin HEAD:refs/heads/master

assert_eq "no marker yet: diff the last commit only" \
  "$c3" "$(bash "$LIB/deploy-base.sh" "$c4")"

bash "$LIB/mark-deployed.sh" "$c2" >/dev/null
git fetch -q origin
assert_eq "first clean apply creates the marker" \
  "$c2" "$(git rev-parse origin/deployed)"

assert_eq "cancelled run's commits stay in the next range" \
  "$c2" "$(bash "$LIB/deploy-base.sh" "$c4")"

bash "$LIB/mark-deployed.sh" "$c4" >/dev/null
bash "$LIB/mark-deployed.sh" "$c3" >/dev/null
git fetch -q origin
assert_eq "slower older run never drags the marker backwards" \
  "$c4" "$(git rev-parse origin/deployed)"

assert_eq "marker at head: empty range, nothing redeployed" \
  "$c4" "$(bash "$LIB/deploy-base.sh" "$c4")"

git checkout -q -b side "$c1"
s1=$(commit side)
assert_eq "marker off head's history falls back to one commit" \
  "$c1" "$(bash "$LIB/deploy-base.sh" "$s1")"

echo ""
echo "Passed: $PASS  Failed: $FAIL"
[[ "$FAIL" -eq 0 ]]
