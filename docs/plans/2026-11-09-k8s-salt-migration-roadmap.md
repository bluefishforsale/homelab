# 2027 roadmap: k3s, Argo CD, Salt, and the sunset of `homelab`

> **For agentic workers:** this is a roadmap, not a single implementation plan.
> Each phase gets its own detailed plan (`docs/plans/<date>-<phase>.md`) when it
> starts, written against the spec and PRD. Do not start a phase before the
> gates it depends on are met and recorded (see "Phase dependencies"). Steps
> use checkbox (`- [ ]`) syntax for tracking.

**Spec:** `docs/specs/2026-11-09-k8s-salt-migration-design.md`
**PRD:** `docs/specs/2026-11-09-k8s-salt-migration-prd.md`

Not in this roadmap: Debian major upgrades, replacing ocean or harbor with
new VMs, and any new hardware. The cluster is ocean and harbor as they are
today (D5), and the distro lifecycle gets its own plan after this one has
proven itself in operation (D11).

## Global constraints

Every phase inherits these. They restate the PRD anti-goals and `agents.md`
hard limits so a phase plan cannot quietly drop one.

- One service per cutover, one service per PR. Never a PR that touches several
  services or a site-wide orchestrator.
- Nothing with a write-ahead log on NFS. WAL state lives on a node-local disk.
- Never a Docker container and a pod against the same state at once.
- Docker keeps working at every step (R32). On a host that still has Docker
  services, every k3s install, upgrade, Cilium change, reboot and cutover runs
  the Docker gate before and after, and stops on a failure. This ends per
  host only when its last container has moved under k3s.
- k3s install and upgrade plays are dispatch-only (never-auto-apply list in
  `.github/workflows/lib/detect-impacted-playbooks.sh`).
- No mutating ZFS verb in any committed play.
- No sshd, `authorized_keys`, fleet key or port 22 change by an agent. DNS,
  DHCP and resolver changes go only through a PR terrac merges and the CI pair
  gates (D25, R9a); never both DNS servers in one step, never by hand from an
  agent. Adding new VMs to the inventory replays the site orchestrators,
  so those PRs are merged by terrac, deliberately.
- No OS major upgrade on a VM, in place or by replacement.
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
| 2026 Q4 | Phase 0 | | `homelab-salt` repo, bootstrap script, first CI apply (D40) |
| 2027 Q1 | Phase 1 (rehearsal VM, upgrade drill, restore, k3s onto ocean, watchdog) | Phase 1b on ocean | Masterless bootstrap on every host |
| 2027 Q2 | Phase 2 (harbor joins) and Phase 3 (GPU gate) in parallel; W1 | Patch pipeline fleet-wide | Base system states |
| 2027 Q3 | Phase 4 (ocean waves W2 to W5) | | k3s install, then ocean and harbor host states, move to Salt |
| 2027 Q4 | Phase 5 (monitoring, ingress) | | Proxmox, then DNS and DHCP (gated CI, D25) |
| 2028 H1 | | | Phase 6 done, Phase 7 archive |

"OS lifecycle track" here means patching only.

Schedule risk: phase 4 is roughly forty cutovers in one quarter, about three a
week, with phase 5 stacked behind it. Nothing in this roadmap carries a hard
date, but ocean's Debian 12 LTS ends June 2028, and the distro-lifecycle plan
that follows this one has to fit in front of that. A slip here eats that
plan's room.

## Phase dependencies

Phases gate on what they need, not on their number (D37).

- Phase 1's rehearsal work needs from phase 0: the base image (for the
  rehearsal VM), the GCS bucket, the `homelab-kube` defects fixed with the
  sonarr reference chart and the Renovate and dispatch workflows, and the
  Salt bootstrap (the watchdog is a Salt state). The rest of phase 0 runs
  beside it.
- Phase 1's ocean install needs the rehearsal boxes checked and phase 0's
  full gate closed. It is the first time production is touched.
- Phase 2 and phase 3 each need phase 1. They run in parallel; neither waits
  for the other.
- W1, W2, W3 and W5 need phase 1. W4 (the GPU wave) also needs phase 3.
- Phase 5 needs phase 4.

## Phase 0: clean up and bootstrap (2026 Q4 into 2027 Q1)

The decisions this phase was waiting on are made and live in the PRD's
decision table (D1 to D50). What is left is work.

- [ ] Salt (D40): `homelab-salt` repo; the D29
      bootstrap script (pinned `.deb` from the GCS mirror, verified by hash,
      tested by CI in a clean Debian container, wrapped by one Ansible play
      for existing hosts); the rekey workflow, which reads the key from the
      host (R31); pillar decryption settled; the per-host deploy lock (R27);
      both hypervisors bootstrapped by hand; the D28 timer as the first state
      applied through CI.
- [ ] Confirm GitHub concurrency groups are per repository (two repos, one
      group name).
- [ ] ml-log-analyzer's Postgres added to `db-backup.sh` and `db-restore.sh`
      (it is live on harbor and in no backup).
- [ ] PVE 9 (D48): node005, then node006, upgraded in place, one per window,
      DNS pair pre-flight green before each. PVE 8 reached end of life in
      August 2026.
- [ ] `homelab-ca` spec (2026-10-14) amended per D39: `pathlen:1` root, cluster
      intermediate, and whether the root's name constraints admit the
      cluster's IP ranges. Lands before the CA is built.
- [ ] `homelab-kube`: PV `ocean-nfs-pvc.yaml` to `Retain`.
- [ ] `homelab-kube`: app-of-apps sync policy `allowEmpty: false`.
- [ ] `homelab-kube`: sops from GPG to age; age private key into the vault at
      a new key (single location).
- [ ] `homelab-kube`: move legacy kubeadm leftovers (flannel manifest, diag
      pods, consul, vault, kubernetes-dashboard, ingress-nginx, MetalLB,
      cloudflare-tunnel-ingress-controller) to `archive/`; drop promtail for
      Alloy.
- [ ] `homelab-kube`: write `docs/chart-style.md` from the 2021 charts (D24),
      then upgrade one chart end to end as the reference (sonarr: wrapper on
      `app-template`, new values schema, pinned image, HTTPRoute, NetworkPolicy)
      and render it in CI. It runs in phase 1 as the Gateway test app, so it does
      not sit untested until W2. The other seven are upgraded in their own wave, each
      just before its cutover, so no chart sits upgraded and untested.
- [ ] `homelab-kube`: Renovate workflow on the self-hosted runner (D21) and
      the `repository_dispatch` workflow that writes a home-grown image tag
      into one app's values (D18). CI: `helm template` plus schema validation.
- [ ] `terraform-root`: GCS location and service account for the k3s server
      backup.
- [ ] Agree the branch and merge policy for `homelab-kube` (merge is deploy
      under Argo CD auto-sync); add it to `docs/architecture/agent-autonomy.md`.
- [ ] Base image per D30: stock Debian genericcloud at the major the fleet
      runs today, pinned by sha512, mirrored to GCS, downloaded per node by
      `terraform-root` (9999 stays until nothing uses it).

Rebuild gaps that exist today, so they do not wait for 2027:

- [ ] Fix `docs/architecture/overview.md`: node005's VM disks are on
      `vmpool-storage`, not `local-lvm`.
- [ ] D52: list everything on ocean's root disk that is in none of git, GCS
      or `/data01`, and give each a home. node006's boot disk holds that
      root disk and nothing mirrors it.
- [ ] D28: config DB backup timer on both hypervisors (a Salt state), GCS
      prefix and write-only service account in `terraform-root`, freshness
      metric and alert; one restore tested onto a nested Proxmox VM.
- [ ] D32: recovery age key generated offline, added as a recipient to every
      `.sops.yaml` (`terraform-root`, `homelab-kube`), files rekeyed; first
      decrypt test recorded.

**Gate:** all Phase 0 boxes checked; PRD phase 0 acceptance. Phase 1's
rehearsal work starts on the subset in "Phase dependencies"; this full gate
closes before k3s goes onto ocean.

## Phase 1: rehearse, then k3s onto ocean (2027 Q1)

The rehearsal VM (D51) takes every risk first. Ocean is touched once, in one
window, after the play has been through three upgrades and a restore.

On the rehearsal VM:

- [ ] `vars/vars_k3s.yaml`: `k3s_version` (stable minus 3 minors at build time;
      on 2026-11-09 stable is v1.37, so v1.34), node labels, taints,
      `registries.yaml` mirrors to registry-cache-01.
- [ ] `terraform-root` PR: `k3s-rehearsal` on node005's `vmpool-storage`, the
      same Debian major as ocean, a static IP from the free infrastructure
      range, Docker installed. Then the `homelab` inventory PR, merged by
      terrac.
- [ ] `k3s.yaml` install and upgrade play: server first, then agents with
      cordon, drain, upgrade, uncordon, `serial: 1`. Added to the
      never-auto-apply list, with a pinned test in
      `detect-impacted-playbooks.test.sh`.
- [ ] Bootstrap Argo CD with Helm once from the play; from then on Argo CD
      manages itself from `homelab-kube`.
- [ ] Tier 1 platform via app-of-apps: Cilium (D15; k3s with flannel and its
      NetworkPolicy controller disabled), Hubble, cert-manager with the
      Let's Encrypt issuer, internal issuer from the cluster intermediate
      (D39), sops-secrets-operator (age), Cilium Gateway in host-network mode
      on alternate host ports (D20), a `local-path` class with
      `reclaimPolicy: Retain` as the default (stock class removed),
      metrics-server. App namespaces at Pod Security `baseline`.
- [ ] The Docker gate script (spec, "The Docker gate"; R32) written, run on
      this VM before k3s is installed, and made to fail once on purpose so a
      red result is known to stop a workflow.
- [ ] Docker plus k3s coexistence proven (spec, "Docker and k3s side by
      side"), including a Docker container reaching the Gateway's host port
      on its own host, and a reboot. The Docker gate brackets the install,
      the reboot and each of the three upgrades below. Nothing goes onto
      ocean before this is recorded.
- [ ] Three toy apps: one behind the Gateway with a cert, one with a local
      PV, one reading a sops secret.
- [ ] Each nginx feature in use today (rate limit, body size, websocket
      upgrade, IP allowlist) proven on the Gateway with a test route, or its
      fallback decided (D50).
- [ ] A Gateway listener bound on 80/443 once, to prove the Envoy capability
      the alternate ports never exercise.
- [ ] Registry fallback test: stop registry-cache-01, pull an image.
- [ ] Renovate opens, and auto-merges, one real update on a toy app; one
      home-grown test image goes push, dispatch, values commit, sync.
- [ ] Nightly server backup (SQLite `.backup`, token, `server/tls`) to GCS;
      covered by `ownership-coverage.test.sh`.
- [ ] **Upgrade 1, 2, 3** (v1.34 to v1.35 to v1.36 to v1.37, or whatever stable
      minus 3 is then). Each: check every add-on's supported Kubernetes range,
      scan for removed APIs, upgrade add-ons that must move first, bump
      `k3s_version` by PR, dispatch, record the upgrade log entry
      (`docs/operations/k3s-upgrade-log.md`).
- [ ] **Deliberate failure and restore:** break an upgrade on purpose, then
      restore the latest backup onto a freshly built rehearsal VM with the old
      IP and MAC. Write `docs/operations/k3s-restore.md` from what actually
      happened.
- [ ] Phase 1 rows of the design's "Assumptions to verify" checked and dated.

On ocean, once the boxes above are checked and phase 0's full gate is closed:

- [ ] Ocean migration table (spec, step 1): every container with class, state
      path, ports, dependants, nginx vhost features, host devices, deploying
      repo.
- [ ] Install window (spec, step 2): package holds, the NVIDIA runtime
      configured for k3s's containerd (Docker's config untouched), k3s at the version the
      drill ended on with the `homelab.role/*` labels and the ocean taint, the
      `/data01` mount guard on the k3s unit (R17). The Docker gate passes
      before the window starts, after the install, and after the reboot.
      Backout (`k3s-uninstall.sh`, restart Docker, gate again) tested on the
      rehearsal VM beforehand.
- [ ] Argo CD and the tier 1 platform on ocean from `homelab-kube`, the same
      charts the rehearsal VM ran. The Gateway on alternate host ports, the
      `/data01` local PVs.
- [ ] Three test apps on ocean: the sonarr reference chart behind the Gateway
      with a cert, one with the `/data01` PV, one reading a sops secret.
- [ ] Nightly server backup from ocean to GCS, with its freshness alert (R2).
- [ ] Alloy in the cluster: metrics by remote write to ocean's Prometheus,
      pod logs to ocean's Loki (R16); blackbox probes for the API and each
      test app.
- [ ] Cluster access (R28): terrac's kubeconfig on the laptop and agentbox,
      the agents' read-only ServiceAccount, the CI sync token.
- [ ] ntfy moved to agentbox (tier 0); it never enters the cluster (D19).
- [ ] Watchdog on agentbox: probes the k3s API, ocean and Alertmanager's
      heartbeat; sends email by a relay that bypasses ocean, and ntfy (D17).
      Tested by stopping the API and receiving both. In place before the
      soak, since the soak has to notice an outage. Alerts only past ten
      minutes (R30), so ocean's monthly window does not page.
- [ ] 14-day soak.

The rehearsal VM stays after the gate: every later upgrade runs there first
(R29).

**Gate:** PRD phase 1 acceptance.

## Phase 1b: patch pipeline (2027 Q1)

Written as Salt states from the start (D40), not in Ansible and then ported.

- [ ] `unattended-upgrades` (security origin only) on ocean, harbor and the
      rehearsal VM; package holds for kernel, NVIDIA and ZFS where they apply.
- [ ] kured on the cluster, Wednesday 03:00 to 05:00 Pacific (D38), one node
      at a time. Ocean is excluded (D22), so it starts doing real work when
      harbor joins.
- [ ] Textfile metrics: pending security updates, `reboot-required`; alerts at
      7 and 14 days.
- [ ] `docs/operations/os-patching.md`: patch policy, held-package monthly
      window with the `dkms status` pre-reboot check, Proxmox patch window.
- [ ] Roll the patch pipeline to tier 0 VMs one VM per PR. The DNS pair goes
      last and reboots one at a time behind the existing `dig` gate.

**Gate:** PRD phase 1b acceptance; the alert fires on a deliberately stale VM.

## Phase 2: harbor joins (2027 Q2)

harbor stays the VM it is. It gains a k3s agent, and its Docker services
become pods on it one at a time, through the same cutover workflow ocean's
services use (first box of phase 4, which therefore lands before the first
cutover here).

- [ ] Join window: k3s agent on harbor beside Docker, with its role label.
      The Docker gate passes before the join, after it, and after a reboot.
      Backout: `k3s-agent-uninstall.sh`, gate again.
- [ ] The `/data01` NFS PV for harbor, proven with a test pod. It is the first
      NFS mount in the cluster.
- [ ] Move the nightly DB restore test (`db_restore_test.yaml`, a Docker job
      on harbor today) to a CronJob on harbor.
- [ ] nzbget cutover: its state on a static local PV at the directory it uses
      today. The passed-through 870 EVO stays attached to harbor as it is.
- [ ] tdarr-node (CPU) cutover.
- [ ] Postgres prerequisites (D35), here and not in phase 4 because
      ml-log-analyzer is the first database to move: the `gs://homelab-pg-wal`
      bucket and write-only service account in `terraform-root`, the
      CloudNativePG operator and Barman Cloud plugin as tier 1 apps, and the
      R22 alerts (archive age, base backup age, `pg_wal` size) firing on a
      test cluster with archiving stopped.
- [ ] ml-log-analyzer cutover: Postgres by `initdb.import` into a
      CloudNativePG cluster with `enablePDB: false`, both Redis volumes on
      `local-path`. Then a kured drain of harbor, to prove the database does
      not block it.
- [ ] Docker removed from harbor after the last soak.

**Gate:** PRD phase 2 acceptance.

## Phase 3: GPU go/no-go (2027 Q2)

- [ ] NVIDIA device plugin with time-slicing via Argo CD.
- [ ] GPU gate: CUDA pod plus NVENC transcode pod while Docker llama.cpp runs;
      record the NVENC session cap.

**Gate:** PRD phase 3 acceptance. If the GPU gate fails, stop and re-plan the
GPU half before W4 (fallback: k3s `--docker` mode on ocean).

## Phase 4: ocean services, one at a time (2027 Q3)

Waves W1 to W5 from the spec. Each service is its own cutover:

- [ ] Cutover workflow (`forward` and `back`) built, with the per-service
      `migrated_to_k8s` flag that makes a play hold its Docker side down and
      its units masked; the first W1 cutover drills `back` on purpose, and an
      orchestrator replay is run in its soak to prove the container stays
      down.
- [ ] W1 stateless (app exporters, homepage, languagetool, mtr, ndt);
      startable after the phase 1 soak.
- [ ] Before W2: every ocean host unit and timer has a destination (R18).
- [ ] Before W2: service-to-service URLs held in app databases repointed to
      front-door names.
- [ ] Before the first R26 change: cross-uid hardlinks proven on the media
      tree with one downloader and one *arr (umask 002, setgid directories).
- [ ] W2 *arr stack (WAL state on local PVs); agentbox's media-refetch and
      replace-reconcile timers move with it as CronJobs (D19)
- [ ] W3 external-repo apps (each repo's deploy pattern switches in the same
      cutover; `docs/operations/deploy-pattern.md` gains the k8s pattern before
      the first one)
- [ ] W4 GPU services; plex in quiet hours; mem0 with no agent run in flight;
      agentbox's tdarr-drive and x265-watch timers move with tdarr (D19)
- [ ] W5 homeassistant, avahi, netatalk (hostNetwork); glances removed

Per cutover: chart and values PR in `homelab-kube` (scaled to 0, CronJobs
suspended) -> window, run by the cutover workflow: the flag takes the Docker
side down and masks its units, scale to 1, unsuspend, flip the nginx
upstream to the Gateway, verify probes, alert rules and panels, with the
Docker gate before and after (R32) -> 14-day
soak (D45) with the image frozen and `back` as the backout -> `homelab` PR
removes the play, files, port entry and vault keys -> `homelab-kube` PR moves
the service to its own uid and a `0700` state directory (R26).
Database services: the `db-backup` entry becomes a CronJob writing the same
GCS object name, verified with `db_restore_test.yaml`.

Postgres (D35). The bucket, the operator and the alerts are phase 2 boxes;
what is left here:

- [ ] ml-log-analyzer went first, in phase 2. Each of jellystat, archmap and
      mem0 cuts over by `initdb.import` from the
      running container; its first base backup lands before the Docker
      container is removed.
- [ ] Monthly mem0 scratch recovery scheduled and passed once.

**Gate:** only W6 and W7 services remain in Docker on ocean.

## Phase 5: monitoring and the front door (2027 Q4)

- [ ] Watchdog (built in phase 1) re-tested: stop the API, receive email and
      ntfy.
- [ ] Stop agentbox; Alertmanager still emails through ocean's exim (D19).
- [ ] W6: kube-prometheus-stack, Loki plus Alloy, Grafana, Alertmanager,
      blackbox, pushgateway, karma and agentbox's alert receiver (D19) in
      cluster; TSDB and Loki on ocean local disk. Scrape configs become
      ServiceMonitors. Old file-based scrape config retires.
- [ ] W7: the Gateway moves from its alternate ports to 80/443 on ocean;
      Docker nginx and certbot retire. A port swap: every route and cert has
      carried traffic since its own cutover. Rate limits and the allowlist
      are re-tested here, the first time the Gateway sees real client
      addresses. W7 has its own 14-day soak before Docker is removed.
      Cilium host firewall in
      audit mode, enforced after a clean week (port 22 rules: terrac approves).
- [ ] Docker removed from ocean.

## Phase 6: Salt takes the hosts (parallel, 2027 Q1 to 2028 H1)

The repo, the bootstrap script and the rekey workflow are phase 0 boxes (D40).

- [ ] `homelab-salt` repo layout complete: `.sops.yaml` per-host creation
      rules, CI apply by rsync over SSH at the merged SHA, by IP. No master
      VM (D7).
- [ ] Bootstrap rolled to every remaining host by the Ansible wrapper;
      cloud-init user-data calls it for new VMs.
- [ ] Per-host `test=True` drift timer, node-exporter textfile metric, alert
      rules (would-change over a day, failed run, checkout SHA differing from
      the runner-exported `master` SHA for 7 days). CI rsyncs the tree to
      every host on every merge.
- [ ] Test that a host cannot decrypt another host's pillar.
- [ ] Second runner on node006 (D36); each runner's states applied by the
      other.
- [ ] Port by concern, one per PR, each deleting the Ansible owner in the same
      change: base packages and users, node-exporter, docker on remaining
      Docker hosts, registry-cache, runners, agentbox. (The patch pipeline
      was born in Salt in phase 1b.)
- [ ] k3s install and upgrade (after phase 3, using the play that survived the
      drill as the reference). One install and one upgrade on the rehearsal
      VM under Salt before the real cluster.
- [ ] Ocean's and harbor's host states (NFS server, zed, NVIDIA, the metrics
      timers).
- [ ] Proxmox hosts (D34): repos, IOMMU arguments for GRUB and
      `proxmox-boot-tool`, vfio-pci binding with softdep ahead of `mpt3sas` and
      `nvidia`, `zfs-import-*` limited to host pools on node006, `storage.cfg`
      via `pvesm`, the D28 timer, NUT, exporters. Network bridges rendered,
      `test=True` in CI, applied by terrac with the iDRAC console open.
- [ ] DNS `rebuild_peer` mode (D33) built and drilled on the isolated test
      pair (a sixth drill: delete a test peer, rebuild it, survivor untouched).
- [ ] DNS and DHCP (D25), in this order:
  - [ ] Address table in pillar; CI check against `terraform-root` VM MACs.
  - [ ] CODEOWNERS and branch protection: terrac required on DNS, DHCP,
        resolver, address table and dns01/dns02/pihole pillar paths.
  - [ ] States for PowerDNS, recursor, dnsdist, zones, Kea DHCP4, DDNS and
        control agent, netboot.xyz, promtail, pihole, fleet resolvers; every
        config `file.managed` with `check_cmd`; per-daemon restarts.
  - [ ] CI apply workflow with the seven gated steps, halt and alert, no
        rollback; offline validation container pinned to host versions.
  - [ ] Isolated test pair and client in `terraform-root`; all five drills
        pass and are recorded; test VMs deleted.
  - [ ] Two weeks of `test=True` on the real pair with Salt's rendered configs
        byte-identical to Ansible's.
  - [ ] Flip PR: delete `roles/dns_infrastructure`, `dns_ha_stack.yaml`,
        `fleet_resolvers.yaml`, `pi_hole.yaml`; first Salt apply changes
        nothing on disk.

## Phase 7: archive `homelab` (2028 H1)

- [ ] Every vault key rehomed: cluster age key, host secrets to Salt pillar,
      the Cloudflare Access email lists `terraform-root` reads.
- [ ] Every deployable repo dispatches to `homelab-kube`, none to `homelab`.
- [ ] Per-chart backup CronJob and restore Job (D31) for every app with a
      node-local database; `db-backup.sh` and `db_restore.yaml` retired.
- [ ] Rebuild drill on the isolated bridge (design, "Rehearsal"): nested
      Proxmox boot disk wiped, recovered from git and GCS only, every step
      timed; result recorded; yearly repeat scheduled.
- [ ] No playbook remains that is not superseded; `main-apply` disabled.
- [ ] Docs worth keeping moved to `homelab-kube` or `homelab-salt`.
- [ ] Operational tooling rehomed or retired on the record: `scripts/`
      (`prom.sh`, `loki.sh`, `docker.sh`, `homelab-health.sh`),
      `health-check.yml`, and the repo the alert-to-issue receiver files into.
- [ ] Repo archived.
