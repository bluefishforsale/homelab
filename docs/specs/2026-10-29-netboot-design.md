# Netboot: menu-driven PXE with CI-built golden images

## Goal

Boot any machine on the LAN into a menu that picks an OS, with a per-MAC default, and serve golden images that CI builds in their own repo. The first payload is throughput: the gaming tower's RTX 4090 sits idle most of the day, and tdarr is bottlenecked on GPU slots (ocean's 3090 has two). Netbooting the tower into a diskless Debian tdarr node adds a second NVENC card to the farm without touching the Windows install on its disks.

## What already exists

The front door is mostly built, which is why this is not a greenfield framework choice. Both DNS VMs (dns01 on node005, dns02 on node006) run the Kea HA pair plus `ghcr.io/netbootxyz/netbootxyz:latest` (`roles/dns_infrastructure/templates/docker-compose.yml.j2:244`), serving TFTP on 69/udp and HTTP on `service_ports.dns_stack.netboot_http` (8070). Kea already chainloads: BIOS clients get `undionly.kpxe`, UEFI clients get `ipxe.efi`, and once a client is running iPXE it gets `http://<this dns node>:8070/menu.ipxe` (`roles/dns_infrastructure/templates/kea/kea-dhcp4.conf.j2:107-129`). Each HA peer points PXE at itself, so the menu survives either node going down. What is missing is everything behind that URL: today it is netboot.xyz's stock menu, `roles/dns_infrastructure/tasks/netboot.yaml` is an empty stub, and there is no per-host logic or image of our own.

The tdarr side has prior art too. `playbooks/individual/workers/tdarr_node.yaml` runs a remote CPU tdarr node on `data01_workers`, pins the node image to the server's version via `/api/v2/status`, keeps its cache local, and steps worker limits through `/api/v2/alter-worker-limit`. The tower is the GPU, ephemeral variant of that pattern, not a new one.

## Decisions

**Framework.** iPXE scripting plus netboot.xyz, on the existing HA dns pair. Tinkerbell needs a Kubernetes cluster and ours is off (`[k8s]` is commented out in `hosts.ini`); Foreman and MAAS are whole platforms for a handful of machines; matchbox does MAC-to-profile matching natively but would replace a working HA path that twenty lines of iPXE covers.

**Control plane vs data plane.** The menu, per-MAC scripts and iPXE binaries are tiny and stay on the dns pair, keeping their HA. Image bytes do not fit there: each dns VM is 2 cores, 2 GB RAM, and has about 9 GB free on a 20 GB root, and filling the disk of the box that serves DNS and DHCP for the house is the worst failure available. Images live on a new VM, `netboot01`, on node005 (`192.168.1.33`, 2 vCPU, 2 GB RAM, 250 GB disk on `vmpool-storage`, which has about 614 GiB free). Not ocean, because ocean is the most likely future rebuild target and an image server must never be something we would PXE-rebuild; node005 also puts the data plane on the opposite node from ocean.

**Golden image.** One generic Debian 13 (trixie) live image, built with mkosi. mkosi has no squashfs output format (`Format=` accepts `directory, tar, cpio, disk, uki, esp, oci, oci-archive, sysext, confext, portable, addon, none`), so the build produces a `directory` and a post-build step runs `mksquashfs` and copies out the kernel and initrd. The initrd must be the image's own initramfs-tools build with `live-boot` in it, not mkosi's default initrd, because `live-boot` is what implements `fetch=http://.../filesystem.squashfs` and copies the root into RAM. The image carries the nvidia driver (DKMS, built in the build chroot against the image kernel), nvidia-container-toolkit, docker, node-exporter, and `netboot-role.service`. It carries no host or role specifics and no secrets.

**Role and config at boot.** The per-MAC iPXE file passes `hostname=<name> role=<role>` on the kernel command line. `netboot-role.service` fetches `http://netboot01/roles/<role>/` (a bundle homelab ansible renders: compose file, node config, worker limits), runs the role's start script, and writes `netboot_role_up` plus `netboot_image_info{sha="..."} 1` to the node-exporter textfile directory. The OS belongs to the image repo; which host does what belongs to homelab, so retargeting a machine is a homelab PR and never an image rebuild. `ansible-pull` was rejected because it puts ansible and the vault password on an ephemeral box.

**tdarr-gpu role.** Read the server version from `/api/v2/status` at boot (a version baked at image build time drifts), pull the matching `tdarr_node` image through `registry-cache-01`, mount `/data01` over NFS from ocean, mount a `tmpfs` with `size=40G` as the transcode cache, start the node with `transcodegpu: 3`. The 64 GB of RAM budgets roughly 4 GB squashfs, 3 GB OS and drivers, 3 GB image layers, 40 GB cache ceiling, and 14 GB headroom. The library has 21,746 media files, 332 over 10 GB, 110 over 20 GB, and 3 over 40 GB; tdarr in mapped mode reads the source over NFS and caches only the HEVC output, which is much smaller, so three concurrent workers fit and the rare pile-up fails one job (tdarr retries it) instead of OOM-killing the box. The tmpfs cap is what turns a full cache into a job failure rather than a crash.

**Build and transport (Shape A variant).** A new repo, `bluefishforsale/netboot-images`, builds on a GitHub-hosted `ubuntu-latest` runner (the self-hosted runners are unprivileged ephemeral containers, and mkosi wants root or user namespaces). CI pushes kernel, initrd and squashfs as separate layers of an OCI artifact with `oras push` to `ghcr.io/bluefishforsale/netboot-images/debian-live:<git-sha>`, then `repository_dispatch`es to homelab. GHCR allows 10 GB per layer with a 10 minute upload timeout, which a 3 to 5 GB squashfs fits. The repo and package are public, since the image holds no secrets. GitHub release assets were rejected on their per-file size cap; rsync from a self-hosted runner was rejected because it needs a privileged runner and an SSH key in CI.

**Promotion with a smoke gate.** On dispatch, the homelab playbook on netboot01 `oras pull`s into `/srv/netboot/images/debian-live/<sha>/`, verifies digests, and points `candidate` at it. It then delegates to node005 and `qm create`s a throwaway VM (fixed MAC with a Kea reservation at `192.168.1.34`, `boot: order=net0`, 4 GB RAM, no disk) whose per-MAC file always boots `candidate` with `role=smoke`. The playbook polls `http://192.168.1.34:9100/metrics` for `netboot_image_info{sha="<new sha>"} 1` with a 5 minute timeout, `qm destroy`s the VM whether it passed or not, and only on a pass flips `current` to the new sha. A failure halts and alerts and leaves `current` alone. The VM has no GPU, so the gate proves kernel, initrd, `fetch=`, live-boot and the role-service skeleton but not nvidia; that is most of the realistic breakage. The last three versions stay on disk.

**Menu and mode switching.** The tower's firmware boot order is PXE (UEFI IPv4) first, then the Windows disk. `menu.ipxe` first checks for a one-shot `next-boot/<mac>` flag on netboot01, then chains `hosts/${mac:hexhyp}.ipxe`, then falls back to the generic menu. The default entry on a countdown is always local disk (`exit` back to firmware, which falls through to Windows Boot Manager); Debian live tdarr is hotkey `t`; the netboot.xyz stock menu is a submenu, which also provides the interactive Debian netinst installer for free. The one-shot flag is set with `curl -X POST http://netboot01/next-boot/gaming?target=tdarr` (or `target=windows`), LAN-only, and the next boot that reads it deletes it. If netboot01 or both dns VMs are down, iPXE fails and firmware falls through to the Windows disk, so gaming never depends on this system.

**Inventory as the single source.** A new `[netboot_hosts]` group, starting with `gaming ansible_ssh_host=192.168.1.40 netboot_mac=58:11:22:ad:f4:7a netboot_role=tdarr-gpu`. The tower's NIC is an Intel I225-V, currently on a dynamic lease at `192.168.1.209` with the DDNS name `gaming.home`. From this group homelab generates the Kea reservation (today's reservations are hand-written in `kea-dhcp4.conf.j2:145`), the per-MAC iPXE file, an NFS export client entry (`nfs_export_data01.yaml:36` loops over `groups['data01_workers']` and becomes `data01_workers + netboot_hosts`), and the Prometheus scrape target. The group is deliberately not `[vms]` or `[data01_workers]`: `tdarr_node.yaml` pushes to `data01_workers` with `any_errors_fatal`, and a powered-off tower would break that deploy.

**Monitoring.** netboot01 runs node-exporter and an nginx health check like any other VM. A powered-off tower is normal and must not page, so netboot host alerts fire only on "up but broken": the node-exporter target is up and `netboot_role_up == 0` for 5 minutes, or the host is up and its tdarr node is missing from `/api/v2/get-nodes`. A failed smoke gate alerts through the playbook run.

## Changes made while building

These replaced parts of the decisions above once they met the real system, and each landed in the PR named.

The netboot hosts are not an inventory group. Inventory means `groups['all']`, so every `hosts: all` play and the fleet-wide scrape loops would chase a tower that is off most of the day, which is the same reason `[k8s]` is commented out. They live in `vars/vars_netboot.yaml` instead, and Kea, the menu, the NFS export and the scrape job all render from it (#1017).

PXE had never worked here at all. Kea named `ipxe.efi` and `undionly.kpxe`, the netbootxyz container shipped neither, and `menu.ipxe` was a 404 because nothing populated the assets dir its nginx serves. The dns role now installs both binaries from iPXE v2.0.0's release tarball, owned by uid 1000 so the container's `--tftp-secure` dnsmasq will serve them (#1036).

The live address is reserved by client-id, not MAC. A MAC reservation also handed `.40`, and with it the NFS export, to the tower's Windows install. A vendor-class pool didn't fix it either, because networkd requests the address its initramfs lease already holds and Kea grants a requested address from any pool the client may use. The image sends DUID-LL with IAID 0 (`ff:00:00:00:00:00:03:00:01:<mac>`), Windows sends `01:<mac>`, and Kea reserves by that client-id (#1043, #1051). Kea config is now validated with the running image's `kea-dhcp4 -t` before it lands.

Netboot hosts never get the `/data01` export. They get `/data01/complete` alone, `all_squash` to the media user, with `subtree_check`, because `complete` and `services` are one dataset and without it a crafted file handle reaches service state (#1043). The real boundary would be a dataset split, which is a hand-run ZFS change.

`tdarr_node` pulls from GHCR directly. `cached_registries` exists only in the registry-cache spec, not in code.

The smoke VM has 8 GB, not 4, because live-boot copies the whole squashfs into RAM. A failed role publishes the failing command as `netboot_role_failed_info` over node-exporter, since the first broken boot had no working SSH to look through.

## Goals (v1)

1. `netboot01` exists on node005 at `192.168.1.33`, serves `/srv/netboot` over nginx, and is monitored.
2. `netboot-images` builds a mkosi trixie live image on every push to its default branch and publishes it to GHCR as an OCI artifact, then dispatches to homelab.
3. Homelab pulls, verifies, smoke-boots the candidate in a throwaway node005 VM, and promotes only on a pass.
4. Every PXE client on the LAN gets the custom menu, defaulting to local disk; netboot.xyz stays reachable as a submenu.
5. The tower, picked into tdarr mode by hotkey or one-shot flag, boots diskless Debian and joins tdarr as a GPU node with three transcode workers, within a few minutes of power-on.
6. Hosts, MACs, roles and IPs live in one inventory group and nowhere else.

## Anti-goals

- No Secure Boot support. The tower has it off and its games do not need it; signing comes only when something requires it.
- No menu entry that writes a disk is ever auto-selected, by countdown, per-MAC default or flag.
- The live image never mounts local disks, NTFS or otherwise. No NTFS scratch.
- No secrets in the image or the role bundles.
- No Kubernetes, Tinkerbell, Foreman or MAAS.
- Netboot does not manage hosts after an install; installed hosts belong to ansible.
- No auto-rollback. A bad `current` is fixed by pointing the symlink back.
- No mutating applies from the laptop. Everything lands through CI.

## Later phases

v2 automates Debian install-to-disk: d-i with a preseed served from netboot01, the target disk pinned by `/dev/disk/by-id`, behind an explicit menu pick plus a confirm prompt. v3 adds Windows install: `wimboot` plus WinPE over HTTP, install media from a Samba share on netboot01 with `autounattend.xml`, and a user-supplied ISO since there is no clean scripted download. Stretch goals are a `role=inference` bundle that spans the 3090 and 4090 with llama.cpp RPC, and Wake-on-LAN from ocean to put the tower into tdarr mode overnight.

## Acceptance criteria (v1)

- A push to `netboot-images` produces a GHCR artifact with three layers whose digests match what lands in `/srv/netboot/images/debian-live/<sha>/` on netboot01.
- A deliberately broken image (for example a missing initrd) fails the smoke gate, alerts, and leaves `current` unchanged; the smoke VM is gone afterwards in both the pass and fail cases.
- A machine not in `[netboot_hosts]` that PXE boots sees the menu and lands on local disk after the countdown with no keypress.
- With dns01 stopped, the tower still reaches the menu through dns02.
- With netboot01 stopped, the tower boots Windows from disk with no keypress.
- After `POST /next-boot/gaming?target=tdarr` and a reboot, the tower appears in `/api/v2/get-nodes` as `gaming` with `transcodegpu: 3`, completes a GPU transcode, and the flag is gone; the following reboot lands on Windows.
- `lsblk` on the live tower shows no local disk partitions mounted.
- With the tower powered off, no alert fires; with the tdarr node container stopped on a running tower, an alert fires within 10 minutes.
- `make validate` passes, and `detect-impacted-playbooks.sh` maps every new `files/`, `vars/` and `roles/` input to an owner.

## Rollout order

1. Inventory group, Kea reservations for `gaming` (`.40`) and the smoke VM (`.34`), DNS records. The Kea change replays the dns stack on both HA peers, so it ships alone and is watched through `main-apply`.
2. `netboot01` VM and nginx, with monitoring.
3. `netboot-images` repo: mkosi build, squashfs, `oras push`, dispatch. Initially without nvidia, to prove the boot path.
4. Homelab pull, verify, smoke gate, promote.
5. Custom `menu.ipxe` and per-MAC files on the dns pair, default local disk.
6. nvidia and the `tdarr-gpu` role bundle, NFS export entry, alerts; first tower boot.
7. One-shot `next-boot` endpoint.

## Backout

Every step is additive. Removing the custom `menu.ipxe` restores netboot.xyz's stock menu exactly as it runs today. Setting the tower's firmware back to disk-first takes it out of the system entirely. A bad image is backed out by pointing `current` at the previous sha, and netboot01 can be destroyed without affecting DNS, DHCP or any running service.

## Open risks

- iPXE driver support for the I225-V: `ipxe.efi` uses the firmware's UEFI network driver (SNP), so this should work, but it is unverified until the first boot.
- NVENC session count on the 4090 with the current driver; three concurrent sessions is the plan, and tdarr metrics decide whether to tune it.
- `sec=sys` NFS trusts client IPs, so anything claiming `192.168.1.40` gets read-write on the media. This is the same trust already given to harbor and is accepted, not new.
