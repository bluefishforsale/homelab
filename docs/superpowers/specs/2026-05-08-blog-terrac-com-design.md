# blog.terrac.com — Design Spec

**Date:** 2026-05-08
**Status:** Approved

## Goal

A minimal, static blog at `blog.terrac.com` powered by Hugo and PaperMod. No runtime process, no database, no CMS. Write markdown, push to git, site updates automatically.

## Architecture

```
GitHub repo: blog_terrac_com
  └── content/posts/     ← markdown posts
  └── static/            ← images and other static assets
  └── themes/PaperMod/   ← git submodule
  └── hugo.toml          ← site config (baseURL: https://blog.terrac.com)
  └── public/            ← built output, committed by CI

GitHub Actions (on push to main)
  └── runs: hugo --minify
  └── commits public/ back with [skip ci]

Ansible (homelab repo)
  └── blog_terrac_com_static.yaml
  └── rsyncs public/ → /data01/services/blog_terrac_com on ocean

Nginx (ocean, port 80)
  └── server_name blog.terrac.com
  └── root /data01/services/blog_terrac_com
  └── static file serving (same pattern as terrac.com)

Cloudflared tunnel (already added)
  └── blog.terrac.com → http://192.168.1.143:80
```

## Components

### 1. Hugo repo (`blog_terrac_com`)
- New GitHub repo under `bluefishforsale`
- PaperMod as a git submodule at `themes/PaperMod`
- `hugo.toml` with `baseURL = "https://blog.terrac.com"`, `theme = "PaperMod"`
- `public/` committed by CI, not edited manually

### 2. GitHub Actions workflow
- Trigger: push to `main`
- Steps: checkout (with submodules), install Hugo, `hugo --minify`, commit `public/` with `[skip ci]`
- Same pattern as `terrac_com_2026`

### 3. Ansible playbook (`blog_terrac_com_static.yaml`)
- Runs on `ocean`
- Ensures `/data01/services/blog_terrac_com` exists with correct permissions
- Rsyncs `public/` from the repo to that directory
- Same structure as `terrac_com_static.yaml`

### 4. Nginx server block
- Replace the WordPress proxy block added for `blog.terrac.com` in `proxy_hostname_web_proxy.conf`
- Static file serving from `/data01/services/blog_terrac_com`
- Same config as the `terrac.com` block: gzip, cache headers for assets, no hidden files

### 5. Cloudflared tunnel
- Entry already added to `vars/vars_cloudflared.yaml`: `blog.terrac.com → http://192.168.1.143:80`
- No further changes needed

## Data Flow

1. Author writes `content/posts/my-post.md` and pushes to `main`
2. GitHub Actions builds site → commits `public/` to repo
3. Ansible pulls repo, rsyncs `public/` to ocean
4. Nginx serves static files; cloudflared proxies `blog.terrac.com` through to nginx

## What's NOT included

- No CMS or web editor
- No comments system
- No analytics
- No search
- No auth

These can be added later without changing the core architecture.

## Files to create/modify

| File | Action |
|------|--------|
| `bluefishforsale/blog_terrac_com` (GitHub) | Create new repo |
| `homelab/playbooks/individual/ocean/services/blog_terrac_com_static.yaml` | Create |
| `homelab/files/nginx-compose/proxy_hostname_web_proxy.conf` | Replace `blog.terrac.com` block with static file server |
| `homelab/vars/vars_cloudflared.yaml` | Already done |
