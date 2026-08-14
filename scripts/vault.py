#!/usr/bin/env python3
"""Query and rotate keys in the ansible-vault secrets file without hand-editing it.

`ansible-vault edit` already covers "open it in $EDITOR". This covers the things a
script (or an agent) needs: enumerate the tree with values redacted, read one value,
compare a value you already have against what's stored (did that rotation land?), and
set/rotate a value in place.

Writes are line-edits on the decrypted text, never a YAML round-trip, so comments,
quoting, block scalars and ordering survive. The result is parsed and diffed against
the original before it is re-encrypted: exactly one path may change.
"""
import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile

import yaml

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
VAULT = os.environ.get("HOMELAB_VAULT", os.path.join(REPO, "vault/secrets.yaml"))
PASS = os.environ.get("ANSIBLE_VAULT_PASSWORD_FILE", os.path.expanduser("~/.ansible_vault_pass"))
KEY_RE = re.compile(r"([A-Za-z0-9_.-]+):(\s*)(.*)$")


def die(msg):
    sys.exit(f"vault.py: {msg}")


def read():
    r = subprocess.run(
        ["ansible-vault", "view", "--vault-password-file", PASS, VAULT],
        capture_output=True, text=True,
    )
    if r.returncode:
        die(r.stderr.strip() or "decrypt failed")
    return r.stdout


def write(text):
    """Re-encrypt `text` over the vault file, keeping the previous ciphertext in TMPDIR."""
    with open(VAULT) as f:
        prev = f.read()
    bak = os.path.join(tempfile.gettempdir(), "secrets.yaml.bak")
    with open(bak, "w") as f:
        f.write(prev)
    r = subprocess.run(
        ["ansible-vault", "encrypt", "--encrypt-vault-id", "default",
         "--vault-password-file", PASS, "--output", VAULT, "-"],
        input=text, capture_output=True, text=True,
    )
    if r.returncode:
        die(r.stderr.strip() or "encrypt failed")
    print(f"wrote {VAULT} (previous ciphertext: {bak})", file=sys.stderr)


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
    if isinstance(node, dict):
        for k, v in node.items():
            yield from flatten(v, prefix + (str(k),))
    else:
        yield ".".join(prefix), node


def load(text):
    return yaml.safe_load(text) or {}


def resolve(data, path):
    for part in path:
        if not isinstance(data, dict) or part not in data:
            return None, False
        data = data[part]
    return data, True


def redact(v):
    if isinstance(v, list):
        return f"[list: {len(v)} items]"
    if isinstance(v, bool) or isinstance(v, int) or isinstance(v, float):
        return str(v)
    s = str(v)
    digest = hashlib.sha256(s.encode()).hexdigest()[:8]
    return f"**** len={len(s)} sha256:{digest}"


def render(value):
    """YAML scalar for a value supplied on the command line."""
    if value.lower() in ("true", "false", "null") or re.fullmatch(r"-?\d+(\.\d+)?", value):
        return value
    return json.dumps(value)  # double-quoted YAML is JSON-compatible for strings


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
        return ""
    _, _, tail = rest.partition(" #")
    return f"#{tail}" if tail else ""


def edit(text, path, value):
    lines = text.split("\n")
    depth, idx, end, indent = locate(lines, path)
    scalar = render(value)

    if depth == len(path):
        ln = lines[idx]
        m = KEY_RE.match(ln.strip())
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
        while end > 0 and not lines[end - 1].strip():
            end -= 1
        lines[end:end] = block

    new = "\n".join(lines)
    try:
        after = dict(flatten(load(new)))
    except yaml.YAMLError as e:
        die(f"edit produced invalid YAML: {e}")
    before = dict(flatten(load(text)))
    touched = {k for k in before.keys() | after.keys() if before.get(k, ()) != after.get(k, ())}
    if touched != {".".join(path)}:
        die(f"refusing to write: edit would change {sorted(touched) or 'nothing'}")
    return new


def cmd_list(args):
    data = load(read())
    prefix = args.path.split(".") if args.path else []
    node, ok = resolve(data, prefix)
    if not ok:
        die(f"no such path: {args.path}")
    for k, v in flatten(node, tuple(prefix)):
        print(f"{k}: {redact(v)}")


def cmd_get(args):
    v, ok = resolve(load(read()), args.path.split("."))
    if not ok:
        die(f"no such path: {args.path}")
    print(v if isinstance(v, str) else yaml.safe_dump(v, default_flow_style=False).rstrip())


def cmd_check(args):
    v, ok = resolve(load(read()), args.path.split("."))
    if not ok:
        die(f"no such path: {args.path}")
    expected = sys.stdin.read().rstrip("\n") if args.value == "-" else args.value
    match = str(v) == expected
    print("match" if match else f"differ (stored sha256:{hashlib.sha256(str(v).encode()).hexdigest()[:8]})")
    return 0 if match else 1


def cmd_set(args):
    text = read()
    current, ok = resolve(load(text), args.path.split("."))
    if ok and args.cmd == "set":
        die(f"{args.path} exists; use `rotate` to replace it")
    if not ok and args.cmd == "rotate":
        die(f"{args.path} does not exist; use `set` to add it")
    if ok and str(current) == args.value:
        die(f"{args.path} already holds that value; nothing to rotate")
    write(edit(text, args.path.split("."), args.value))
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
        s.add_argument("value")
        s.set_defaults(fn=cmd_set)

    args = p.parse_args()
    sys.exit(args.fn(args) or 0)


if __name__ == "__main__":
    main()
