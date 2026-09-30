#!/bin/bash
# Partitions, formats, and populates the final raw disk image from the
# built rootfs. Run as root inside WSL (or any Linux build host).
set -euo pipefail

BUILD_DIR="/root/build"
ROOTFS="$BUILD_DIR/rootfs"
IMG="$BUILD_DIR/disk.img"

losetup -D || true

truncate -s 8G "$IMG"
parted -s "$IMG" mklabel gpt mkpart primary ext4 1MiB 100%

LOOP=$(losetup -fP --show "$IMG")
echo "Using loop device: $LOOP"

mkfs.ext4 -L rootfs "${LOOP}p1"

MNT="$BUILD_DIR/mnt"
mkdir -p "$MNT"
mount "${LOOP}p1" "$MNT"

echo "Copying rootfs into image (this takes a while)..."
rsync -aHAX --exclude=/proc/* --exclude=/sys/* --exclude=/dev/* --exclude=/tmp/* \
  "$ROOTFS"/ "$MNT"/
mkdir -p "$MNT"/proc "$MNT"/sys "$MNT"/dev "$MNT"/tmp

ROOT_UUID=$(blkid -s UUID -o value "${LOOP}p1")
echo "Root filesystem UUID: $ROOT_UUID"

KERNEL_FILE=$(basename "$(ls "$MNT"/boot/vmlinux-* | head -1)")
INITRD_FILE=$(basename "$(ls "$MNT"/boot/initrd.img-* | head -1)")
echo "Kernel: $KERNEL_FILE  Initrd: $INITRD_FILE"

mkdir -p "$MNT"/boot/extlinux
cat > "$MNT"/boot/extlinux/extlinux.conf <<EOF
DEFAULT linux
LABEL linux
	KERNEL /boot/$KERNEL_FILE
	INITRD /boot/$INITRD_FILE
	APPEND rw root=UUID=$ROOT_UUID rootwait console=ttyS0 console=tty0 mitigations=off
EOF
cat "$MNT"/boot/extlinux/extlinux.conf

cat > "$MNT"/etc/fstab <<EOF
UUID=$ROOT_UUID / ext4 defaults 0 1
EOF

sync
umount "$MNT"
losetup -d "$LOOP"

echo "Disk image built: $IMG"
ls -la "$IMG"
