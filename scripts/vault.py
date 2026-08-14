#!/usr/bin/env python3
"""Query and rotate keys in the ansible-vault secrets file without hand-editing it.

`ansible-vault edit` already covers "open it in $EDITOR". This covers the things a
script (or an agent) needs: enumerate the tree with values redacted, read one value,
compare a value you already have against what's stored (did that rotation land?), and
set/rotate a value in place.

Writes are line-edits on the decrypted text, never a YAML round-trip, so comments,
quoting, block scalars and ordering survive. The result is parsed and diffed against
the original before it is re-encrypted (exactly one path may change), then encrypted
to a sibling temp file and decrypted back to prove it round-trips before it replaces
the vault. Nothing overwrites the vault until that check passes.

Assumes the vault's 2-space indentation. A deeper file is refused, never mangled.
Exit codes: 0 ok, 1 `check` mismatch, 2 error.
"""
import argparse
import json
import os
import re
import subprocess
import sys

import yaml

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
VAULT = os.environ.get("HOMELAB_VAULT", os.path.join(REPO, "vault/secrets.yaml"))
PASS = os.environ.get("ANSIBLE_VAULT_PASSWORD_FILE", os.path.expanduser("~/.ansible_vault_pass"))
KEY_RE = re.compile(r"([A-Za-z0-9_.-]+):(\s*)(.*)$")


def die(msg):
    print(f"vault.py: {msg}", file=sys.stderr)
    sys.exit(2)


def run(*cmd, stdin=None):
    r = subprocess.run(cmd, input=stdin, capture_output=True, text=True)
    if r.stderr.strip():
        # ansible-vault warns and still exits 0 for a bad password file; that warning is
        # the only signal the vault got encrypted with a fallback password
        print(r.stderr.strip(), file=sys.stderr)
    if r.returncode:
        die(f"{' '.join(cmd[:2])} failed")
    return r.stdout


def read():
    text = run("ansible-vault", "view", "--vault-password-file", PASS, VAULT)
    if not text.strip():
        die(f"{VAULT} decrypted to nothing; refusing to treat that as an empty tree")
    return text


def write(text):
    """Encrypt beside the vault, prove it decrypts back to `text`, then replace it."""
    tmp = VAULT + ".new"
    run("ansible-vault", "encrypt", "--encrypt-vault-id", "default",
        "--vault-password-file", PASS, "--output", tmp, "-", stdin=text)
    if run("ansible-vault", "view", "--vault-password-file", PASS, tmp) != text:
        die(f"re-encrypted vault does not decrypt to what we wrote; it is at {tmp}, {VAULT} untouched")
    os.chmod(tmp, 0o600)  # ciphertext, but no reason to leave it readable to other local users
    os.replace(tmp, VAULT)
    print(f"wrote {VAULT}", file=sys.stderr)


def children(lines, start, end, indent):
    """Map keys at exactly `indent` within [start, end) -> (key line, end of its block)."""
    out = {}
    i = start
    while i < end:
        ln = lines[i]
        s = ln.strip()
        if not s or s.startswith("#"):
            i += 1
            continue
        ind = len(ln) - len(ln.lstrip())
        if ind < indent:
            break
        if ind > indent:  # nested map, list item, or block-scalar content
            i += 1
            continue
        m = KEY_RE.match(s)
        if m:
            j = i + 1
            while j < end:
                nxt = lines[j]
                if nxt.strip() and not nxt.strip().startswith("#") and len(nxt) - len(nxt.lstrip()) <= indent:
                    break
                j += 1
            out[m.group(1)] = (i, j)
        i += 1
    return out


def locate(lines, path):
    """Walk `path`; return (matched depth, key line index or None, block end, indent)."""
    start, end, indent, idx = 0, len(lines), 0, None
    for depth, part in enumerate(path):
        found = children(lines, start, end, indent).get(part)
        if not found:
            return depth, None, end, indent
        idx, end = found
        start, indent = idx + 1, indent + 2
    return len(path), idx, end, indent - 2


def flatten(node, prefix=()):
    """Leaf paths as tuples, not joined strings: a key containing a `.` must not
    collide with a nested path and hide a change from the pre-write diff."""
    if isinstance(node, dict):
        for k, v in node.items():
            yield from flatten(v, prefix + (str(k),))
    else:
        yield prefix, node


def load(text, what="vault plaintext"):
    try:
        data = yaml.safe_load(text) or {}
    except yaml.YAMLError as e:
        # never echo the exception: its message quotes the offending line, i.e. a secret
        mark = getattr(e, "problem_mark", None)
        die(f"{what} is not valid YAML" + (f" (line {mark.line + 1})" if mark else ""))
    dotted = [k for k, _ in flatten(data) if any("." in part for part in k)]
    if dotted:
        # `a.b.c` would be ambiguous between a nested path and a literal `a.b` key, and
        # the tool would quietly create the nested one alongside it
        die(f"{what} has a key containing a dot ({'.'.join(dotted[0])}); this tool cannot address it")
    return data


def resolve(data, path):
    for part in path:
        if not isinstance(data, dict) or part not in data:
            return None, False
        data = data[part]
    return data, True


def value_at(data, path):
    v, ok = resolve(data, path)
    dotted = ".".join(path)
    if not ok:
        die(f"no such path: {dotted}")
    if isinstance(v, (dict, list)):
        die(f"{dotted} is a {type(v).__name__}, not a value; use `list {dotted}`")
    return v


def supplied(value):
    return sys.stdin.read().rstrip("\n") if value == "-" else value


def redact(v):
    # no digest, and no passthrough for numbers: `list` output ends up in agent
    # transcripts and CI logs, and an unsalted hash of a short secret is a crackable
    # oracle there. `check` answers "does my value match" without publishing anything
    if isinstance(v, list):
        return f"[list: {len(v)} items]"
    return f"**** len={len(str(v))}"


def strip_scalar(rest):
    """Everything after the value on a `key: value` line, quotes respected."""
    if rest[:1] in ("\"", "'"):
        q, i = rest[0], 1
        while i < len(rest):
            if rest[i] == "\\" and q == '"':
                i += 2
                continue
            if rest[i] == q:
                return rest[i + 1:].strip()
            i += 1
        die("cannot edit a scalar whose quotes do not close on the key line")
    _, _, tail = rest.partition(" #")
    return f"#{tail}" if tail else ""


def edit(text, path, value):
    lines = text.split("\n")
    depth, idx, end, indent = locate(lines, path)
    # always double-quoted (JSON is a YAML subset for strings): everything here came
    # from argv as a string, and bare `0755`/`TRUE` would come back as 493/True
    scalar = json.dumps(value)

    if depth == len(path):
        m = KEY_RE.match(lines[idx].strip())
        if m.group(3).strip() in ("|", ">", "|-", ">-", ""):
            die(f"{'.'.join(path)} is a block scalar or a map, not an inline value")
        # keep any trailing comment, but only a real one: a `#` inside the quoted
        # value is part of the old secret and must not survive the rotation
        tail = strip_scalar(m.group(3))
        tail = f"  {tail}" if tail.startswith("#") else ""
        lines[idx] = " " * indent + f"{path[depth - 1]}: {scalar}{tail}"
    else:
        block = [" " * ((depth + i) * 2) + f"{p}:" for i, p in enumerate(path[depth:-1])]
        block.append(" " * (len(path[:-1]) * 2) + f"{path[-1]}: {scalar}")
        # back up over trailing blanks AND comments: a banner comment sits above the
        # section it labels, so inserting under it would steal the next section's header
        while end > 0 and (not lines[end - 1].strip() or lines[end - 1].lstrip().startswith("#")):
            end -= 1
        lines[end:end] = block

    new = "\n".join(lines)
    before, after = dict(flatten(load(text))), dict(flatten(load(new, "the edited vault")))
    touched = {k for k in before.keys() | after.keys() if before.get(k, ()) != after.get(k, ())}
    if touched != {tuple(path)}:
        die(f"refusing to write: edit would change {sorted('.'.join(t) for t in touched) or 'nothing'}")
    return new


def cmd_list(args):
    prefix = tuple(args.path.split(".")) if args.path else ()
    node, ok = resolve(load(read()), prefix)
    if not ok:
        die(f"no such path: {args.path}")
    for k, v in flatten(node, prefix):
        print(f"{'.'.join(k)}: {redact(v)}")


def cmd_get(args):
    print(value_at(load(read()), args.path.split(".")))


def same(stored, expected):
    # a YAML-native `true` stringifies as "True", and answering "differ" to
    # `check flag true` is a false negative on exactly the question this tool exists for
    return str(stored).lower() == expected.lower() if isinstance(stored, bool) else str(stored) == expected


def cmd_check(args):
    stored = value_at(load(read()), args.path.split("."))
    if same(stored, supplied(args.value)):
        print("match")
        return 0
    print("differ")
    return 1


def cmd_set(args):
    text = read()
    path = args.path.split(".")
    value = supplied(args.value)
    current, ok = resolve(load(text), path)
    if ok and args.cmd == "set":
        die(f"{args.path} exists; use `rotate` to replace it")
    if not ok and args.cmd == "rotate":
        die(f"{args.path} does not exist; use `set` to add it")
    if ok and same(current, value):
        # the asked-for end state already holds; a rerun of a rotation script is not a failure
        print(f"{args.path} already holds that value", file=sys.stderr)
        return 0
    write(edit(text, path, value))
    return 0


def main():
    p = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    sub = p.add_subparsers(dest="cmd", required=True)

    ls = sub.add_parser("list", help="print every leaf path with its value redacted")
    ls.add_argument("path", nargs="?", help="dotted path to list under")
    ls.set_defaults(fn=cmd_list)

    g = sub.add_parser("get", help="print one value in the clear")
    g.add_argument("path")
    g.set_defaults(fn=cmd_get)

    c = sub.add_parser("check", help="compare a value against what is stored (exit 1 on mismatch)")
    c.add_argument("path")
    c.add_argument("value", help="value to compare, or - to read stdin")
    c.set_defaults(fn=cmd_check)

    for name, helptext in (("set", "add a new key"), ("rotate", "replace an existing value")):
        s = sub.add_parser(name, help=helptext)
        s.add_argument("path")
        s.add_argument("value", help="new value, or - to read stdin (keeps it out of shell history and ps)")
        s.set_defaults(fn=cmd_set)

    args = p.parse_args()
    sys.exit(args.fn(args) or 0)


if __name__ == "__main__":
    main()
