# 2027 roadmap: k3s, Argo CD, Salt, and the sunset of `homelab`

> **For agentic workers:** this is a roadmap, not a single implementation plan.
> Each phase gets its own detailed plan (`docs/plans/<date>-<phase>.md`) when it
> starts, written against the spec and PRD. Do not start a phase before the
> previous phase's gate is met and recorded. Steps use checkbox (`- [ ]`)
> syntax for tracking.

**Spec:** `docs/specs/2026-11-09-k8s-salt-migration-design.md`
**PRD:** `docs/specs/2026-11-09-k8s-salt-migration-prd.md`

## Global constraints

Every phase inherits these. They restate the PRD anti-goals and `agents.md`
hard limits so a phase plan cannot quietly drop one.

- One service per cutover, one service per PR. Never a PR that touches several
  services or a site-wide orchestrator.
- Nothing with a write-ahead log on NFS. WAL state lives on a node-local disk.
- Never a Docker container and a pod against the same state at once.
- k3s install and upgrade plays are dispatch-only (never-auto-apply list in
  `.github/workflows/lib/detect-impacted-playbooks.sh`).
- No mutating ZFS verb in any committed play. Pool export and import in the
  passthrough swap are hand-run by terrac.
- No DNS, resolver, DHCP, sshd, `authorized_keys`, fleet key or port 22 change
  by an agent. Adding new VMs to the inventory replays the site orchestrators,
  so those PRs are merged by terrac, deliberately.
- No in-place OS major upgrade on a VM.
- Pods select node role labels (`homelab.role/*`), never hostnames.
- Every migrated service ships with a health check, blackbox probe, metrics
  scrape (if it has metrics), and a default-deny NetworkPolicy.
- Repos: cluster config in `homelab-kube`; the k3s play in `homelab` until
  Salt takes it; Salt states in `homelab-salt` (new). Proxmox VM definitions
  (create, resize, passthrough devices, MAC) are PRs in `terraform-root`, never
  a hand `qm` command.

## Timeline (target, not commitment)

| Quarter | Kubernetes track | OS lifecycle track | Salt track |
|---|---|---|---|
| 2026 Q4 | Phase 0 | | |
| 2027 Q1 | Phase 1 (build, upgrade drill, restore) | Phase 1b on new VMs | Master and minions |
| 2027 Q2 | Phase 2 (harbor), Phase 3 (ocean joins, GPU gate) | Debian 13 template; patch pipeline fleet-wide | Base system states |
| 2027 Q3 | Phase 4 (ocean waves W1 to W5) | | k3s install moves to Salt |
| 2027 Q4 | Phase 5 (monitoring, ingress), then 5b (ocean swap) by year end | Every VM on Debian 13 | Proxmox, then DNS (terrac) |
| 2028 H1 | | | Phase 6 done, Phase 7 archive |

Schedule risk: phase 4 is roughly forty cutovers in one quarter, about three a
week, with phase 5 and 5b stacked behind it. The June 2028 LTS date gives six
months of slack; a slip past that is the thing to watch.

Ocean's Debian 12 LTS ends June 2028; phase 5b is the deadline-bearing item. It
runs only after Docker on ocean is drained (end of phase 5), so phases 4 and 5
must leave room for it.

## Phase 0: decide and clean up (2026 Q4)

- [ ] terrac reviews spec and PRD; answers Q4 (old charts), Q5 (reboot
      window), Q6 (internal CA), Q8 (stray host daemons); signs off D19, D21,
      D23. Q7 (is Salt still worth it) answered before phase 6 work starts.
- [ ] `homelab-kube`: PV `ocean-nfs-pvc.yaml` to `Retain`.
- [ ] `homelab-kube`: app-of-apps sync policy `allowEmpty: false`.
- [ ] `homelab-kube`: sops from GPG to age; age private key into the vault at
      a new key (single location).
- [ ] `homelab-kube`: move legacy kubeadm leftovers (flannel manifest, diag
      pods, consul, vault, kubernetes-dashboard, ingress-nginx, MetalLB,
      cloudflare-tunnel-ingress-controller) to `archive/`; drop promtail for
      Alloy.
- [ ] `homelab-kube`: Renovate workflow on the self-hosted runner (D21) and
      the `repository_dispatch` workflow that writes a home-grown image tag
      into one app's values (D18). CI: `helm template` plus schema validation.
- [ ] `terraform-root`: GCS location and service account for the k3s server
      backup.
- [ ] Agree the branch and merge policy for `homelab-kube` (merge is deploy
      under Argo CD auto-sync); add it to `docs/architecture/agent-autonomy.md`.
- [ ] Debian 13 base image for new VMs on both hypervisors, wherever
      `terraform-root` sources it (9999 stays until nothing uses it).

**Gate:** all Phase 0 boxes checked; PRD phase 0 acceptance.

## Phase 1: build, then upgrade three times (2027 Q1)

Everything here is new VMs and a new cluster; nothing in production moves.

- [ ] `vars/vars_k3s.yaml`: `k3s_version` (stable minus 3 minors at build time;
      on 2026-11-09 stable is v1.37, so v1.34), node labels, taints,
      `registries.yaml` mirrors to registry-cache-01.
- [ ] `terraform-root` PR: `k3s-server01` (node006) and `k3s-agent01`
      (node005), Debian 13, static IPs from the free infrastructure range.
      Then the `homelab` inventory PR, merged by terrac.
- [ ] `k3s.yaml` install and upgrade play: server first, then agents with
      cordon, drain, upgrade, uncordon, `serial: 1`. Added to the
      never-auto-apply list, with a pinned test in
      `detect-impacted-playbooks.test.sh`.
- [ ] Nightly server backup (SQLite `.backup`, token, `server/tls`) to GCS;
      covered by `ownership-coverage.test.sh`.
- [ ] Bootstrap Argo CD with Helm once from the play; from then on Argo CD
      manages itself from `homelab-kube`.
- [ ] Tier 1 platform via app-of-apps: Cilium (D15; k3s with flannel and its
      NetworkPolicy controller disabled), Hubble, cert-manager with the
      Let's Encrypt issuer (internal CA per Q6), sops-secrets-operator (age),
      Cilium Gateway (D20), local-path, the `/data01` NFS PV, metrics-server.
- [ ] `k3s-agent01` added to ocean's NFS export client list before the NFS PV
      is used.
- [ ] Alloy in the cluster: metrics by remote write to ocean's Prometheus,
      pod logs to ocean's Loki (R16); blackbox probes for the API and each
      test app.
- [ ] Registry fallback test: stop registry-cache-01, pull an image.
- [ ] Renovate opens, and auto-merges, one real update on a test app; one
      home-grown test image goes push, dispatch, values commit, sync.
- [ ] Three test apps: one behind the Gateway with a cert, one with the NFS PV,
      one reading a sops secret.
- [ ] **Upgrade 1, 2, 3** (v1.34 to v1.35 to v1.36 to v1.37, or whatever stable
      minus 3 is then). Each: check every add-on's supported Kubernetes range,
      scan for removed APIs, upgrade add-ons that must move first, bump
      `k3s_version` by PR, dispatch, record the upgrade log entry
      (`docs/operations/k3s-upgrade-log.md`).
- [ ] **Deliberate failure and restore:** break an upgrade on purpose, then
      restore the latest backup onto a freshly built server VM with the old IP
      and MAC. Write `docs/operations/k3s-restore.md` from what actually
      happened.
- [ ] Throwaway rehearsal cluster: the same plays pointed at a disposable
      inventory, so future upgrades run there first.
- [ ] 14-day soak.

**Gate:** PRD phase 1 acceptance.

## Phase 1b: patch pipeline (2027 Q1, new VMs first)

- [ ] `unattended-upgrades` (security origin only) on the k3s VMs; package
      holds for kernel, NVIDIA and ZFS where they apply.
- [ ] kured on the cluster, window per Q5, one node at a time.
- [ ] Textfile metrics: pending security updates, `reboot-required`; alerts at
      7 and 14 days.
- [ ] `docs/operations/os-lifecycle.md`: patch policy, held-package monthly
      window with the `dkms status` pre-reboot check, replacement patterns per
      VM kind, Proxmox patch window.
- [ ] Roll the patch pipeline to tier 0 VMs one VM per PR (DNS pair excluded,
      terrac's).

**Gate:** PRD phase 1b acceptance; the alert fires on a deliberately stale VM.

## Phase 2: retire the harbor VM (2027 Q2)

- [ ] node005 RAM upgrade installed and verified (D16). Phase 2 does not
      start without it.
- [ ] Prove Docker plus k3s coexistence on `k3s-agent01` (spec, "Docker and k3s
      side by side"), including a Docker container reaching a NodePort on its
      own host, and a reboot.
- [ ] Move the nightly DB restore test (`db_restore_test.yaml`, runs on harbor
      today) to `k3s-agent01` before harbor goes.
- [ ] Move the passed-through 870 EVO (by WWN) and nzbget to `k3s-agent01`;
      nzbget state on its local disk.
- [ ] Move tdarr-node (CPU) to `k3s-agent01`.
- [ ] **Rehearse the passthrough swap on `k3s-agent01`**: build a replacement
      VM, swap the scratch disk, reschedule pods, then back out and swap
      again. Fix the runbook from what happened.
- [ ] Decommission the harbor VM through the Decommission Service workflow;
      `data01_workers` group membership follows.

**Gate:** PRD phase 2 acceptance plus the swap rehearsal recorded.

## Phase 3: ocean joins, GPU go/no-go (2027 Q2)

- [ ] Ocean migration table (spec, step 1): every container with class, state
      path, ports, dependants, deploying repo.
- [ ] Join window (spec, step 2): holds, NVIDIA runtime for k3s containerd,
      k3s agent with `homelab.role/*` labels and the ocean taint, verify, reboot,
      verify. Backout tested on paper beforehand.
- [ ] NVIDIA device plugin with time-slicing via Argo CD.
- [ ] GPU gate: CUDA pod plus NVENC transcode pod while Docker llama.cpp runs;
      record the NVENC session cap.

**Gate:** PRD phase 3 acceptance. If the GPU gate fails, stop and re-plan the
GPU half before phase 4 (fallback: k3s `--docker` mode on ocean).

## Phase 4: ocean services, one at a time (2027 Q3)

Waves W1 to W5 from the spec. Each service is its own cutover:

- [ ] Before W1: every ocean host unit and timer has a destination (R18);
      k3s agent mount guard on ocean in place (R17).
- [ ] W1 stateless (app exporters, homepage, languagetool, karma, ...)
- [ ] W2 *arr stack (WAL state via `hostPath`)
- [ ] W3 external-repo apps (each repo's deploy pattern switches in the same
      cutover; `docs/operations/deploy-pattern.md` gains the k8s pattern before
      the first one)
- [ ] W4 GPU services; plex in quiet hours; mem0 with no agent run in flight
- [ ] W5 homeassistant (hostNetwork)

Per cutover: chart and values PR in `homelab-kube` (scaled to 0) -> window:
retire play stops the container, scale to 1, flip the nginx upstream, verify
probes -> `homelab` PR removes the play, files, port entry and vault keys.
Database services: the `db-backup` entry becomes a CronJob writing the same
GCS object name, verified with `db_restore_test.yaml`.

**Gate:** only W6 and W7 services remain in Docker on ocean.

## Phase 5: monitoring and the front door (2027 Q4)

- [ ] ntfy moved to agentbox (tier 0); it never enters the cluster (D19).
- [ ] Watchdog on agentbox: probes the k3s API, ocean and Alertmanager's
      heartbeat; sends email by a relay that bypasses ocean, and ntfy (D17).
      Tested by stopping the API and receiving both.
- [ ] W6: kube-prometheus-stack, Loki plus Alloy, Grafana, Alertmanager in
      cluster; TSDB and Loki on ocean local disk. Scrape configs become
      ServiceMonitors. Old file-based scrape config retires.
- [ ] W7: Cilium Gateway on ocean in host-network mode on 80/443; certbot
      retires to cert-manager; Docker nginx retires. Cilium host firewall in
      audit mode, enforced after a clean week (port 22 rules: terrac approves).
- [ ] Docker removed from ocean.

## Phase 5b: ocean passthrough swap to Debian 13 (after phase 5, by end of 2027)

Starts only once Docker on ocean is drained and removed (D13). The new VM is
k3s-only.

- [ ] Build and patch the replacement ocean VM (spec, "Passthrough swap
      procedure", before the window).
- [ ] `terraform-root` swap PR and its revert PR prepared.
- [ ] Window, with terrac hand-running the ZFS steps.
- [ ] 14-day backout period; then delete the old VM.

**Gate:** PRD phase 5b acceptance.

## Phase 6: Salt takes the hosts (parallel, 2027 Q1 to 2028 H1)

- [ ] `salt01` VM on node005 (tier 0); `homelab-salt` repo with CI apply and a
      scheduled `test=True` drift report.
- [ ] Minions on every Linux host.
- [ ] Port by concern, one per PR, each deleting the Ansible owner in the same
      change: base packages and users, node-exporter, unattended-upgrades and
      the patch metrics, docker on remaining Docker hosts, registry-cache,
      runners, agentbox.
- [ ] k3s install and upgrade (after phase 3, using the play that survived the
      drill as the reference).
- [ ] Proxmox hosts.
- [ ] DNS pair: terrac, by hand.

## Phase 7: archive `homelab` (2028 H1)

- [ ] Every vault key rehomed: cluster age key, host secrets to Salt pillar,
      the Cloudflare Access email lists `terraform-root` reads.
- [ ] Every deployable repo dispatches to `homelab-kube`, none to `homelab`.
- [ ] No playbook remains that is not superseded; `main-apply` disabled.
- [ ] Docs worth keeping moved to `homelab-kube` or `homelab-salt`.
- [ ] Repo archived.
