# 2027 roadmap: k3s, Argo CD, Salt, and the sunset of `homelab`

> **For agentic workers:** this is a roadmap, not a single implementation plan.
> Each phase gets its own detailed plan (`docs/plans/<date>-<phase>.md`) when it
> starts, written against the spec and PRD. Do not start a phase before the
> gates it depends on are met and recorded (see "Phase dependencies"). Steps
> use checkbox (`- [ ]`) syntax for tracking.

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
- No sshd, `authorized_keys`, fleet key or port 22 change by an agent. DNS,
  DHCP and resolver changes go only through a PR terrac merges and the CI pair
  gates (D25, R9a); never both DNS servers in one step, never by hand from an
  agent. Adding new VMs to the inventory replays the site orchestrators,
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
| 2026 Q4 | Phase 0 | | `homelab-salt` repo, bootstrap script, first CI apply (Q7) |
| 2027 Q1 | Phase 1 (build, watchdog, upgrade drill, restore) | Phase 1b on new VMs | Masterless bootstrap on every host |
| 2027 Q2 | Phase 2 (harbor) and Phase 3 (ocean joins, GPU gate) in parallel; W1 on `k3s-agent01` | Debian 13 template; patch pipeline fleet-wide | Base system states |
| 2027 Q3 | Phase 4 (ocean waves W2 to W5) | | k3s install, then ocean host states, move to Salt |
| 2027 Q4 | Phase 5 (monitoring, ingress), then 5b (ocean swap) by year end | Every VM on Debian 13 | Proxmox, then DNS and DHCP (gated CI, D25) |
| 2028 H1 | | | Phase 6 done, Phase 7 archive |

Schedule risk: phase 4 is roughly forty cutovers in one quarter, about three a
week, with phase 5 and 5b stacked behind it. The June 2028 LTS date gives six
months of slack; a slip past that is the thing to watch.

Ocean's Debian 12 LTS ends June 2028; phase 5b is the deadline-bearing item. It
runs only after Docker on ocean is drained (end of phase 5), so phases 4 and 5
must leave room for it.

## Phase dependencies

Phases gate on what they need, not on their number (D37).

- Phase 1 needs from phase 0: node006's `vmpool`, the Debian image, the GCS
  bucket, the `homelab-kube` defects fixed, and Q7 answered. The rest of
  phase 0 runs beside it.
- Phase 2 and phase 3 each need phase 1 and the coexistence proof. They run
  in parallel; neither waits for the other. Phase 3 also needs phase 0's
  full gate.
- W1 needs phase 1 and the coexistence proof, which installs Docker on
  `k3s-agent01` and reboots it, so it finishes before production pods live
  there. W2 to W5 need phase 3.
- Phase 5 needs phase 4. Phase 5b needs phase 5, phase 2's swap rehearsal,
  and ocean's host states in Salt.

## Phase 0: decide and clean up (2026 Q4 into 2027 Q1)

- [x] terrac answers Q5 (Wednesday 03:00 to 05:00), Q6 (a), Q8 (drop
      glances; avahi and netatalk to pods), Q9 (a), Q10 (no); signs off D21,
      D23, D26, D32 (2026-11-15).
- [x] terrac answers Q7 (yes, bootstrap now), Q11 (no), Q12 (per-service
      uid, `0700`), Q13 (14 days), Q14 (ml-log-analyzer is live), Q15 (two
      minors); signs off D36 (2026-11-15).
- [x] Q16 answered (agentbox is slimmed, not retired); D19 signed off
      (2026-11-15).
- [x] terrac answers Q17 (PVE 9 in place, in phase 0), Q18 (ocean's k3s
      upgrades in its monthly window), Q19 (nginx fallback decided per
      feature) (2026-11-15).
- [ ] Salt, moved up from phase 6 (Q7): `homelab-salt` repo; the D29
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
- [ ] PVE 8 reached end of life in August 2026 (Q17):
      node005, then node006, upgraded in place to PVE 9, one per window, DNS
      pair pre-flight green before each.
- [ ] `homelab-ca` spec (2026-10-14) amended per Q6: `pathlen:1` root, cluster
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
- [ ] Debian 13 base image per D30: stock genericcloud pinned by sha512,
      mirrored to GCS, downloaded per node by `terraform-root` (9999 stays
      until nothing uses it).

Rebuild gaps that exist today, so they do not wait for 2027:

- [ ] Fix `docs/architecture/overview.md`: node005's VM disks are on
      `vmpool-storage`, not `local-lvm`.
- [ ] D27, node006: two SSDs as ZFS mirror `vmpool` (terrac, by hand: free
      bays and parts checked first); confirm whether `terraform-root` moves a
      disk in place on `datastore_id`; then dns02, llmeval and ocean's root
      disk moved off `local-lvm`, one per window, `terraform-root` updated in
      the same change.
- [ ] D28: config DB backup timer on both hypervisors (a Salt state), GCS
      prefix and write-only service account in `terraform-root`, freshness
      metric and alert; one restore tested onto a nested Proxmox VM.
- [ ] D32: recovery age key generated offline, added as a recipient to every
      `.sops.yaml` (`terraform-root`, `homelab-kube`), files rekeyed; first
      decrypt test recorded.

**Gate:** all Phase 0 boxes checked; PRD phase 0 acceptance. Phase 1 starts
on the subset in "Phase dependencies"; this full gate closes before phase 3.

## Phase 1: build, then upgrade three times (2027 Q1)

Everything here is new VMs and a new cluster; nothing in production moves.

- [ ] `vars/vars_k3s.yaml`: `k3s_version` (stable minus 3 minors at build time;
      on 2026-11-09 stable is v1.37, so v1.34), node labels, taints,
      `registries.yaml` mirrors to registry-cache-01.
- [ ] `terraform-root` PR: `k3s-server01` (node006, disk on `vmpool`, never
      `local-lvm`) and `k3s-agent01` (node005), Debian 13, static IPs from the
      free infrastructure range. Proxmox startup order on node006 puts the
      server before ocean (R25).
      Then the `homelab` inventory PR, merged by terrac.
- [ ] `k3s.yaml` install and upgrade play: server first, then agents with
      cordon, drain, upgrade, uncordon, `serial: 1`. Ocean's agent is a
      separate target, run only in its monthly window (R29). Added to the
      never-auto-apply list, with a pinned test in
      `detect-impacted-playbooks.test.sh`.
- [ ] Nightly server backup (SQLite `.backup`, token, `server/tls`) to GCS;
      covered by `ownership-coverage.test.sh`.
- [ ] Bootstrap Argo CD with Helm once from the play; from then on Argo CD
      manages itself from `homelab-kube`.
- [ ] Tier 1 platform via app-of-apps: Cilium (D15; k3s with flannel and its
      NetworkPolicy controller disabled), Hubble, cert-manager with the
      Let's Encrypt issuer, internal issuer from the cluster intermediate (Q6), sops-secrets-operator (age),
      Cilium Gateway in host-network mode on alternate host ports (D20), a
      `local-path` class with `reclaimPolicy: Retain` as the default (stock
      class removed), the `/data01` NFS PV, metrics-server. App namespaces at
      Pod Security `baseline`.
- [ ] `k3s-agent01` added to ocean's NFS export client list before the NFS PV
      is used.
- [ ] Alloy in the cluster: metrics by remote write to ocean's Prometheus,
      pod logs to ocean's Loki (R16); blackbox probes for the API and each
      test app.
- [ ] Registry fallback test: stop registry-cache-01, pull an image.
- [ ] Renovate opens, and auto-merges, one real update on a test app; one
      home-grown test image goes push, dispatch, values commit, sync.
- [ ] Three test apps: one behind the Gateway with a cert (the sonarr
      reference chart), one with the NFS PV, one reading a sops secret.
- [ ] Each nginx feature in use today (rate limit, body size, websocket
      upgrade, IP allowlist) proven on the Gateway with a test route, or its
      fallback decided (Q19).
- [ ] A Gateway listener bound on 80/443 on `k3s-agent01` once, to prove the
      Envoy capability the alternate ports never exercise.
- [ ] Cluster access (R28): terrac's kubeconfig on the laptop and agentbox,
      the agents' read-only ServiceAccount, the CI sync token.
- [ ] Phase 1 rows of the design's "Assumptions to verify" checked and dated.
- [ ] ntfy moved to agentbox (tier 0); it never enters the cluster (D19).
- [ ] Watchdog on agentbox: probes the k3s API, ocean and Alertmanager's
      heartbeat; sends email by a relay that bypasses ocean, and ntfy (D17).
      Tested by stopping the API and receiving both. In place before the
      soak, since the soak has to notice an outage. Alerts only past ten
      minutes (R30), so kured's weekly server reboot does not page.
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

Written as Salt states from the start (Q7), not in Ansible and then ported.

- [ ] `unattended-upgrades` (security origin only) on the k3s VMs; package
      holds for kernel, NVIDIA and ZFS where they apply.
- [ ] kured on the cluster, Wednesday 03:00 to 05:00 Pacific (Q5), one node at a time.
- [ ] Textfile metrics: pending security updates, `reboot-required`; alerts at
      7 and 14 days.
- [ ] `docs/operations/os-lifecycle.md`: patch policy, held-package monthly
      window with the `dkms status` pre-reboot check, replacement patterns per
      VM kind, Proxmox patch window.
- [ ] Roll the patch pipeline to tier 0 VMs one VM per PR. The DNS pair goes
      last and reboots one at a time behind the existing `dig` gate.

**Gate:** PRD phase 1b acceptance; the alert fires on a deliberately stale VM.

## Phase 2: retire the harbor VM (2027 Q2)

- [x] node005 RAM upgrade installed and verified (D16): 252 GiB, read from
      Prometheus 2026-11-15.
- [ ] Prove Docker plus k3s coexistence on `k3s-agent01` (spec, "Docker and k3s
      side by side"), including a Docker container reaching the Gateway's
      host port on its own host, and a reboot. This box also gates phase 3
      and W1, so it goes first. Docker is removed from `k3s-agent01` once it
      is recorded.
- [ ] Move the nightly DB restore test (`db_restore_test.yaml`, runs on harbor
      today) to `k3s-agent01` as a CronJob before harbor goes.
- [ ] Move the passed-through 870 EVO (by WWN) and nzbget to `k3s-agent01`;
      nzbget state on its local disk.
- [ ] Move tdarr-node (CPU) to `k3s-agent01`.
- [ ] Postgres prerequisites (D35), moved up from phase 4 because
      ml-log-analyzer is the first database to move: the `gs://homelab-pg-wal`
      bucket and write-only service account in `terraform-root`, the
      CloudNativePG operator and Barman Cloud plugin as tier 1 apps, and the
      R22 alerts (archive age, base backup age, `pg_wal` size) firing on a
      test cluster with archiving stopped.
- [ ] Move ml-log-analyzer to `k3s-agent01`: Postgres by `initdb.import`
      into a CloudNativePG cluster with `enablePDB: false`, both Redis
      volumes on `local-path`. Then a kured drain of the node, to prove the
      database does not block it.
- [ ] **Rehearse the passthrough swap on `k3s-agent01`**: build a replacement
      VM, swap the scratch disk, reschedule pods, then back out and swap
      again. Fix the runbook from what happened.
- [ ] Decommission the harbor VM through the Decommission Service workflow;
      `data01_workers` group membership follows.

**Gate:** PRD phase 2 acceptance plus the swap rehearsal recorded.

## Phase 3: ocean joins, GPU go/no-go (2027 Q2)

- [ ] Ocean migration table (spec, step 1): every container with class, state
      path, ports, dependants, nginx vhost features, host devices, deploying
      repo.
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

- [ ] Cutover workflow (`forward` and `back`) built, with the per-service
      `migrated_to_k8s` flag that makes a play hold its Docker side down and
      its units masked; the first W1 cutover drills `back` on purpose, and an
      orchestrator replay is run in its soak to prove the container stays
      down.
- [ ] W1 stateless (app exporters, homepage, languagetool, mtr, ndt) on
      `k3s-agent01`; startable after the phase 1 soak.
- [ ] Before W2: every ocean host unit and timer has a destination (R18);
      k3s agent mount guard on ocean in place (R17).
- [ ] Before W2: service-to-service URLs held in app databases repointed to
      front-door names.
- [ ] Before the first R26 change: cross-uid hardlinks proven on the media
      tree with one downloader and one *arr (umask 002, setgid directories).
- [ ] W2 *arr stack (WAL state on local PVs); agentbox's media-refetch and
      replace-reconcile timers move with it as CronJobs (Q16)
- [ ] W3 external-repo apps (each repo's deploy pattern switches in the same
      cutover; `docs/operations/deploy-pattern.md` gains the k8s pattern before
      the first one)
- [ ] W4 GPU services; plex in quiet hours; mem0 with no agent run in flight;
      agentbox's tdarr-drive and x265-watch timers move with tdarr (Q16)
- [ ] W5 homeassistant, avahi, netatalk (hostNetwork); glances removed

Per cutover: chart and values PR in `homelab-kube` (scaled to 0, CronJobs
suspended) -> window, run by the cutover workflow: the flag takes the Docker
side down and masks its units, scale to 1, unsuspend, flip the nginx
upstream to the Gateway, verify probes, alert rules and panels -> 14-day
soak (Q13) with the image frozen and `back` as the backout -> `homelab` PR
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
      blackbox, pushgateway, karma and agentbox's alert receiver (Q16) in
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

## Phase 5b: ocean passthrough swap to Debian 13 (after phase 5, by end of 2027)

Starts only once Docker on ocean is drained and removed (D13). The new VM is
k3s-only.

- [ ] Build and patch the replacement ocean VM (spec, "Passthrough swap
      procedure", before the window).
- [ ] `terraform-root` swap PR and its revert PR prepared.
- [ ] Window, with terrac hand-running the ZFS steps.
- [ ] 14-day backout period, with no `zpool upgrade` in it; then delete the
      old VM.

**Gate:** PRD phase 5b acceptance.

## Phase 6: Salt takes the hosts (parallel, 2027 Q1 to 2028 H1)

The repo, the bootstrap script and the rekey workflow are phase 0 boxes (Q7).

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
      cluster under Salt before the real cluster.
- [ ] Ocean's host states (NFS server, zed, NVIDIA, the metrics timers),
      before phase 5b, so the replacement VM is built by Salt.
- [ ] Proxmox hosts (D34): repos, IOMMU arguments for GRUB and
      `proxmox-boot-tool`, vfio-pci binding with softdep ahead of `mpt3sas` and
      `nvidia`, `zfs-import-*` limited to host pools on node006, `storage.cfg`
      via `pvesm`, the D28 timer, NUT, exporters. Network bridges rendered,
      `test=True` in CI, applied by terrac with the iDRAC console open.
- [ ] D27 boot mirrors: node005 reinstalled onto a ZFS boot mirror as the
      first hardware rehearsal of the recovery order, then node006. Each
      lands on the current PVE major; the `config.db` restore across that
      jump is tried on the nested host first. node006's first boot runs with
      the shelf disconnected or `mpt3sas` blacklisted until the import guard
      state has applied (R21).
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
