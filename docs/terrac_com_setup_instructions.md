# Setting Up terrac.com for Automated Deployment

## Quick Start

The homelab deployment expects the `dist/` folder to be committed to the terrac_com_2026 repository.

### Option 1: Manual Build and Commit (Quick)

```bash
# In terrac_com_2026 repository
cd /path/to/terrac_com_2026

# Remove dist from .gitignore
sed -i '' '/^dist$/d' .gitignore
sed -i '' '/^dist-ssr$/d' .gitignore

# Build the app
npm install
npm run build

# Commit dist folder
git add dist/ .gitignore
git commit -m "build: add dist folder for deployment"
git push
```

Now you can deploy from the homelab:

```bash
# In homelab repository
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/services/terrac_com_static.yaml
```

### Option 2: Automated Build with GitHub Actions (Recommended)

1. **Copy the workflow file** from `docs/terrac_com_build_workflow.yml` to terrac_com_2026 repo at:
   ```
   .github/workflows/build-and-commit.yml
   ```

2. **Create GitHub Personal Access Token** (for automatic deployment trigger):
   - Go to GitHub Settings → Developer settings → Personal access tokens
   - Create a new token with `repo` scope
   - Add it as a secret in terrac_com_2026 repository:
     - Repository Settings → Secrets → Actions
     - Name: `HOMELAB_DEPLOY_TOKEN`
     - Value: your token

3. **Push the workflow**:
   ```bash
   git add .github/workflows/build-and-commit.yml
   git commit -m "ci: add build and commit workflow"
   git push
   ```

4. **How it works**:
   - Every push to `main` branch triggers the build
   - The workflow builds the Vite app
   - Commits the `dist/` folder back to the repo
   - Triggers the homelab deployment webhook
   - Homelab automatically pulls and deploys the new dist

## Deployment Flow

```
┌─────────────────────────────────────────────────────────────┐
│ Developer pushes to terrac_com_2026/main                    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions: Build and Commit Workflow                   │
│  1. npm install                                              │
│  2. npm run build                                            │
│  3. git commit dist/                                         │
│  4. git push                                                 │
│  5. Trigger homelab webhook                                  │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ Homelab: Deploy terrac.com Website Workflow                 │
│  1. git pull (gets new dist/ folder)                        │
│  2. rsync dist/ → /data01/services/terrac_com/              │
│  3. Set permissions                                          │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ nginx serves files from /data01/services/terrac_com/        │
│ Available at https://terrac.com                              │
└─────────────────────────────────────────────────────────────┘
```

## Testing the Setup

After committing dist/ to the repo:

```bash
# Deploy from homelab
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/services/terrac_com_static.yaml

# Verify deployment
curl -I http://192.168.1.143/
ssh terrac@192.168.1.143 "ls -la /data01/services/terrac_com/"

# Check the site
open https://terrac.com
```

## Troubleshooting

### "dist/ directory not found" error

```bash
# Check if dist exists in repo
ssh terrac@192.168.1.143 "ls -la /data01/services/terrac_com/repo/dist/"

# If missing, build and commit dist in terrac_com_2026 repo
cd /path/to/terrac_com_2026
npm run build
git add -f dist/
git commit -m "build: add dist folder"
git push

# Force clean deployment
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/services/terrac_com_static.yaml \
  -e "force_clean=true"
```

### Site not updating after push

```bash
# Check if dist was committed
cd /path/to/terrac_com_2026
git log --oneline --name-only -5 | grep dist

# Force pull on server
ssh terrac@192.168.1.143 "cd /data01/services/terrac_com/repo && git pull"

# Redeploy
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/services/terrac_com_static.yaml
```

## Manual Deployment

You can also manually trigger deployment via GitHub Actions:

1. Go to: https://github.com/bluefishforsale/homelab/actions
2. Select "Deploy terrac.com Website"
3. Click "Run workflow"
4. Choose branch and options
5. Click "Run workflow"
