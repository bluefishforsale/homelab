#!/usr/bin/env python3
"""Print the inventory groups that exist only for hosts added between two
versions of an INI inventory.

Usage: new-host-groups.py <base.ini> <head.ini>

Adding a host is the one inventory change that touches nobody else, so it
should deploy that host's own playbook instead of replaying the fleet. Prints
one group per line whose members are all new hosts (e.g. [agent_hosts] for
agentbox; never [vms], which holds existing hosts too). Exits 1 when the diff
is anything other than pure host additions (a host line edited or removed, a
vars or children section changed, an existing host joining a group), so the
caller falls back to the orchestrator replay.
"""
import sys


def parse(path):
    """Return ({group: {host: line}}, [non-host lines]) for an INI inventory."""
    groups, other, section = {}, [], None
    with open(path) as f:
        for raw in f:
            line = raw.strip()
            if not line or line.startswith(("#", ";")):
                continue
            if line.startswith("["):
                section = line.strip("[]")
                other.append(line)
                continue
            if section is None or ":" in section:
                other.append(line)
                continue
            groups.setdefault(section, {})[line.split()[0]] = line
    return groups, other


def main():
    if len(sys.argv) != 3:
        print("usage: new-host-groups.py <base> <head>", file=sys.stderr)
        return 2
    base, base_other = parse(sys.argv[1])
    head, head_other = parse(sys.argv[2])

    # A new empty section header is still a pure add; anything else is not.
    new_headers = {l for l in head_other if l.startswith("[") and l not in base_other}
    if [l for l in head_other if l not in new_headers] != base_other:
        return 1
    old_hosts = {h for members in base.values() for h in members}
    for group, members in base.items():
        if any(head.get(group, {}).get(h) != line for h, line in members.items()):
            return 1
    added = {h for g, members in head.items() for h in members
             if h not in base.get(g, {})}
    if not added or added & old_hosts:
        return 1

    for group in sorted(head):
        if set(head[group]) <= added:
            print(group)
    return 0


if __name__ == "__main__":
    sys.exit(main())
