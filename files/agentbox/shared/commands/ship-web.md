---
description: Deploy the current web repo to the homelab, archetype-detected (image-based or source-pull).
---

# /ship-web

Ship a web repo to the homelab. Both archetypes route through CI/dispatch — never
deploy manually on the host. Reference: `homelab/docs/operations/deploy-pattern.md`.

## 1 — Detect the archetype
- **Dockerfile present** → IMAGE-BASED (e.g. paia, my-ta-jose).
- **A built static dir (`public/`, `dist/`), no Dockerfile** → SOURCE-PULL (e.g.
  terrac.com, blog.terrac.com).

## 2a — IMAGE-BASED
1. Commit/push source to `main`.
2. `build.yml` builds the image → pushes to GHCR → `curl` POSTs a `repository_dispatch`
   to `bluefishforsale/homelab` → `deploy-<service>.yml` runs on the self-hosted runner
   (docker pull + systemd restart).
3. Watch the homelab deploy workflow to green.

## 2b — SOURCE-PULL
1. Build the static output (e.g. `hugo --minify`).
2. **Commit the built dir** (`public/` or `dist/`) — CI only triggers on changes under
   it, so an uncommitted build silently does not deploy.
3. Push to `main` → CI dispatches the deploy to homelab → `git clone` / `rsync` the
   built files to ocean under `/data01/services/<svc>/` → nginx serves them.

## 3 — Verify
- Watch the deploy workflow finish successfully (green ≠ done until it lands).
- `curl` the public URL (e.g. `https://<service>.terrac.com`) and confirm it responds.
- On failure, read the workflow log and fix forward; do not hand-edit on ocean.
