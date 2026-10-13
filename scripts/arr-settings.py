#!/usr/bin/env python3
"""Snapshot Radarr/Sonarr settings to JSON, diff them, and restore them.

The nightly DB backup (files/db-backup) restores everything or nothing, library
included, as of that night. This keeps just the settings, readable and diffable,
so a single profile change can be seen, committed and put back on its own.

  arr-settings.py snapshot radarr               # write arr-settings/radarr/*.json
  arr-settings.py diff radarr                   # what restore would change
  arr-settings.py restore radarr [--only customformat,qualityprofile] [--yes]

Objects are matched by name, never by database id, and every id that points at
another object (custom formats in a profile, tags, the profile an import list
uses) is stored as that object's name. Restore is a dry run without --yes,
creates or updates, and never deletes: anything live but absent from the
snapshot is only reported.

Secrets read back as ******** and the apps keep the stored value when the mask
is sent back, so updating an existing download client or notification is safe.
Creating one that carries a masked secret (a fresh install) cannot work from a
snapshot, so restore skips it and says so.
"""
import argparse
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import media_clients as mc  # noqa: E402

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MASK = "********"
# Provider types run a connection test on save; a snapshot restore must not
# fail because NZBGet or Plex is briefly down.
PROVIDERS = {"downloadclient", "indexer", "notification", "importlist", "metadata"}
# Live-only fields that change on their own and would show as false diffs.
VOLATILE = {"id", "freeSpace", "totalSpace", "unmappedFolders", "accessible", "presets",
            "message", "lastExecution", "lastInfoSync"}


def area(path, key, kind="list"):
    return {"path": path, "key": key, "kind": kind}


# Restore order: everything an object refers to is restored before it.
AREAS = {
    "tag": area("tag", lambda o: o["label"]),
    "customformat": area("customformat", lambda o: o["name"]),
    "qualitydefinition": area("qualitydefinition", lambda o: o["quality"]["name"]),
    "qualityprofile": area("qualityprofile", lambda o: o["name"]),
    "delayprofile": area("delayprofile", lambda o: str(o["order"])),
    # Unnamed release profiles are told apart only by their rules.
    "releaseprofile": area("releaseprofile", lambda o: o.get("name") or json.dumps(
        [o["required"], o["ignored"], o.get("indexerId"), o.get("tags")])),
    "downloadclient": area("downloadclient", lambda o: o["name"]),
    "indexer": area("indexer", lambda o: o["name"]),
    "notification": area("notification", lambda o: o["name"]),
    "metadata": area("metadata", lambda o: o["name"]),
    "importlist": area("importlist", lambda o: o["name"]),
    "rootfolder": area("rootfolder", lambda o: o["path"]),
    "remotepathmapping": area("remotepathmapping", lambda o: o["host"] + o["remotePath"]),
    "naming": area("config/naming", None, "single"),
    "mediamanagement": area("config/mediamanagement", None, "single"),
    "indexerconfig": area("config/indexer", None, "single"),
    "downloadclientconfig": area("config/downloadclient", None, "single"),
    "ui": area("config/ui", None, "single"),
}
# The two apps name this endpoint differently and key it by different ids.
EXCLUSIONS = {"radarr": area("exclusions", lambda o: str(o["tmdbId"])),
              "sonarr": area("importlistexclusion", lambda o: str(o["tvdbId"]))}


def areas(app):
    return {**AREAS, "exclusion": EXCLUSIONS[app]}


def api(app, path, **params):
    return mc.get(app, f"/api/v3/{path}", **params)


class Names:
    """id <-> name lookups for the objects other objects point at."""

    def __init__(self, app):
        self.app = app
        self.refresh()

    def refresh(self):
        self.tag = {t["id"]: t["label"] for t in api(self.app, "tag")}
        self.format = {f["id"]: f["name"] for f in api(self.app, "customformat")}
        self.profile = {p["id"]: p["name"] for p in api(self.app, "qualityprofile")}
        self.indexer = {i["id"]: i["name"] for i in api(self.app, "indexer")}

    @staticmethod
    def back(table, name, what):
        for i, n in table.items():
            if n == name:
                return i
        raise KeyError(f"no {what} named {name!r} on the live app; restore it first")


def to_names(o, n):
    """Live object -> snapshot form: no ids, references by name."""
    o = {k: v for k, v in o.items() if k not in VOLATILE}
    if "tags" in o:
        o["tags"] = sorted(n.tag[t] for t in o["tags"])
    if "formatItems" in o:
        o["formatItems"] = sorted(({"name": n.format[f["format"]], "score": f["score"]}
                                   for f in o["formatItems"]), key=lambda f: f["name"])
    if "qualityProfileId" in o:
        o["qualityProfileId"] = n.profile.get(o["qualityProfileId"], o["qualityProfileId"])
    if o.get("indexerId"):
        o["indexerId"] = n.indexer[o["indexerId"]]
    return o


def to_ids(o, n):
    """Snapshot object -> what the API accepts, against the live app's ids."""
    o = dict(o)
    if "tags" in o:
        o["tags"] = [Names.back(n.tag, t, "tag") for t in o["tags"]]
    if "formatItems" in o:
        o["formatItems"] = [{"format": Names.back(n.format, f["name"], "custom format"),
                             "name": f["name"], "score": f["score"]} for f in o["formatItems"]]
    if isinstance(o.get("qualityProfileId"), str):
        o["qualityProfileId"] = Names.back(n.profile, o["qualityProfileId"], "quality profile")
    if isinstance(o.get("indexerId"), str):
        o["indexerId"] = Names.back(n.indexer, o["indexerId"], "indexer")
    return o


def masked(o):
    return MASK in json.dumps(o)


def snapshot_dir(a):
    return a.dir or os.path.join(REPO, "arr-settings", a.app)


def live(app, name, spec, n):
    """(id, snapshot-form object) for every live object, keyed by match key."""
    got = api(app, spec["path"])
    if spec["kind"] == "single":
        return {"": (got["id"], to_names(got, n))}
    out = {}
    for o in got:
        key = spec["key"](o)
        # NOTE: two objects on one key would collapse into one and the snapshot
        # would silently lose the other, so a collision is fatal.
        if key in out:
            sys.exit(f"{name}: two live objects share the match key {key!r}")
        out[key] = (o["id"], to_names(o, n))
    return out


def cmd_snapshot(a):
    d = snapshot_dir(a)
    os.makedirs(d, exist_ok=True)
    n = Names(a.app)
    for name, spec in areas(a.app).items():
        objs = [o for _, o in live(a.app, name, spec, n).values()]
        if spec["kind"] == "list":
            objs.sort(key=spec["key"])
        else:
            objs = objs[0]
        with open(os.path.join(d, f"{name}.json"), "w") as f:
            json.dump(objs, f, indent=2, sort_keys=True)
            f.write("\n")
        print(f"{name:22} {len(objs) if isinstance(objs, list) else 'settings'}")
    print(f"wrote {d}")


def cmd_restore(a):
    d = snapshot_dir(a)
    only = set(a.only.split(",")) if a.only else None
    n = Names(a.app)
    changes = 0
    for name, spec in areas(a.app).items():
        if only and name not in only:
            continue
        path = os.path.join(d, f"{name}.json")
        if not os.path.exists(path):
            continue
        with open(path) as f:
            want = json.load(f)
        have = live(a.app, name, spec, n)
        want = {"": want} if spec["kind"] == "single" else {spec["key"](o): o for o in want}
        params = {"forceSave": "true"} if name in PROVIDERS else {}
        for key, obj in want.items():
            label = f"{name} {key}".rstrip()
            if key in have and have[key][1] == obj:
                continue
            changes += 1
            if key not in have and masked(obj):
                print(f"skip   {label}: carries a masked secret, add it in the UI")
                continue
            verb = "update" if key in have else "create"
            print(f"{verb:6} {label}")
            if not a.yes:
                continue
            body = to_ids(obj, n)
            if key in have:
                oid = have[key][0]
                mc.put(a.app, f"/api/v3/{spec['path']}/{oid}", {**body, "id": oid}, **params)
            else:
                mc.post(a.app, f"/api/v3/{spec['path']}", body, **params)
        for key in have.keys() - want.keys():
            print(f"extra  {name} {key}: live only, left alone")
        # Later areas point at what this one just created.
        if a.yes:
            n.refresh()
    if not changes:
        print("live settings match the snapshot")
    elif not a.yes:
        print("dry run: re-run with --yes to apply")


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    for cmd in ("snapshot", "diff", "restore"):
        p = sub.add_parser(cmd)
        p.add_argument("app", choices=("radarr", "sonarr"))
        p.add_argument("-d", "--dir", help="snapshot directory (default arr-settings/<app>)")
        if cmd == "restore":
            p.add_argument("--only", help="comma-separated areas, e.g. customformat,qualityprofile")
            p.add_argument("--yes", action="store_true", help="apply; without it, a dry run")
    a = ap.parse_args()
    if a.cmd == "snapshot":
        cmd_snapshot(a)
    else:
        a.only = getattr(a, "only", None)
        a.yes = getattr(a, "yes", False)
        cmd_restore(a)


if __name__ == "__main__":
    main()
