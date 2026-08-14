#!/usr/bin/env python3
"""Pins the line-editor in vault.py: run `python3 scripts/test_vault.py` (no pytest).

Every case here is something that would silently corrupt the secrets file if the
indentation walker regressed.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vault

DOC = """# header comment
top: one
contacts:
  terrac:
    email: "a@b.com"  # primary
    phone: "+1"
    leaky: "old #frag"
    plain: bare # why
tunnel:
  cert_pem: |
    -----BEGIN-----
    nested: not-a-key
    -----END-----
  token: "abc"
lists:
  members:
    - one
    - two
"""


def check(path, value, expect_line, doc=DOC):
    out = vault.edit(doc, path, value)
    assert expect_line in out.split("\n"), f"{path}: missing {expect_line!r} in\n{out}"
    return out


def main():
    # rotate a nested value, keeping its trailing comment
    check(["contacts", "terrac", "email"], "c@d.com", '    email: "c@d.com"  # primary')

    # a key whose name also appears as text inside a block scalar is not confused for it
    out = check(["tunnel", "token"], "xyz", '  token: "xyz"')
    assert "    nested: not-a-key" in out, "block scalar body was rewritten"

    # a `#` inside the old quoted value is part of the secret, not a comment to keep
    check(["contacts", "terrac", "leaky"], "new", '    leaky: "new"')
    check(["contacts", "terrac", "plain"], "new", '    plain: "new"  # why')

    # add a leaf under an existing map, at the map's indent
    check(["contacts", "terrac", "signal"], "@me", '    signal: "@me"')

    # add a whole missing subtree
    out = check(["a", "b", "c"], "v", '    c: "v"')
    assert "a:" in out.split("\n") and "  b:" in out.split("\n"), out

    # scalars stay scalars, strings stay quoted
    check(["top"], "42", "top: 42")
    check(["top"], "true", "top: true")
    check(["top"], "yes", 'top: "yes"')
    check(["top"], "pa ss: word", 'top: "pa ss: word"')

    # a list leaf is left alone rather than half-rewritten
    try:
        vault.edit(DOC, ["lists", "members"], "x")
        raise AssertionError("expected refusal to overwrite a map/list key")
    except SystemExit:
        pass

    print("ok")


if __name__ == "__main__":
    main()
