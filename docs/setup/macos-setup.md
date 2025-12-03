# macOS Setup

## Development Environment

For local Ansible development on macOS, see [DEVELOPMENT.md](/DEVELOPMENT.md).

```bash
# Quick setup
make setup
echo 'export PATH="$HOME/Library/Python/3.13/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
make validate
```

---

## Experimental: Proxmox in QEMU on macOS

**Note**: This is experimental/reference documentation for running Proxmox virtualized on macOS. Not recommended for production use.

### Prerequisites

```bash
brew install qemu cdrtools wget
```

### VPNKit for Networking (Optional)

```bash
brew install opam
opam init
git clone https://github.com/moby/vpnkit.git
cd vpnkit && make
./vpnkit --ethernet /tmp/vpnkit.sock
```

### Download ISOs

```bash
mkdir -p ~/VMs/ProxMox && cd ~/VMs/ProxMox

# Proxmox ISO
wget https://enterprise.proxmox.com/iso/proxmox-ve_8.3-1.iso

# Debian cloud images (for testing)
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-arm64.qcow2

# Create disk for Proxmox
qemu-img create proxmox.img 10G
```

### Cloud-Init ISO

```bash
mkdir -p cloud-init
touch cloud-init/meta-data
# Edit cloud-init/user-data with your config
mkisofs -output cloud-init.iso -volid cidata -joliet -rock cloud-init/user-data cloud-init/meta-data
```

### Test ARM64 VM (Apple Silicon)

```bash
qemu-system-aarch64 \
  -M virt,highmem=off \
  -bios "/opt/homebrew/Cellar/qemu/9.2.0/share/qemu/edk2-aarch64-code.fd" \
  -accel hvf \
  -m 1G \
  -smp 4 \
  -cpu cortex-a72 \
  -drive file=debian-12-generic-arm64.qcow2,if=virtio,format=qcow2 \
  -drive file=cloud-init.iso,if=virtio,format=raw \
  -boot d \
  -serial stdio \
  -display none \
  -netdev socket,id=net0,path=/tmp/vpnkit.sock \
  -device virtio-net-pci,netdev=net0
```

### Run Proxmox x86_64 (Intel Mac Only)

```bash
qemu-system-x86_64 \
  -M q35 \
  -enable-kvm \
  -bios "/opt/homebrew/Cellar/qemu/9.2.0/share/qemu/edk2-x86_64-code.fd" \
  -m 4G \
  -smp 4 \
  -cpu host \
  -drive file=proxmox-ve_8.3-1.iso,if=virtio,cache=writeback \
  -drive file=proxmox.img,if=virtio,format=raw \
  -boot menu=on \
  -serial stdio \
  -display none \
  -netdev user,id=net0 \
  -device virtio-net-pci,netdev=net0
```

**Note**: `-enable-kvm` requires Intel Mac with KVM support. Apple Silicon cannot run x86 Proxmox.