#!/bin/bash
# Builds the base Debian 13 (trixie) riscv64 + XFCE4 rootfs via cross-arch
# debootstrap. Run as root on a Linux host (WSL Ubuntu during development).
# Prerequisites: debootstrap, qemu-user-binfmt (riscv64 interpreter
# registered), systemd-container. See docs/architecture.md for why this
# approach (cross-arch chroot, not booting a VM) was chosen.
set -euo pipefail

BUILD_DIR=/root/build
ROOTFS="$BUILD_DIR/rootfs"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p "$ROOTFS"

echo "=== Stage 1: debootstrap (first stage, foreign) ==="
debootstrap --arch=riscv64 --variant=minbase --foreign trixie "$ROOTFS" http://deb.debian.org/debian

echo "=== Stage 2: second stage via qemu-riscv64 binfmt ==="
cp /usr/bin/qemu-riscv64 "$ROOTFS"/usr/bin/
chroot "$ROOTFS" /debootstrap/debootstrap --second-stage

echo "=== apt sources + resolv.conf ==="
cat > "$ROOTFS"/etc/apt/sources.list <<'EOF'
deb http://deb.debian.org/debian trixie main
deb http://deb.debian.org/debian trixie-updates main
deb http://deb.debian.org/debian-security trixie-security main
EOF
cp /etc/resolv.conf "$ROOTFS"/etc/resolv.conf

mountpoint -q "$ROOTFS"/dev || mount --bind /dev "$ROOTFS"/dev
mountpoint -q "$ROOTFS"/proc || mount --bind /proc "$ROOTFS"/proc
mountpoint -q "$ROOTFS"/sys || mount --bind /sys "$ROOTFS"/sys

echo "=== apt-get update ==="
chroot "$ROOTFS" apt-get update

echo "=== Install kernel + XFCE4 + CLI tools ==="
chroot "$ROOTFS" env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends linux-image-riscv64
chroot "$ROOTFS" env DEBIAN_FRONTEND=noninteractive apt-get install -y \
  xfce4 xfce4-terminal mousepad lightdm openssh-server curl iputils-ping sudo \
  systemd-zram-generator

echo "=== System configuration (autologin, networking, disabled services, plymouth removal) ==="
cp "$SCRIPT_DIR"/chroot-config/setup-system.sh "$ROOTFS"/tmp/setup-system.sh
chmod +x "$ROOTFS"/tmp/setup-system.sh
chroot "$ROOTFS" /tmp/setup-system.sh
rm -f "$ROOTFS"/tmp/setup-system.sh

echo "=== Cleanup ==="
chroot "$ROOTFS" apt-get clean

echo "=== Base rootfs build complete ==="
du -sh "$ROOTFS" 2>/dev/null | tail -1
echo "Next: run build-kernel.sh to build and install the custom display-enabled kernel,"
echo "then make-disk-image.sh to produce the final disk.img."
