# blog.terrac.com

A Hugo static blog with no runtime process: the external repo
`bluefishforsale/blog_terrac_com` builds `public/` and commits it, and ocean's
nginx serves those files straight out of a fresh clone. It is the canonical
**Shape B** deploy; the dispatch flow, payload rules and CI gotchas are in
[`deploy-pattern.md`](../operations/deploy-pattern.md) and are not repeated
here. This doc covers only what is specific to the blog.

## Playbooks and workflows

| File | Host | What it owns |
|---|---|---|
| `.github/workflows/deploy-blog-terrac-com.yml` | runner | `repository_dispatch: deploy-blog-terrac-com` (branch from `client_payload.branch`, default `main`) or `workflow_dispatch`. Runs the playbook with `-e git_branch=...`, then an SSH check that `repo/public/index.html` exists. `deploy-ocean` group. |
| `ocean/services/blog_terrac_com_static.yaml` | ocean | Deletes and re-clones the repo into `/data01/services/blog_terrac_com/repo` as `terrac`, warns if `public/index.html` is missing, then purges the Cloudflare edge cache for `blog.terrac.com`. |
| `ocean/network/nginx_compose.yaml` | ocean | Owns the vhost in `files/nginx-compose/proxy_hostname_web_proxy.conf` and the `:ro` mount of `/data01/services/blog_terrac_com` in `files/nginx-compose/docker-compose.yml.j2`. |
| `ocean/network/cloudflared.yaml` | ocean | Tunnel ingress from `vars/vars_cloudflared.yaml`: `blog.terrac.com` to `http://192.168.1.143:80`. First label `blog` is in `fully_public_services`, so no Access gate. |

## Serving path

```
blog.terrac.com -> Cloudflare (cache-everything, 2h edge TTL)
  -> tunnel -> 192.168.1.143:80 -> nginx server_name blog.terrac.com
  -> root /data01/services/blog_terrac_com/repo/public
```

The vhost serves files only: `try_files $uri $uri/ =404`, gzip, `nosniff` /
`SAMEORIGIN` / XSS headers, 30d `immutable` on static assets, dotfiles denied.
There is no `.home` vhost for the blog and no port in
`vars/vars_service_ports.yaml`; it rides nginx's `:80`.

## Gotchas

- Clone, not rsync. The playbook removes `repo/` and clones fresh every
  run; nginx serves `repo/public` in place. `rsync` is installed but unused.
  NOTE: between the delete and the clone finishing, the site has no files and
  serves 404s.
- Git auth is terrac's key on ocean. `/home/terrac/.ssh/id_ed25519`, also
  reused by `mem0.yaml` and `tdarr.yaml` for private clones. If the key is
  missing the play generates one, prints it for a GitHub deploy key, and fails
  on purpose until you add it and re-run.
- Edge cache. `scripts/cloudflare-harden.py` puts `blog.terrac.com` in
  `CACHE_HOSTS` with a 7200s edge TTL that overrides origin headers. The
  playbook's purge (`hosts: [blog.terrac.com]`, vault `cloudflare.api_email` /
  `api_key`, zone id inline) is the only thing that makes a deploy visible
  before 2h. It is skipped when `public/index.html` is missing.
- A stale `index.html` from the old rsync layout still sits at
  `/data01/services/blog_terrac_com/`. Nothing serves it; the health check tested
  it until #1099, which is why that check passed while looking in the wrong place.

## Monitoring

None specific. `blog.terrac.com` is not a blackbox target in
`files/ocean-prometheus/prometheus.yml.j2` (`terrac.com` and
`blog.saetnere.com` are). A broken blog shows up only as nginx or
cloudflared being down.

## Divergences from the spec and plan

- Spec and plan rsync `public/` to `/data01/services/blog_terrac_com`; code
  serves `/data01/services/blog_terrac_com/repo/public` from the clone. The
  archetype table in `deploy-pattern.md` still says "git clone, rsync
  `public/` to web_root", which is stale for this site.
- Cloudflare purge after deploy was added; the spec had none.

## Not built

- `deploy-ocean-service.yml` entry (plan Task 4). Its option list has
  `terrac_com` but not `blog_terrac_com_static`; deploy only through
  `deploy-blog-terrac-com.yml`.
- Uptime probe for `blog.terrac.com` (see Monitoring).
- The external repo (Hugo, PaperMod submodule, build workflow) lives outside
  this repo and is not verified here. CMS, comments, analytics, search and auth
  were non-goals and do not exist.

## Backout

The site is a git checkout: revert or force a commit in `blog_terrac_com` and
re-dispatch, or run the workflow with `git_branch` set to a known-good branch.
To take it offline, drop the `blog.terrac.com` ingress from
`vars/vars_cloudflared.yaml` and redeploy cloudflared. No state, no database;
deleting `/data01/services/blog_terrac_com/repo` loses nothing the repo lacks.
