#!/usr/bin/env python3
"""Print the service_ports.<key> names whose value changed between two versions
of vars_service_ports.yaml.

Usage: changed-service-port-keys.py <base.yaml> <head.yaml>

Compares the parsed `service_ports:` mapping structurally (so comment/whitespace
edits produce no output) and prints one changed key per line: added, removed, or
value-modified. Exit 2 on a load/parse failure so the caller can fall back to
deploying every consumer rather than under-deploying.
"""
import sys
import yaml


def load_ports(path):
    with open(path) as f:
        doc = yaml.safe_load(f) or {}
    ports = doc.get("service_ports")
    return ports if isinstance(ports, dict) else {}


def main():
    if len(sys.argv) != 3:
        print("usage: changed-service-port-keys.py <base> <head>", file=sys.stderr)
        return 2
    try:
        base = load_ports(sys.argv[1])
        head = load_ports(sys.argv[2])
    except (OSError, yaml.YAMLError):
        return 2
    changed = sorted(k for k in set(base) | set(head) if base.get(k) != head.get(k))
    if changed:
        print("\n".join(changed))
    return 0


if __name__ == "__main__":
    sys.exit(main())
