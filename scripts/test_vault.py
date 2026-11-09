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

# banner for the section below
last:
  k: v
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

    # a new key lands under its own section, not under the next section's banner
    out = check(["lists", "x"], "v", '  x: "v"')
    lines = out.split("\n")
    assert lines.index('  x: "v"') < lines.index("# banner for the section below"), out

    # everything from argv is a string: bare 0755 would come back as 493, TRUE as True
    check(["top"], "0755", 'top: "0755"')
    check(["top"], "42", 'top: "42"')
    check(["top"], "TRUE", 'top: "TRUE"')
    check(["top"], "pa ss: word", 'top: "pa ss: word"')

    for doc, path, why in (
        # a list leaf must not be half-rewritten into a scalar
        (DOC, ["lists", "members"], "overwriting a list key"),
        # `svc.api.key` is ambiguous against a literal `api.key` key: refuse rather than
        # quietly create a nested twin that shadows the real one
        ('svc:\n  api.key: "old"\n', ["svc", "api", "key"], "a dotted key in the file"),
    ):
        try:
            vault.edit(doc, path, "x")
            raise AssertionError(f"expected refusal: {why}")
        except SystemExit:
            pass

    # replace a list's items, keeping its indent and the section after it
    out = vault.edit_list(DOC, ["lists", "members"], ["b@x.com", "a@x.com"])
    assert "lists:\n  members:\n    - \"b@x.com\"\n    - \"a@x.com\"\n\n# banner" in out, out

    # replacing with the same items in another order is still a write, not a refusal
    vault.edit_list(DOC, ["lists", "members"], ["two", "one"])

    for path, items, why in (
        # a scalar must not be turned into a list
        (["top"], ["x"], "overwriting a scalar with a list"),
        # an empty list would lock every user out of whatever reads it
        (["lists", "members"], [], "an empty list"),
        (["lists", "nope"], ["x"], "a missing key"),
    ):
        try:
            vault.edit_list(DOC, path, items)
            raise AssertionError(f"expected refusal: {why}")
        except SystemExit:
            pass

    print("ok")


if __name__ == "__main__":
    main()
