#!/usr/bin/env python3
"""Print the email of everyone the Plex server is shared with, owner first.

The vault's `cloudflare.access_allow_emails.plex-users` list is meant to be exactly
these people (it seeds the "Allow plex users" Access policy), and it drifts every time
someone is invited in the Plex UI. One email per line, so it pipes straight into the vault:

  scripts/plex-shared-users.py                  # print, e.g. to address a mail to everyone
  scripts/plex-shared-users.py | scripts/vault.py replace-list cloudflare.access_allow_emails.plex-users

The plex.tv account token comes from the vault (media_services.plex.api_key), or
PLEX_TV_TOKEN wins if set.
"""
import json
import os
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


def main():
    token = os.environ.get("PLEX_TV_TOKEN") or vault.value_at(
        vault.load(vault.read()), ["media_services", "plex", "api_key"])
    owner = json.loads(plex_tv("/api/v2/user", token))["email"].lower()
    # /api/users answers XML only; every friend with a library share has an email
    users = ET.fromstring(plex_tv("/api/users", token)).findall("User")
    shared = sorted({u.get("email").lower() for u in users if u.get("email")} - {owner})
    print(f"{len(shared)} shared users + owner", file=sys.stderr)
    print("\n".join([owner, *shared]))


if __name__ == "__main__":
    main()
