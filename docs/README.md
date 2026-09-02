# Homelab Documentation

Documentation for the homelab infrastructure.

## Addresses in these docs are placeholders

Every address below is in `192.0.2.0/24`, the range RFC 5737 reserves for
documentation, and every hostname is under `example.com`, reserved by RFC 2606.
Neither routes anywhere. They are here so the shape of the network is legible:
which host does what, what talks to what, which service sits behind which name.

If you are cloning this repo to run it yourself, the addresses are yours to
supply. Start with `inventories/production/hosts.ini` and the `group_vars`
beside it. Nothing will deploy until those describe real machines, which is
deliberate: the interesting part of this repo is the patterns, not the numbers.

The last-octet mapping is kept stable across every document, so `192.0.2.143`
is the same host wherever it appears.

---

## Quick Reference

| Host | IP | Purpose |
|------|----|---------|  
| node005 | 192.0.2.105 | Proxmox (56 cores, 128GB) |
| node006 | 192.0.2.106 | Proxmox (40 cores, 680GB, RTX 3090) |
| ocean | 192.0.2.143 | Docker services VM |
| dns01 | 192.0.2.2 | BIND9 DNS |
| pihole | 192.0.2.9 | DNS filtering |
| gitlab | 192.0.2.5 | GitLab CI/CD |

---

## Getting Started

- [Getting Started Guide](setup/getting-started.md) - Complete deployment guide
- [macOS Setup](setup/macos-setup.md) - Local development environment
- [DEVELOPMENT.md](/DEVELOPMENT.md) - Developer setup

---

## Architecture

- [System Overview](architecture/overview.md) - High-level architecture
- [Network Design](architecture/networking.md) - Network topology
- [Ocean Services](architecture/ocean-services.md) - Service architecture
- [Deployment Flow](architecture/deployment-flow.md) - CI/CD pipeline

---

## Operations

- [Proxmox](operations/proxmox.md) - VM management, clustering
- [ZFS Storage](operations/zfs.md) - Storage operations, snapshots
- [ZFS Disk Replacement](operations/zfs-disk-replacement.md) - Disk failure recovery
- [GPU Management](operations/gpu-management.md) - NVIDIA RTX 3090 configuration
- [Dell Hardware](operations/dell-hardware.md) - iDRAC, RAID, firmware
- [UniFi Network](operations/unifi.md) - Switch and AP configuration
- [Database Restore](operations/db-restore.md) - Per-service runbook, dry-run by default, point-in-time from GCS generations

---

## CI/CD

- [Deploy Pattern](operations/deploy-pattern.md) - READ FIRST when onboarding a new external-repo service
- [Deploy Tracer](operations/deploy-tracer.md) - Validating the deploy chain end to end
- [scripts/README.md](/scripts/README.md) - Diagnostic and admin tooling
- [.github/workflows/](/.github/workflows/) - Workflow definitions

---

## Troubleshooting

- [Common Issues](troubleshooting/common-issues.md) - Frequently encountered problems
- [Internal DNS flapping / hostname collisions](troubleshooting/common-issues.md#internal-dns-intermittent--flapping-home-answers) - check with [`scripts/dns-drift-check.sh`](/scripts/dns-drift-check.sh)

## Quick Links by Topic

### Infrastructure

| Topic | Docs |
|-------|------|
| Proxmox | [Operations](operations/proxmox.md) |
| ZFS | [Operations](operations/zfs.md) \| [Disk Replacement](operations/zfs-disk-replacement.md) |
| GPU | [Management](operations/gpu-management.md) |
| Dell Hardware | [Operations](operations/dell-hardware.md) |

### Services

| Service | Location |
|---------|----------|
| Media (Plex, Arr) | [Getting Started](setup/getting-started.md#media-services) |
| AI/ML (llama.cpp, ComfyUI) | [Getting Started](setup/getting-started.md#aiml-services) |
| Monitoring (Grafana, Prometheus) | [Getting Started](setup/getting-started.md#monitoring-setup) |

---

## Service Access

### Web Interfaces

| Service | URL |
|---------|-----|
| Grafana | `http://192.0.2.143:8910` |
| Prometheus | `http://192.0.2.143:9090` |
| Plex | `http://192.0.2.143:32400` |
| Open WebUI | `http://192.0.2.143:3000` |
| llama.cpp | `http://192.0.2.143:8080` |
| Pi-hole | `http://192.0.2.9/admin` |
| Proxmox node006 | `https://192.0.2.106:8006` |
| Proxmox node005 | `https://192.0.2.105:8006` |

### SSH Access

```bash
# Ocean (Docker services)
ssh terrac@192.0.2.143

# Proxmox hosts
ssh root@192.0.2.106  # node006
ssh root@192.0.2.105  # node005

# VMs
ssh debian@192.0.2.2   # dns01
ssh debian@192.0.2.5   # gitlab
ssh debian@192.0.2.9   # pihole
```

---

## Common Commands

### Service Deployment

```bash
# Deploy all ocean services
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/03_ocean_services.yaml --ask-vault-pass

# Deploy specific service
ansible-playbook -i inventories/production/hosts.ini \
  playbooks/individual/ocean/media/plex.yaml --ask-vault-pass
```

### Status Checks

```bash
# Check Docker services on ocean
ssh terrac@192.0.2.143 "docker ps --format 'table {{.Names}}\t{{.Status}}'"

# Check ZFS pool
ssh terrac@192.0.2.143 "zpool status data01"

# Check GPU status
ssh terrac@192.0.2.143 "nvidia-smi"
```

### Logs and Diagnostics

```bash
# View service logs
ssh terrac@192.0.2.143 "docker logs plex --tail 50"

# Test DNS
nslookup ocean.home 192.0.2.2

# Check Prometheus targets
curl -s http://192.0.2.143:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'
```

---

## Emergency Procedures

| Issue | Documentation |
|-------|---------------|
| System Down | [Emergency Recovery](troubleshooting/common-issues.md#emergency-recovery-procedures) |
| Network Issues | [Network Troubleshooting](troubleshooting/common-issues.md#network-issues) |
| Storage Failure | [ZFS Disk Replacement](operations/zfs-disk-replacement.md) |
| GPU Issues | [GPU Troubleshooting](operations/gpu-management.md#troubleshooting) |

---

## Related Files

- [playbooks/README.md](/playbooks/README.md) - Playbook documentation
- [DEVELOPMENT.md](/DEVELOPMENT.md) - Development environment setup
- [vault/secrets.yaml.template](/vault/secrets.yaml.template) - Vault structure
