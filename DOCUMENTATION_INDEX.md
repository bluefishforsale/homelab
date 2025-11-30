# Documentation Index

Index of all documentation in the homelab repository.

---

## Structure

```text
homelab/
├── Readme.md                    # Main project overview
├── ROADMAP.md                   # Status and future plans
├── DEVELOPMENT.md               # Developer setup
├── CONTRIBUTING.md              # Contribution guidelines
│
├── docs/
│   ├── README.md                # Documentation hub
│   │
│   ├── architecture/
│   │   ├── README.md            # Architecture index
│   │   ├── overview.md          # System architecture
│   │   ├── networking.md        # Network configuration
│   │   ├── network-topology.md  # Network diagram
│   │   ├── ocean-services.md    # Ocean service diagram
│   │   ├── deployment-flow.md   # Deployment order
│   │   └── physical-architecture.md  # Rack layout
│   │
│   ├── setup/
│   │   ├── getting-started.md   # Deployment guide
│   │   └── macos-setup.md       # Local dev setup
│   │
│   ├── operations/
│   │   ├── proxmox.md           # VM management
│   │   ├── zfs.md               # ZFS storage
│   │   ├── zfs-disk-replacement.md  # Disk failure recovery
│   │   ├── gpu-management.md    # RTX 3090 configuration
│   │   ├── dell-hardware.md     # iDRAC, RAID
│   │   ├── unifi.md             # Switch/AP config
│   │   └── ocean-migration-plan.md  # Migration notes
│   │
│   └── troubleshooting/
│       └── common-issues.md     # Problem resolution
│
├── playbooks/
│   └── README.md                # Playbook reference
│
└── .github/
    └── SETUP.md                 # GitHub Actions setup
```

---

## Quick Links

### Getting Started

| Document | Description |
|----------|-------------|
| [Readme.md](/Readme.md) | Project overview |
| [docs/setup/getting-started.md](/docs/setup/getting-started.md) | Deployment guide |
| [docs/setup/macos-setup.md](/docs/setup/macos-setup.md) | Local development |

### Architecture

| Document | Description |
|----------|-------------|
| [docs/architecture/overview.md](/docs/architecture/overview.md) | System architecture |
| [docs/architecture/networking.md](/docs/architecture/networking.md) | Network configuration |
| [docs/architecture/ocean-services.md](/docs/architecture/ocean-services.md) | Service diagram |
| [docs/architecture/deployment-flow.md](/docs/architecture/deployment-flow.md) | Deployment order |

### Operations

| Document | Description |
|----------|-------------|
| [docs/operations/proxmox.md](/docs/operations/proxmox.md) | VM management |
| [docs/operations/zfs.md](/docs/operations/zfs.md) | ZFS storage |
| [docs/operations/gpu-management.md](/docs/operations/gpu-management.md) | GPU configuration |
| [docs/operations/dell-hardware.md](/docs/operations/dell-hardware.md) | Hardware management |
| [docs/operations/unifi.md](/docs/operations/unifi.md) | Network operations |

### Reference

| Document | Description |
|----------|-------------|
| [playbooks/README.md](/playbooks/README.md) | Playbook usage |
| [ROADMAP.md](/ROADMAP.md) | Project status |
| [.github/SETUP.md](/.github/SETUP.md) | CI/CD setup |

---

## Infrastructure Summary

| Host | IP | Purpose |
|------|----|---------|
| node005 | 192.168.1.105 | Proxmox - Control VMs |
| node006 | 192.168.1.106 | Proxmox - Ocean VM |
| ocean | 192.168.1.143 | Docker services |
| dns01 | 192.168.1.2 | BIND DNS |
| pihole | 192.168.1.9 | DNS filtering |
| gitlab | 192.168.1.5 | CI/CD |

---

## Key Topics

| Topic | Document |
|-------|----------|
| Deploy services | [playbooks/README.md](/playbooks/README.md) |
| GPU passthrough | [gpu-management.md](/docs/operations/gpu-management.md) |
| ZFS disk failure | [zfs-disk-replacement.md](/docs/operations/zfs-disk-replacement.md) |
| Network troubleshooting | [unifi.md](/docs/operations/unifi.md) |
| Common issues | [common-issues.md](/docs/troubleshooting/common-issues.md) |
