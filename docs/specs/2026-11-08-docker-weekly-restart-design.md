# Weekly cache-gated restarts for every docker service

Status: draft, awaiting sign-off. No code until this is approved.

## Goal

Every docker service in the fleet runs as a compose project, and every compose
project restarts exactly once a week, in its own committed time slot, after its
image has been pulled through registry-cache-01 and its digest confirmed against
the cache. Restarts happen on weekday mornings between 02:00 and 08:00 Pacific,
when a break gets found and fixed by 8am, never on a weekend when I'm out.
Predictability beats velocity here: a slot that is missed is skipped, not caught
up.

## Why

Three restart paths exist today and none of them is predictable.
`docker-refresh@<project>` fires Sunday 08:00 with `RandomizedDelaySec=6h` and
`Persistent=true` (dns02 on Wednesday), and only recreates when the image
changed. `docker-restart@` fires Saturday 03:00 for host-built images. The 16
legacy `docker run` units restart on `RuntimeMaxSec=7d`, which herds them into
whatever minute they last started and is how bazarr ended up stuck in
`start-pre operation timed out` (its `ExecStartPre` does `docker rmi` and a
pull inside a 60s `TimeoutStartSec`). Those legacy units also never get a new
image from the refresh timer, because they are not compose projects.

Only Docker Hub pulls go through registry-cache-01 today, via the dockerd
`registry-mirrors` entry in `files/docker/daemon.json.j2`. The ghcr, gitlab and
nvcr caches exist and nothing points at them, and the dns role overwrites
`daemon.json` so dns01 and dns02 skip the mirror entirely.

## What exists

- `files/node-exporter/docker-refresh.sh`, `docker-refresh@.service`,
  `docker-refresh@.timer.j2`, `docker-restart@.service`,
  `docker-restart@.timer.j2`, enabled per project from live
  `docker compose ls -a` in `playbooks/individual/infrastructure/node_exporter.yaml`.
  The script already handles `--ignore-buildable`, health wait, and refuses a
  compose file with an unset variable.
- `docker_refresh_exclude: [github-runners]`, with the 2026-09-06 token outage
  NOTE.
- registry-cache-01 (192.168.1.30): `registry:2` proxies for docker-hub 5001,
  ghcr 5002, gitlab 5003, nvcr 5004, documented in
  `docs/architecture/registry-cache.md`. ghcr-cache has no upstream auth.
- Five playbooks log in to ghcr for private images: `etrading_boy.yaml`,
  `photonic_inventory.yaml`, `my_ta_jose.yaml`, `paia.yaml`, `mem0.yaml`.
- dockerd reloads `insecure-registries` and `registry-mirrors` on SIGHUP
  (docker/cli `docs/reference/dockerd.md`), and the docker unit on the fleet has
  `ExecReload=/bin/kill -s HUP $MAINPID`, so registry changes need no dockerd
  restart and bounce no containers.

## Not on compose today

- process-exporter on all 11 hosts.
- ocean: bazarr, prowlarr, prowlarr-exporter, radarr, radarr-exporter, sonarr,
  sonarr-exporter, blackbox-exporter, mtr-exporter, tdarr, tdarr-node, overseerr.
- node005: ipmi-exporter, pve-exporter. node006: ipmi-exporter.

## Decisions

1. **Everything is compose.** Each legacy unit above becomes a compose project
   with the same data dirs, ports and image. The `RuntimeMaxSec` units are
   deleted.
2. **One unit, `docker-weekly@<project>`**, replaces `docker-refresh@`,
   `docker-restart@` and `RuntimeMaxSec`. Deploy-time pulls from CI dispatch are
   unchanged.
3. **Always restart weekly**, even when the image is unchanged, once the pull is
   confirmed.
4. **All registries go through registry-cache-01.** Every image ref is rewritten
   to `registry-cache.home:<port>/...`:
   - docker.io via docker-hub-cache 5001
   - ghcr.io and lscr.io via ghcr-cache 5002 (lscr fronts ghcr)
   - registry.gitlab.com via gitlab-cache 5003
   - nvcr.io via nvcr-cache 5004
   - private `ghcr.io/bluefishforsale/*` via a new `ghcr-private-cache` 5005,
     upstream auth from a read-only `read:packages` token in the vault, htpasswd
     in front, hosts log in to it the way they log in to ghcr today
   - gcr.io (cadvisor only) via a new `gcr-cache` 5006

   registry-cache-01 itself pulls straight from upstream (it sets
   `registry_cache_mirror: false` today and keeps doing so).
5. **The cache is a hard dependency.** No fallback to upstream from clients.
6. **`daemon.json` gains `insecure-registries`** for `registry-cache.home:5001-5006`,
   applied with `reload docker`, not restart. The dns role renders the same
   template instead of writing its own.
7. **The run sequence for one slot**, in order, stopping at the first gate that
   says no:
   1. Halt gate: query ocean Prometheus for
      `max(docker_weekly_last_result{result="unhealthy"}) > 0`. A match, or
      Prometheus unreachable, means skip with `result="halted"`.
   2. Internet gate: probe `https://registry-1.docker.io/v2/`. Unreachable means
      skip with `result="no_internet"`.
   3. Pull: `docker compose pull` (skipped for services with `build:`). Failure
      means skip with `result="pull_failed"`; the old container keeps running.
   4. Digest check: HEAD `/v2/<repo>/manifests/<tag>` on the cache and compare
      with the pulled image's repo digest. Mismatch means skip with
      `result="digest_mismatch"`. Skipped for `build:` services.
   5. Restart: `docker compose up -d --force-recreate`.
   6. Health wait (the existing 120 to 600s logic). Unhealthy means
      `result="unhealthy"`, which halts every remaining slot fleet-wide.
      Otherwise `result="ok"`.

   Every outcome writes `docker_weekly_last_result{host,project,result}` and a
   timestamp to the node-exporter textfile dir. No outcome fails the systemd
   unit; the metric and alerts are the signal. Nothing rolls back.
8. **The halt is cleared by a human only.** Fix the project, then dispatch the new
   `docker-weekly-run` workflow for that host and project. That CI-mediated run
   overwrites the metric with `ok` and the halt lifts. A week rolling over does
   not clear it. Only `unhealthy` halts; `pull_failed` and `digest_mismatch`
   alert without halting, because the old container is still running untouched.
9. **Slots.** A committed file, `vars/vars_restart_schedule.yaml`, maps every
   `host/project` to a slot. Slots are 20 minutes apart fleet-wide, one global
   list, on Tue, Wed and Thu between 02:00 and 08:00 America/Los_Angeles
   (18 slots a morning, 54 a week).
   - Each host's generic exporters (cadvisor, node-exporter, fail2ban-exporter,
     smart-exporter, process-exporter, and on ocean the other exporters) share
     one slot per host and restart one after another inside it. That turns
     about 55 exporter projects into 11 slots, plus about 44 for real services.
   - Wednesday carries the risky ones: registry-cache-01 at 02:00 first, then
     dns01, the monitoring stack, Plex, Jellyfin, llamacpp.
   - dns02 is on Thursday, so dns01 and dns02 never restart on the same day.
   - Tuesday and Thursday carry the exporter slots and quieter services.
   - New projects take the next free slot.
   - Timers use `Persistent=false` and no `RandomizedDelaySec`: a missed slot is
     skipped, never caught up.
10. **CI guards the schedule.** A test in `ci-validate.yml` fails when two slots
    collide, when two slots sit less than 20 minutes apart, when a slot falls
    outside Tue/Wed/Thu 02:00 to 08:00, when dns01 and dns02 share a day, or
    when a compose project known to the repo has no slot.
11. **A running project with no slot** is reported by a metric and fires an
    alert. It does not get a timer until it has a slot.
12. **netboot01 gets timers** like every other host.

## Anti-goals

- No restart of `github-runners`. It stays excluded, for the token reason in the
  existing NOTE.
- No catch-up after a missed slot, and no randomised delay.
- No automatic rollback, ever.
- No fallback to pulling from upstream when the cache is down.
- No dockerd restarts to apply registry config.
- No weekend restarts.
- No change to deploy-time pulls or the CI dispatch deploy paths.
- No credentials on the anonymous ghcr-cache.
- No ZFS, storage or NFS changes; converted services keep their state dirs
  exactly where they are.

## Rollout

Five PRs in order. Each merges, deploys through `main-apply` as normal, and
passes `scripts/homelab-health.sh verify` before the next goes up. Nothing merges
undeployed.

1. Cache side: `gcr-cache` 5006, `ghcr-private-cache` 5005, vault token,
   htpasswd, scrape targets, and a docs update to
   `docs/architecture/registry-cache.md`. Clients are untouched.
2. `daemon.json` `insecure-registries` with a reload handler, and the dns role
   switched to the shared template.
3. Legacy to compose, one PR per group: bazarr first (it is broken now), then
   the other *arr apps and their exporters, tdarr, overseerr,
   blackbox-exporter and mtr-exporter, then process-exporter fleet-wide, then
   the node005 and node006 exporters.
4. Image refs rewritten to the cache, with the ghcr logins pointed at
   `ghcr-private-cache`.
5. `docker-weekly@`, the slot file, the CI test, metrics, alerts and the halt
   gate, retiring `docker-refresh@`, `docker-restart@` and their timers in the
   same PR.

Any PR that recreates `dns-stack` on dns01 or dns02 merges only on a weekday
morning, and dns01 and dns02 go in separate PRs.

Backout for each step is reverting its PR and letting `main-apply` redeploy. Two
steps need more than a revert: step 5 must re-enable the old timers it removed
(the revert restores the files, and the play must enable them), and step 2's
reload leaves `insecure-registries` harmless if a revert drops it.

## Acceptance criteria

- `docker ps -a` on every host shows no container without a
  `com.docker.compose.project` label.
- No `RuntimeMaxSec` unit, `docker-refresh@` timer or `docker-restart@` timer
  exists anywhere in the fleet.
- Every running compose project except `github-runners` has exactly one
  `docker-weekly@` timer, and its `OnCalendar` matches the slot file.
- The CI schedule test passes on master and fails on a fixture with a collision,
  a 19-minute gap, a Saturday slot, dns01 and dns02 on one day, and a missing
  project.
- Every image ref in `files/` and `playbooks/` starts with `registry-cache.home:`.
- Pushing a new `:latest` for one Shape A project, then pulling through
  `ghcr-private-cache`, returns the new digest (the proxy rechecks tags upstream).
- Anonymous pulls of a private image from `ghcr-private-cache` return 401.
- With the WAN blocked on one host, a slot records `result="no_internet"` and
  the container is not recreated.
- With `registry-cache-01` stopped, a slot records `result="pull_failed"`, an
  alert fires, and the old container keeps running.
- A project forced unhealthy records `result="unhealthy"`, the next slot on
  another host records `result="halted"`, and a `docker-weekly-run` dispatch
  clears it.
- Applying the `daemon.json` change leaves every container's `StartedAt`
  unchanged.
- bazarr runs as a compose project and survives a weekly restart.
