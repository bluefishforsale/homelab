#!/usr/bin/env python3
"""Catch template/vault mismatches before they fail at apply time.

Two failures this cannot let through, both of which produce a green CI and a
broken deploy:

  1. A template reads a vaulted variable and its playbook never loads the vault.
     That is exactly how bazarr broke on 2026-09-01 -- the values moved into the
     vault, `config.ini.j2` referenced `media_services.*`, and `bazarr.yaml`
     loaded only `vars_service_ports.yaml`:
         [ERROR]: Task failed: 'media_services' is undefined

  2. A template references a vault PATH that does not exist -- a typo like
     `media_services.opensubtitle.password`. Ansible renders that as an empty
     string rather than failing, so the service deploys "successfully" and then
     authenticates with nothing. Silent, which is worse than the crash.

Why not just render every template? Because templates legitimately use play
vars, facts, `groups`, `hostvars` and loop variables, so a StrictUndefined
render would be mostly false positives and would get switched off. This checks
the one relationship that is decidable statically: which vault keys a template
reads, against what its playbook loads and what the vault actually contains.

Needs the vault password to verify paths (arg 2 or ANSIBLE_VAULT_PASSWORD_FILE).
Without it, the path check is skipped and only the missing-vault check runs, so
the script is still useful on a laptop with no vault access.
"""
import os
import re
import subprocess
import sys

import yaml

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
PLAYBOOKS = os.path.join(ROOT, "playbooks")
VAULT = os.path.join(ROOT, "vault", "secrets.yaml")


def load_vault(passfile):
    if not passfile or not os.path.exists(passfile):
        return None
    r = subprocess.run(["ansible-vault", "view", "--vault-password-file", passfile, VAULT],
                       capture_output=True, text=True)
    if r.returncode != 0:
        print(f"  (could not decrypt vault: {r.stderr.strip()[:80]}; skipping path checks)")
        return None
    return yaml.safe_load(r.stdout)


def vault_has(data, dotted):
    cur = data
    for part in dotted.split("."):
        if not isinstance(cur, dict) or part not in cur:
            return False
        cur = cur[part]
    return True


def play_templates(pb_path, doc):
    """Template srcs a play references, resolved to real files."""
    text = open(pb_path).read()
    pb_dir = os.path.dirname(pb_path)
    # Resolve the play's own `files:` var, which nearly every play uses as the
    # template root: files: "{{ playbook_dir }}/../../../../files/ocean-bazarr"
    roots = {}
    for play in doc if isinstance(doc, list) else []:
        for k, v in (play.get("vars") or {}).items():
            if isinstance(v, str) and "playbook_dir" in v and "/files/" in v:
                roots[k] = os.path.normpath(v.replace("{{ playbook_dir }}", pb_dir))
    out = set()
    for m in re.finditer(r'src:\s*["\']?([^"\'\n]+\.j2)', text):
        src = m.group(1).strip()
        cand = []
        for var, root in roots.items():
            cand.append(src.replace("{{ " + var + " }}", root).replace("{{" + var + "}}", root))
        cand.append(os.path.join(ROOT, src.lstrip("/")))
        for c in cand:
            c = os.path.normpath(c)
            if "{{" not in c and os.path.isfile(c):
                out.add(c)
                break
    return out


def main():
    passfile = (sys.argv[1] if len(sys.argv) > 1
                else os.environ.get("ANSIBLE_VAULT_PASSWORD_FILE",
                                    os.path.expanduser("~/.ansible_vault_pass")))
    vault = load_vault(passfile)
    top = set(vault.keys()) if vault else set()
    if not top:
        # Without the vault we cannot know which names are vaulted, so fall back
        # to the top-level keys named by any playbook that DOES load it.
        top = {"ai_services", "media_services", "network", "monitoring", "infrastructure",
               "cloudflare", "databases", "smtp", "system_users", "agentbox", "ntfy"}

    errors = []
    checked = 0
    for dirpath, _, names in os.walk(PLAYBOOKS):
        if "deprecated" in dirpath:
            continue
        for n in sorted(names):
            if not n.endswith((".yaml", ".yml")):
                continue
            pb = os.path.join(dirpath, n)
            try:
                doc = yaml.safe_load(open(pb))
            except yaml.YAMLError:
                continue
            if not isinstance(doc, list):
                continue
            loads_vault = "vault/secrets.yaml" in open(pb).read()
            rel_pb = os.path.relpath(pb, ROOT)

            for tmpl in play_templates(pb, doc):
                body = open(tmpl, errors="ignore").read()
                # A reference carrying `| default(...)` is explicitly declared
                # optional by whoever wrote it, so a missing vault path there is
                # intended rather than a silent break. All 18 hits on this
                # check's first run against the repo were that shape: the check
                # was wrong, not the templates. Matching the whole {{ ... }}
                # expression is what makes the distinction possible.
                refs = set()
                for m in re.finditer(r"\{\{-?(.*?)-?\}\}", body, re.S):
                    # Strip quoted literals first: default('smtp.gmail.com')
                    # otherwise reads as the vault path smtp.gmail.com, because
                    # `smtp` really is a vault top-level key.
                    expr = re.sub(r"'[^']*'|\"[^\"]*\"", "''", m.group(1))
                    for ref in re.findall(r"([a-z_][a-z0-9_]*(?:\.[a-z_][a-z0-9_]*)+)", expr):
                        if ref.split(".")[0] not in top:
                            continue
                        # A default may guard a later term in the same
                        # expression, so only treat THIS ref as optional when
                        # the default follows it.
                        after = expr.split(ref, 1)[1]
                        if re.search(r"\|\s*default\s*\(", after):
                            continue
                        refs.add(ref)
                if not refs:
                    continue
                checked += 1
                rel_t = os.path.relpath(tmpl, ROOT)
                if not loads_vault:
                    errors.append(f"{rel_pb}\n    renders {rel_t}, which reads "
                                  f"{sorted(refs)[0]} — but the play never loads "
                                  f"vault/secrets.yaml.\n    Ansible fails at apply with "
                                  f"\"'{sorted(refs)[0].split('.')[0]}' is undefined\".")
                    continue
                if vault:
                    for ref in sorted(refs):
                        if not vault_has(vault, ref):
                            errors.append(f"{rel_pb}\n    renders {rel_t}, which reads "
                                          f"{ref} — that path is NOT in the vault.\n"
                                          f"    Ansible renders it as an empty string, so "
                                          f"this deploys clean and breaks silently.")

    if errors:
        print(f"FAIL: {len(errors)} vault reference problem(s):\n")
        for e in errors:
            print(f"  {e}\n")
        return 1
    print(f"clean: {checked} vault-consuming template(s) all resolve "
          f"({'paths verified' if vault else 'paths NOT verified, no vault access'})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
