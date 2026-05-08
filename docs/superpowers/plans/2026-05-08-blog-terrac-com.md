# blog.terrac.com Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a Hugo + PaperMod static blog at blog.terrac.com, served as static files via nginx, deployed via GitHub Actions → Ansible, using the same pattern as terrac.com.

**Architecture:** A new GitHub repo `blog_terrac_com` holds markdown posts and a Hugo config with PaperMod. GitHub Actions builds `public/` on push and dispatches to homelab, which Ansible-rsyncs to `/data01/services/blog_terrac_com` on ocean. Nginx serves that directory as static files; cloudflared tunnels `blog.terrac.com` to nginx (tunnel entry already added).

**Tech Stack:** Hugo, PaperMod theme (git submodule), GitHub Actions, Ansible, nginx static file serving, cloudflared tunnel.

---

## File Map

| File | Action |
|------|--------|
| `homelab/files/nginx-compose/proxy_hostname_web_proxy.conf` | Replace WordPress proxy block for `blog.terrac.com` with static file server |
| `homelab/playbooks/individual/ocean/services/blog_terrac_com_static.yaml` | Create — Ansible deploy playbook (mirrors `terrac_com_static.yaml`) |
| `homelab/.github/workflows/deploy-blog-terrac-com.yml` | Create — homelab workflow to run Ansible on dispatch |
| `homelab/.github/workflows/deploy-ocean-service.yml` | Modify — add `blog_terrac_com_static` to service list |
| `blog_terrac_com/hugo.toml` | Create — Hugo site config |
| `blog_terrac_com/themes/PaperMod` | Create — git submodule |
| `blog_terrac_com/content/posts/hello-world.md` | Create — first post |
| `blog_terrac_com/.github/workflows/build.yml` | Create — CI: build Hugo and dispatch to homelab |

---

### Task 1: Fix nginx blog.terrac.com block

**Files:**
- Modify: `homelab/files/nginx-compose/proxy_hostname_web_proxy.conf` (lines 652–691)

The current block is an incorrect WordPress proxy. Replace it with a static file server matching the `terrac.com` pattern.

- [ ] **Step 1: Replace the WordPress proxy block**

In `homelab/files/nginx-compose/proxy_hostname_web_proxy.conf`, replace the entire `blog.terrac.com` server block (lines 652–691):

```nginx
# WordPress Blog (terrac.com)
# External access via Cloudflare - ensures HTTPS detection
server {
    listen 80;
    server_name blog.terrac.com;

    # Increase client max body size for WordPress uploads (large videos)
    client_max_body_size 2G;

    location / {
        proxy_redirect off;
        proxy_set_header Host $host;
        ...
    }
    ...
}
```

With this static file server block:

```nginx
# blog.terrac.com - Hugo static blog
server {
    listen 80;
    server_name blog.terrac.com;

    root /data01/services/blog_terrac_com;
    index index.html index.htm;

    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml application/xml+rss text/javascript image/svg+xml;
    gzip_vary on;

    location / {
        try_files $uri $uri/ =404;

        add_header X-Content-Type-Options "nosniff" always;
        add_header X-Frame-Options "SAMEORIGIN" always;
        add_header X-XSS-Protection "1; mode=block" always;
    }

    location ~* \.(jpg|jpeg|png|gif|ico|css|js|svg|woff|woff2|ttf|otf|eot)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
        access_log off;
    }

    location ~ /\. {
        deny all;
        access_log off;
        log_not_found off;
    }
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/terrac/Projects/bluefishorsale/homelab
git add files/nginx-compose/proxy_hostname_web_proxy.conf
git commit -m "fix: replace blog.terrac.com nginx block with Hugo static file server"
```

---

### Task 2: Create Ansible deploy playbook

**Files:**
- Create: `homelab/playbooks/individual/ocean/services/blog_terrac_com_static.yaml`

- [ ] **Step 1: Create the playbook**

Create `homelab/playbooks/individual/ocean/services/blog_terrac_com_static.yaml`:

```yaml
---
# Deploy blog.terrac.com Hugo static blog from GitHub
#
# Deploys pre-built static files (public/) committed by GitHub Actions CI.
# Always does a clean deployment to avoid git ownership/permissions issues.
#
# Usage:
#   ansible-playbook playbooks/individual/ocean/services/blog_terrac_com_static.yaml
#
- name: Deploy blog.terrac.com Hugo Static Blog from GitHub
  hosts: ocean
  become: true
  gather_facts: true

  vars_files:
    - "{{ playbook_dir }}/../../../../vault/secrets.yaml"

  vars:
    service: blog_terrac_com
    git_repo: "{{ lookup('env', 'GIT_REPO') | default('git@github.com:bluefishforsale/blog_terrac_com.git', true) }}"
    git_branch: "{{ lookup('env', 'GIT_BRANCH') | default('main', true) }}"
    data: /data01
    home: "{{ data }}/services/{{ service }}"
    git_clone_path: "{{ home }}/repo"
    web_root: "{{ home }}"
    user: media
    uid: 1001
    gid: 1001
    accept_hostkey: yes

  tasks:

  - name: Display deployment information
    ansible.builtin.debug:
      msg:
        - "Deploying blog.terrac.com Hugo static blog (clean deployment)"
        - "Repository: {{ git_repo }}"
        - "Branch: {{ git_branch }}"
        - "Destination: {{ web_root }}"
    tags: [always]

  - name: Ensure base directory exists
    ansible.builtin.file:
      path: "{{ home }}"
      state: directory
      owner: "{{ user }}"
      group: "{{ user }}"
      mode: '0755'
    tags: [always, setup]

  - name: Ensure repo directory exists
    ansible.builtin.file:
      path: "{{ git_clone_path }}"
      state: directory
      owner: terrac
      group: terrac
      mode: '0755'
    tags: [always, setup]

  - name: Install required packages
    ansible.builtin.apt:
      name:
        - git
        - rsync
      state: present
      update_cache: no
    tags: [setup]

  - name: Check if SSH key exists for terrac user
    ansible.builtin.stat:
      path: /home/terrac/.ssh/id_ed25519
    register: terrac_ssh_key
    tags: [setup, git]

  - name: Generate SSH key for terrac user if not exists
    ansible.builtin.user:
      name: terrac
      generate_ssh_key: true
      ssh_key_type: ed25519
      ssh_key_file: /home/terrac/.ssh/id_ed25519
      ssh_key_comment: "terrac@ocean-github"
    when: not terrac_ssh_key.stat.exists
    register: ssh_key_generated
    tags: [setup, git]

  - name: Read SSH public key
    ansible.builtin.slurp:
      src: /home/terrac/.ssh/id_ed25519.pub
    register: terrac_ssh_pub_key
    when: ssh_key_generated.changed
    tags: [setup, git]

  - name: Display SSH public key for GitHub
    ansible.builtin.debug:
      msg:
        - "⚠️  SSH key generated for terrac user!"
        - "Add this public key to GitHub deploy keys for the repository:"
        - "{{ terrac_ssh_pub_key.content | b64decode }}"
        - ""
        - "Steps:"
        - "1. Go to: https://github.com/bluefishforsale/blog_terrac_com/settings/keys"
        - "2. Click 'Add deploy key'"
        - "3. Paste the key above"
        - "4. Re-run this playbook"
    when: ssh_key_generated.changed
    tags: [setup, git]

  - name: Fail if SSH key was just generated
    ansible.builtin.fail:
      msg: "SSH key was just generated. Please add it to GitHub and re-run the playbook."
    when: ssh_key_generated.changed
    tags: [setup, git]

  - name: Always remove existing git repository for clean deployment
    ansible.builtin.file:
      path: "{{ git_clone_path }}"
      state: absent
    tags: [deploy, git]

  - name: Recreate repo directory with correct ownership
    ansible.builtin.file:
      path: "{{ git_clone_path }}"
      state: directory
      owner: terrac
      group: terrac
      mode: '0755'
    tags: [deploy, git]

  - name: Clone git repository (fresh every time)
    ansible.builtin.git:
      repo: "{{ git_repo }}"
      dest: "{{ git_clone_path }}"
      version: "{{ git_branch }}"
      force: yes
      update: yes
      accept_hostkey: "{{ accept_hostkey }}"
    become_user: terrac
    environment:
      GIT_SSH_COMMAND: "ssh -i /home/terrac/.ssh/id_ed25519 -o StrictHostKeyChecking=no"
    register: git_clone
    tags: [deploy, git]

  - name: Display git clone result
    ansible.builtin.debug:
      msg:
        - "Git operation completed"
        - "Before: {{ git_clone.before | default('N/A') }}"
        - "After: {{ git_clone.after | default('N/A') }}"
        - "Changed: {{ git_clone.changed }}"
    when: git_clone is defined
    tags: [deploy, git]

  - name: Sync built files to web root
    ansible.builtin.synchronize:
      src: "{{ git_clone_path }}/public/"
      dest: "{{ web_root }}/"
      delete: yes
      recursive: yes
      rsync_opts:
        - "--exclude=.git"
        - "--exclude=.gitignore"
        - "--exclude=.github"
        - "--exclude=README.md"
        - "--exclude=repo"
        - "--chown={{ uid }}:{{ gid }}"
    delegate_to: "{{ inventory_hostname }}"
    tags: [deploy]

  - name: Ensure all files have correct ownership
    ansible.builtin.file:
      path: "{{ web_root }}"
      owner: "{{ user }}"
      group: "{{ user }}"
      recurse: yes
    tags: [deploy]

  - name: Verify index.html exists
    ansible.builtin.stat:
      path: "{{ web_root }}/index.html"
    register: index_file
    tags: [deploy]

  - name: Display warning if index.html not found
    ansible.builtin.debug:
      msg: "WARNING: index.html not found in {{ web_root }}. The website may not load correctly."
    when: not index_file.stat.exists
    tags: [deploy]

  - name: Display deployment success
    ansible.builtin.debug:
      msg:
        - "Deployment completed successfully!"
        - "Blog root: {{ web_root }}"
        - "Files deployed from branch: {{ git_branch }}"
        - "Git commit: {{ git_clone.after | default('N/A') }}"
        - ""
        - "Access URL: https://blog.terrac.com"
    when: git_clone is defined
    tags: [always]
```

- [ ] **Step 2: Commit**

```bash
cd /Users/terrac/Projects/bluefishorsale/homelab
git add playbooks/individual/ocean/services/blog_terrac_com_static.yaml
git commit -m "feat: add Ansible deploy playbook for blog.terrac.com Hugo static blog"
```

---

### Task 3: Create homelab GitHub Actions deploy workflow

**Files:**
- Create: `homelab/.github/workflows/deploy-blog-terrac-com.yml`

- [ ] **Step 1: Create the workflow**

Create `homelab/.github/workflows/deploy-blog-terrac-com.yml`:

```yaml
---
name: Deploy blog.terrac.com

on:
  workflow_dispatch:
    inputs:
      git_branch:
        description: "Git branch to deploy"
        required: false
        default: "main"
        type: string

  repository_dispatch:
    types: [deploy-blog]

env:
  ANSIBLE_FORCE_COLOR: "true"
  ANSIBLE_HOST_KEY_CHECKING: "false"

jobs:
  deploy:
    name: Deploy blog.terrac.com Hugo Blog
    runs-on: [self-hosted, homelab, ansible]
    environment: Github Actions CI
    steps:
      - name: Checkout homelab repository
        uses: actions/checkout@v4

      - name: Install Ansible
        run: |
          python3 -m pip install --user --quiet ansible
          echo "$HOME/.local/bin" >> $GITHUB_PATH

      - name: Setup vault password
        env:
          ANSIBLE_VAULT_PASSWORD: ${{ secrets.ANSIBLE_VAULT_PASSWORD }}
        run: |
          echo "$ANSIBLE_VAULT_PASSWORD" > /tmp/.vault_pass
          chmod 600 /tmp/.vault_pass

      - name: Determine deployment parameters
        id: params
        run: |
          if [ "${{ github.event_name }}" = "repository_dispatch" ]; then
            BRANCH="${{ github.event.client_payload.branch || 'main' }}"
            COMMIT_SHA="${{ github.event.client_payload.sha || 'unknown' }}"
            COMMIT_MESSAGE="${{ github.event.client_payload.commit_message || 'Webhook deployment' }}"
          else
            BRANCH="${{ inputs.git_branch || 'main' }}"
            COMMIT_SHA="unknown"
            COMMIT_MESSAGE="Manual deployment"
          fi
          echo "branch=$BRANCH" >> $GITHUB_OUTPUT
          echo "commit_sha=$COMMIT_SHA" >> $GITHUB_OUTPUT
          echo "commit_message=$COMMIT_MESSAGE" >> $GITHUB_OUTPUT

      - name: Run deployment playbook
        env:
          ANSIBLE_VAULT_PASSWORD: ${{ secrets.ANSIBLE_VAULT_PASSWORD }}
        run: |
          export PATH="$HOME/.local/bin:$PATH"
          ansible-playbook \
            -i inventories/production/hosts.ini \
            --vault-password-file=/tmp/.vault_pass \
            -e git_branch=${{ steps.params.outputs.branch }} \
            playbooks/individual/ocean/services/blog_terrac_com_static.yaml

      - name: Cleanup vault password
        if: always()
        run: rm -f /tmp/.vault_pass

      - name: Post-deployment health check
        if: success()
        run: |
          sleep 3
          if ssh terrac@192.168.1.143 "test -f /data01/services/blog_terrac_com/index.html"; then
            echo "✅ index.html exists on server"
          else
            echo "❌ index.html NOT FOUND"
            exit 1
          fi
```

- [ ] **Step 2: Commit**

```bash
cd /Users/terrac/Projects/bluefishorsale/homelab
git add .github/workflows/deploy-blog-terrac-com.yml
git commit -m "feat: add GitHub Actions workflow to deploy blog.terrac.com"
```

---

### Task 4: Add blog_terrac_com_static to deploy-ocean-service.yml

**Files:**
- Modify: `homelab/.github/workflows/deploy-ocean-service.yml`

- [ ] **Step 1: Add service to the options list and case statement**

In `homelab/.github/workflows/deploy-ocean-service.yml`:

Find the options list (around line 35) and add after `- terrac_com_static`:
```yaml
          - terrac_com_static
          - blog_terrac_com_static
```

Find the case statement (around line 94) and add after the `terrac_com_static` line:
```bash
            terrac_com_static) PLAYBOOK="playbooks/individual/ocean/services/terrac_com_static.yaml" ;;
            blog_terrac_com_static) PLAYBOOK="playbooks/individual/ocean/services/blog_terrac_com_static.yaml" ;;
```

- [ ] **Step 2: Commit**

```bash
cd /Users/terrac/Projects/bluefishorsale/homelab
git add .github/workflows/deploy-ocean-service.yml
git commit -m "feat: add blog_terrac_com_static to deploy-ocean-service workflow"
```

---

### Task 5: Push homelab changes and deploy nginx + cloudflared

- [ ] **Step 1: Push homelab to GitHub**

```bash
cd /Users/terrac/Projects/bluefishorsale/homelab
git push origin master
```

Expected: push succeeds with 4 new commits.

- [ ] **Step 2: Deploy nginx via GitHub Actions**

Go to: `https://github.com/bluefishforsale/homelab/actions/workflows/deploy-ocean-service.yml`

Click "Run workflow", select service: `nginx`, click Run.

Expected: workflow completes successfully, nginx reloaded on ocean with new `blog.terrac.com` static file server block.

- [ ] **Step 3: Deploy cloudflared via GitHub Actions**

Go to: `https://github.com/bluefishforsale/homelab/actions/workflows/deploy-ocean-service.yml`

Click "Run workflow", select service: `cloudflared`, click Run.

Expected: cloudflared restarted with new `blog.terrac.com` tunnel ingress entry.

---

### Task 6: Create the blog_terrac_com repo locally

- [ ] **Step 1: Create the repo on GitHub**

Go to `https://github.com/new`:
- Owner: `bluefishforsale`
- Repository name: `blog_terrac_com`
- Private or public (your choice)
- Do NOT initialize with README

- [ ] **Step 2: Init the repo locally**

```bash
mkdir -p /Users/terrac/Projects/bluefishorsale/blog_terrac_com
cd /Users/terrac/Projects/bluefishorsale/blog_terrac_com
git init
git remote add origin git@github.com:bluefishforsale/blog_terrac_com.git
```

- [ ] **Step 3: Add PaperMod as a git submodule**

```bash
cd /Users/terrac/Projects/bluefishorsale/blog_terrac_com
git submodule add --depth=1 https://github.com/adityatelange/hugo-PaperMod.git themes/PaperMod
git -C themes/PaperMod checkout $(git -C themes/PaperMod describe --tags --abbrev=0)
```

Expected: `themes/PaperMod/` populated, `.gitmodules` created.

- [ ] **Step 4: Create hugo.toml**

Create `/Users/terrac/Projects/bluefishorsale/blog_terrac_com/hugo.toml`:

```toml
baseURL = "https://blog.terrac.com"
languageCode = "en-us"
title = "terrac"
theme = "PaperMod"
paginate = 10

[params]
  env = "production"
  description = ""
  author = "Terrac"
  defaultTheme = "auto"
  ShowReadingTime = true
  ShowPostNavLinks = true
  ShowBreadCrumbs = false
  ShowCodeCopyButtons = true
  ShowShareButtons = false
  ShowToc = false

[params.homeInfoParams]
  Title = "terrac"
  Content = ""

[[params.socialIcons]]
  name = "github"
  url = "https://github.com/bluefishforsale"

[menu]
  [[menu.main]]
    identifier = "home"
    name = "home"
    url = "/"
    weight = 1

[markup.highlight]
  noClasses = false
```

- [ ] **Step 5: Commit**

```bash
cd /Users/terrac/Projects/bluefishorsale/blog_terrac_com
git add hugo.toml .gitmodules themes/
git commit -m "init: Hugo site with PaperMod theme"
```

---

### Task 7: Create first post and directory structure

**Files:**
- Create: `blog_terrac_com/content/posts/hello-world.md`
- Create: `blog_terrac_com/archetypes/default.md`

- [ ] **Step 1: Create archetypes/default.md**

Create `/Users/terrac/Projects/bluefishorsale/blog_terrac_com/archetypes/default.md`:

```markdown
---
title: "{{ replace .Name "-" " " | title }}"
date: {{ .Date }}
draft: false
---
```

- [ ] **Step 2: Create first post**

Create `/Users/terrac/Projects/bluefishorsale/blog_terrac_com/content/posts/hello-world.md`:

```markdown
---
title: "Hello World"
date: 2026-05-08
draft: false
---

First post.
```

- [ ] **Step 3: Verify Hugo builds locally**

```bash
cd /Users/terrac/Projects/bluefishorsale/blog_terrac_com
# Install hugo if not present: brew install hugo
hugo version
hugo --minify
```

Expected output: something like `Start building sites … hugo v0.xxx`, `Total in Xms`, and a `public/` directory created containing `index.html`.

```bash
ls public/
```

Expected: `index.html`, `posts/`, `css/`, etc.

- [ ] **Step 4: Add public/ to .gitignore (CI will commit it, not local)**

Create `/Users/terrac/Projects/bluefishorsale/blog_terrac_com/.gitignore`:

```
public/
resources/
.hugo_build.lock
```

- [ ] **Step 5: Commit**

```bash
cd /Users/terrac/Projects/bluefishorsale/blog_terrac_com
git add archetypes/ content/ .gitignore
git commit -m "feat: add first post and archetype"
```

---

### Task 8: Create GitHub Actions build workflow

**Files:**
- Create: `blog_terrac_com/.github/workflows/build.yml`

- [ ] **Step 1: Create the workflow**

Create `/Users/terrac/Projects/bluefishorsale/blog_terrac_com/.github/workflows/build.yml`:

```yaml
name: Build

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: write

    steps:
      - name: Checkout with submodules
        uses: actions/checkout@v4
        with:
          submodules: true
          fetch-depth: 0

      - name: Setup Hugo
        uses: peaceiris/actions-hugo@v3
        with:
          hugo-version: "latest"
          extended: false

      - name: Build
        run: hugo --minify

      - name: Commit and push public/ folder
        run: |
          git config --local user.email "github-actions[bot]@users.noreply.github.com"
          git config --local user.name "github-actions[bot]"
          git add public/ -f
          git diff --staged --quiet || git commit -m "Build: update public folder [skip ci]"
          git push

      - name: Trigger homelab deployment
        env:
          HOMELAB_DISPATCH_TOKEN: ${{ secrets.HOMELAB_DISPATCH_TOKEN }}
        run: |
          curl -f -X POST \
            -H "Authorization: Bearer $HOMELAB_DISPATCH_TOKEN" \
            -H "Accept: application/vnd.github+json" \
            https://api.github.com/repos/bluefishforsale/homelab/dispatches \
            -d "{\"event_type\":\"deploy-blog\",\"client_payload\":{\"branch\":\"main\",\"sha\":\"${{ github.sha }}\",\"commit_message\":\"${{ github.event.head_commit.message }}\"}}"
```

- [ ] **Step 2: Commit**

```bash
cd /Users/terrac/Projects/bluefishorsale/blog_terrac_com
git add .github/
git commit -m "feat: add GitHub Actions build and deploy workflow"
```

---

### Task 9: Add deploy key and push to GitHub

- [ ] **Step 1: Add HOMELAB_DISPATCH_TOKEN secret to blog repo**

Go to `https://github.com/bluefishforsale/blog_terrac_com/settings/secrets/actions`.

Add secret `HOMELAB_DISPATCH_TOKEN` — use the same token value already in `terrac_com_2026` repo secrets.

- [ ] **Step 2: Push to GitHub and trigger first build**

```bash
cd /Users/terrac/Projects/bluefishorsale/blog_terrac_com
git push -u origin main
```

Expected: push succeeds, GitHub Actions build workflow starts automatically.

- [ ] **Step 3: Watch the build**

Go to `https://github.com/bluefishforsale/blog_terrac_com/actions`.

Expected: build completes, `public/` committed back, homelab `deploy-blog-terrac-com` workflow triggered and completes.

- [ ] **Step 4: Verify the site**

```bash
curl -s -o /dev/null -w "%{http_code}" https://blog.terrac.com
```

Expected: `200`

```bash
curl -s https://blog.terrac.com | grep -i "terrac\|papermod\|hello"
```

Expected: HTML containing "terrac" in the title and "Hello World" post.
