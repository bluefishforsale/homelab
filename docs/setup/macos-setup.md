# Readme OSX / Proxmox

# install VM solution
# requirements
brew install qemu

# MacOS VPN kit for bridging
# terminal 1 qemu-local
# requirements
brew install opam
opam init
# package build and start
git clone https://github.com/moby/vpnkit.git
cd vpnkit
make
./vpnkit --ethernet /tmp/vpnkit.sock


# These prep steps are done in your data dir (here it's another disk)
# terminal 1
cd /Volumes/4TB/VMs/ProxMox

# download required ISOs
wget https://enterprise.proxmox.com/iso/proxmox-ve_8.3-1.iso
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-arm64.qcow2

# Create Qemu disk for Proxmox to be installed on
qemu-img create proxmox.img 10G


# start in the local directory with relative sources and files
# terminal 2
cd qemu-local || mkdir qemu-local

# make the cloud-init iso
# requirements
brew install cdrtools

touch cloud-init/meta-data
mkisofs -output cloud-init.iso -volid cidata -joliet -rock cloud-init/user-data cloud-init/meta-data
cp cloud-init.iso /Volumes/4TB/VMs/ProxMox/

# Run these from terminal 1 (the data dir)
cd /Volumes/4TB/VMs/ProxMox

# test that QEMU can run at least native arm64 img
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
  -netdev socket,id=net0,path=/tmp/vpnkit.sock -device virtio-net-pci,netdev=net0
  -nodefaults


# run proxmox ISO and created disk image for installation phase 
qemu-system-x86_64 \
  -M q35 \
  -enable-kvm \
  -bios "/opt/homebrew/Cellar/qemu/9.2.0/share/qemu/edk2-x86_64-code.fd" \
  -m 4G \
  -smp 4 \
  -cpu host \
  -drive file=proxmox-ve_8.3-1.iso,if=virtio,cache=writeback \
  -drive file=cloud-init.iso,if=virtio \
  -boot menu=on \
  -serial stdio \
  -display none \
  -netdev bridge,id=net0,br=vm-br0 -device virtio-net-pci,netdev=net0 \
  -nodefaults