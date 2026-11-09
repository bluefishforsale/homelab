#!/usr/bin/env python3
"""Print the email of everyone the Plex server is shared with, owner first.

The vault's `cloudflare.access_allow_emails.plex-users` list is meant to be exactly
these people, and it drifts every time someone is invited in the Plex UI. One email per
line, so it pipes straight into the vault:

  scripts/plex-shared-users.py                  # print, e.g. to address a mail to everyone
  scripts/plex-shared-users.py | scripts/vault.py replace-list cloudflare.access_allow_emails.plex-users

The vault alone changes nobody's access. terraform-root applies the "Allow plex users"
Access policy from its repo secret TF_VAR_ACCESS_PLEX_USER_EMAILS, so after the vault
edit merges, reset that secret from the vault and apply terraform-root (the command is
in terraform-root's agents.md, "Where the Access (tunnel) user emails live").

The plex.tv account token comes from the vault (media_services.plex.api_key), or
PLEX_TV_TOKEN wins if set.
"""
import json
import os
import re
import sys
import urllib.request
import xml.etree.ElementTree as ET

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vault  # noqa: E402


def plex_tv(path, token):
    req = urllib.request.Request(f"https://plex.tv{path}",
                                 headers={"X-Plex-Token": token, "Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=20) as r:
        return r.read()


# NOTE: these emails become an Access allow rule. A quote, backslash, brace or space
# in one has no business there and would survive into the secret and the HCL, so
# refuse the whole list rather than pass it on.
EMAIL = re.compile(r"[^\s\"\\{}@]+@[^\s\"\\{}@]+\.[^\s\"\\{}@]+")


def shared_here(user, machine_id):
    """A friend can hold shares on other people's servers too; only ours counts."""
    return any(s.get("machineIdentifier") == machine_id and s.get("numLibraries", "0") != "0"
               for s in user.findall("Server"))


def main():
    plex = vault.load(vault.read())["media_services"]["plex"]
    token = os.environ.get("PLEX_TV_TOKEN") or plex["api_key"]
    owner = json.loads(plex_tv("/api/v2/user", token))["email"].lower()
    # /api/users answers XML only
    users = ET.fromstring(plex_tv("/api/users", token)).findall("User")
    shared = sorted({u.get("email").lower() for u in users
                     if u.get("email") and shared_here(u, plex["pms_identifier"])} - {owner})
    bad = [e for e in [owner, *shared] if not EMAIL.fullmatch(e)]
    if bad:
        sys.exit(f"plex-shared-users.py: refusing malformed emails: {bad}")
    print(f"{len(shared)} shared users + owner", file=sys.stderr)
    print("\n".join([owner, *shared]))


if __name__ == "__main__":
    main()
