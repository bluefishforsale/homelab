# homelab

This is the Ansible repo that runs my house: two Dell servers in a rack, a handful
of VMs, one big ZFS pool, one RTX 3090, and the forty-some services that live on
top of them. It didn't start out as anything like that. It started as notes to
myself about how I'd set a machine up by hand, and somewhere along the way it
turned into a small production environment, with CI, monitoring, alerting,
backups, and a couple of AI agents that open pull requests against it.

## How it got here

The first commit is from March 2020, and it's a week of trying to get Packer to
build an Ubuntu image under QEMU. The commit messages are honest about how that
went ("still trying to get a packer build", "continued dev, still not working").
It never really worked, and the repo went quiet for most of a year.

When it came back in January 2021 it was mostly a readme and a 320-line
`install.sh`, the manual setup steps written down so I wouldn't have to remember
what I'd done to the last machine. Within a month the shell script was gone,
replaced by playbooks for users, packages and shell config. Consistency was the
whole point: a fresh box should come up the same way every time.

Then, because it was 2021, it turned into Kubernetes, and this is where the three
Dell T640s come in. Big towers with PERC controllers and room for a lot of disks,
two of them carrying NVIDIA GPUs, sitting next to three UP Squared boards that
ran the first control plane. I did Kubernetes the hard way on that pile: etcd,
kube-apiserver, haproxy and keepalived, all templated by hand, containerd, flannel
("I broke the ability to schedule flannel"), a GPU operator, 10G networking for
the Dells, and in March 2022 the Dells joined the control plane and etcd
themselves. It was a lot of machine for a home cluster, and I loved every loud,
power-hungry minute of it, including the ones where I was rebuilding it from
scratch again. One commit from that era is just titled "spaaareeee chaaaaaange".

Storage has its own arc running underneath all of that. It started as a plain NFS
playbook in August 2021, then a ceph-ansible experiment that came back out a few
months later, then Ceph again on Proxmox, ending with a crush map whose failure
domain was a single host, which tells you most of what you need to know about
running Ceph at home. In January 2025 it settled on ZFS: `data01`, raidz2 over
eight 12 TB disks, about 87 TB raw, sitting on ocean. That wasn't the end of the
story either. In August 2026 the pool went DEGRADED with two faulted disks, and
four drives got replaced in twelve days. Out of that came the replacement runbook,
pool and drive-wear dashboards, SAS error counters, and a hard rule that nothing
committed to this repo is allowed to touch the pool. Now it's exported over NFS to
worker VMs, so the NAS that started as one playbook is shared storage for more
than one host.

In 2023 the hardware got rebuilt around Proxmox, and the Kubernetes nodes became
VMs. 2024 was one more lap of the cluster with kubeadm and Kubernetes 1.29. 2025
is where it changed shape: the actual workload was a media server, not a cluster,
so the work moved to one large VM, ocean, running Docker Compose services with the
GPU passed through. Plex and the Cloudflare tunnel landed in February, local LLM
inference in May, self-hosted GitHub Actions runners in October, and in November
the `main-apply` workflow, which made merging to master a deploy. That year had
more commits than the four before it put together.

2026 is the year it picked up real production habits. PowerDNS and Kea replaced
the old DNS and DHCP. Every service got a health check and a scrape job.
agentbox arrived in June, a VM where coding agents watch the issue queue and open
PRs, so most changes now begin as an issue and land as a reviewed squash-merge.
The Kubernetes nodes were finally switched off in the inventory in May. About
1,250 commits so far, more than half of them this year.

It hasn't been a straight line. There was a DNS change that blinded everything on
ocean because seven of nine hosts only knew about one resolver, a fleet-wide uid
renumber that meant shutting ocean down and re-owning every file on `data01`, a
Plex token rotation that broke playback on the desktop apps, UPSes that would have shut
the lab down a minute into an outage, and plenty of nights where the fix for one
thing broke another. Every one of those left something behind, a runbook, an
alert, a CI check, a line in [`agents.md`](agents.md), and the lab is better for
each of them. It keeps getting better, and it's still the most fun I have with a
computer.

There's a lot of hard-won experience in this repo, mostly in the form of mistakes I've already made so you don't have to.
If you're building something similar, I hope some of it saves you an afternoon.

## What runs where

There are two Proxmox hypervisors. **node006** is a Dell R720 that hosts
**ocean**, the workhorse VM: 30 cores, about 350 GiB of RAM, the RTX 3090 through
PCI passthrough, and the `data01` pool (raidz2 over eight 12 TB disks, about 87 TB
raw). **node005** is a Dell R630 that hosts the infrastructure VMs: dns01, the GitHub
Actions runner, a registry cache, agentbox, and harbor, a second Docker host
being brought up. DNS and DHCP run as a pair split across both hypervisors, dns01
on node005 and dns02 on node006, so losing one box doesn't take name resolution
with it. Three UPSes report through NUT, and
node006 gets a shutdown signal before the batteries run out.

Most of the work happens on ocean, which runs:

- **Media**, the reason all this exists: Plex and Jellyfin, the Sonarr/Radarr/
  Prowlarr/Bazarr/Overseerr chain, nzbget, Tdarr transcoding on the GPU, Tautulli,
  and an Audible downloader for the audiobook library.
- **Monitoring**: Prometheus, Loki, Grafana, Alertmanager fanning out to ntfy,
  GitHub issues and email, and exporters
  for IPMI, ZFS, the GPU, UniFi, Proxmox, Cloudflare, the UPSes and the Emporia
  power plugs.
- **AI**: llama.cpp behind `llama.home`, Open WebUI, and mem0 for agent memory.
- **Edge**: nginx, a Cloudflare tunnel so nothing is port-forwarded, certbot, and an
  outbound mail relay.
- **Side projects**, each in its own repo, deployed here.

## The side projects

A lot of what this lab does is give my other projects somewhere real to run. Each
one lives in its own repo, builds in its own CI, and dispatches a deploy to this
repo, which owns the host, the compose file, the secrets and the monitoring.

- **etrading-boy**, a personal trading bot with backtest-validated strategies,
  served on the LAN.
- **photonic_inventory**, the lighting inventory and reservation site for Radiant
  Atmospheres. A FastAPI app behind the Cloudflare tunnel.
- **my-ta-jose**, a local-first paper grader that marks up student PDFs against a
  rubric using the llama.cpp endpoint, so nothing leaves the house.
- **paia**, a personal AI assistant on Discord.
- **terrac.com**, **blog.terrac.com** and **blog.saetnere.com**: two static sites
  and a WordPress blog.
- **The log anomaly detector**, an ML pass over the fleet's logs that flagged
  unusual patterns into Grafana, ran from November 2025 until it was retired in
  July 2026. Globalview got archived the same way in September. Projects come and go; the playbooks stay in `playbooks/archive/`.

## How a change ships

`master` is the desired state of the fleet, and merging to it is a production
deploy. A change goes branch, PR, CI (lint, syntax, and a check that every file
maps to a playbook that owns it), squash-merge, then `main-apply` runs only the
playbooks that change touches, on the self-hosted runner inside the network.
Nobody runs `ansible-playbook` from a laptop. Storage is the one exception to
merge-is-deploy: nothing committed here may change the ZFS pool, and CI enforces
that.

The details, and the traps that each cost an afternoon, are in
[`agents.md`](agents.md). The vocabulary is in [`CONTEXT.md`](CONTEXT.md).

## Layout

```text
playbooks/   00-03 site orchestrators, individual/ per-service plays, operations/
roles/       reusable roles
files/       config templates and compose files, one directory per service
vars/        shared variables, including the port registry
inventories/ hosts
vault/       ansible-vault encrypted secrets
scripts/     diagnostics: prom.sh, loki.sh, alerts.sh, vault.py, ...
docs/        architecture, operations runbooks, specs, ADRs
```

## Where to read next

- [docs/README.md](docs/README.md) is the full documentation index.
- [docs/architecture/overview.md](docs/architecture/overview.md) and
  [docs/architecture/deployment-flow.md](docs/architecture/deployment-flow.md)
  cover how the pieces fit together.
- [docs/operations/deploy-pattern.md](docs/operations/deploy-pattern.md) is how a
  new side project gets onboarded.
- [scripts/README.md](scripts/README.md) covers the diagnostic tooling.
- [docs/adr/0001-tiered-agent-autonomy.md](docs/adr/0001-tiered-agent-autonomy.md)
  is the policy for what the agents may do on their own.
