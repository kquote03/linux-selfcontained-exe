#!/bin/bash
# Cross-compiles a Linux kernel matching Debian's riscv64 package version,
# with CONFIG_DRM_SIMPLEDRM + CONFIG_SYSFB_SIMPLEFB built in (required for
# the RVVM guest display to work at all — see docs/risk-register.md and
# docs/phase1-spike-results.md for why the stock Debian kernel can't be used
# as-is). Trims several large, irrelevant driver trees (GPU vendors other
# than the simple-framebuffer path, all NIC vendors except Realtek, legacy
# SCSI/FC HBAs, the wireless stack) so the cross-compile finishes in a
# reasonable time. Installs the result into the rootfs and regenerates its
# initramfs.
#
# Prerequisites (apt-get install on the WSL/Linux build host):
#   crossbuild-essential-riscv64 build-essential bc bison flex libssl-dev
#   libelf-dev cpio kmod rsync
#
# Run as root. Must run build-rootfs.sh first (this script installs into
# $ROOTFS, which build-rootfs.sh creates and populates with the stock
# Debian kernel that this script's output replaces).
set -euo pipefail

BUILD_DIR=/root/build
ROOTFS="$BUILD_DIR/rootfs"
KBUILD_DIR=/root/kbuild
KVER_FULL=6.12.111
LOCALVERSION="+deb13-riscv64"
KVER="${KVER_FULL}${LOCALVERSION}"
JOBS="${JOBS:-4}"

mkdir -p "$KBUILD_DIR"
KSRC="$KBUILD_DIR/linux-$KVER_FULL"

if [ ! -d "$KSRC" ]; then
  echo "=== Downloading kernel source $KVER_FULL ==="
  curl -sL "https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-$KVER_FULL.tar.xz" \
    -o "$KBUILD_DIR/linux-$KVER_FULL.tar.xz"
  tar xf "$KBUILD_DIR/linux-$KVER_FULL.tar.xz" -C "$KBUILD_DIR"
fi

cd "$KSRC"

echo "=== Seeding config from Debian's shipped kernel ==="
DEBIAN_CONFIG=$(ls "$ROOTFS"/boot/config-*"$LOCALVERSION" 2>/dev/null | head -1)
if [ -z "$DEBIAN_CONFIG" ]; then
  echo "ERROR: no Debian kernel config found in $ROOTFS/boot/ — run build-rootfs.sh first" >&2
  exit 1
fi
cp "$DEBIAN_CONFIG" .config

./scripts/config --enable CONFIG_SYSFB_SIMPLEFB
./scripts/config --enable CONFIG_DRM
./scripts/config --enable CONFIG_DRM_SIMPLEDRM
# RVVM's emulated keyboard/mouse is NOT a Goldfish device (that hypothesis
# was wrong - see docs/phase1-spike-results.md correction note). Dumping
# RVVM's actual generated device tree (`rvvm -dumpdtb`) shows an OpenCores
# I2C controller (compatible "opencores,i2c-ocores") with three child nodes
# compatible "hid-over-i2c" - i.e. RVVM exposes keyboard/mouse/touch as
# HID-over-I2C devices described via the device tree, not via any Goldfish
# node (goldfish_rtc is the only actual Goldfish device RVVM exposes).
# The Debian stock config has the whole chain as modules, and critically is
# missing CONFIG_I2C_HID_OF entirely - that's the specific glue driver that
# matches a DT "hid-over-i2c" node to the i2c-hid core (CONFIG_I2C_HID_ACPI
# is the sibling driver for ACPI-described devices, irrelevant here since
# ACPI is disabled on this platform). Without I2C_HID_OF, none of the three
# input devices ever bind, regardless of I2C_HID/I2C_OCORES/HID being
# present. Built-in (not modules) for the same reason as DRM_SIMPLEDRM: the
# lightdm greeter needs keyboard/mouse immediately, before any modprobe/udev
# module-loading race could resolve.
./scripts/config --enable CONFIG_I2C_OCORES
./scripts/config --enable CONFIG_HID
./scripts/config --enable CONFIG_HID_GENERIC
./scripts/config --enable CONFIG_I2C_HID
./scripts/config --enable CONFIG_I2C_HID_OF
./scripts/config --set-str CONFIG_LOCALVERSION "$LOCALVERSION"
./scripts/config --disable CONFIG_LOCALVERSION_AUTO

echo "=== Trimming large irrelevant driver trees (GPU vendors, non-Realtek NICs, legacy SCSI/FC, wireless) ==="
./scripts/config --disable CONFIG_DRM_AMDGPU
./scripts/config --disable CONFIG_DRM_RADEON
./scripts/config --disable CONFIG_DRM_NOUVEAU
./scripts/config --disable CONFIG_DRM_I915
./scripts/config --disable CONFIG_DRM_AST
./scripts/config --disable CONFIG_SCSI_LOWLEVEL
./scripts/config --disable CONFIG_MAC80211
./scripts/config --disable CONFIG_WLAN
./scripts/config --disable CONFIG_RDS
./scripts/config --disable CONFIG_INFINIBAND
./scripts/config --disable CONFIG_ATM
for v in $(grep "^CONFIG_NET_VENDOR_.*=y" .config | sed 's/^CONFIG_//;s/=y//'); do
  [ "$v" = "NET_VENDOR_REALTEK" ] || ./scripts/config --disable "CONFIG_$v"
done

echo "=== Disabling per-syscall/per-access security overhead not needed for a single-user ephemeral teaching VM ==="
./scripts/config --disable CONFIG_AUDIT
./scripts/config --disable CONFIG_IMA
./scripts/config --disable CONFIG_EVM
./scripts/config --disable CONFIG_SECURITY_APPARMOR
./scripts/config --disable CONFIG_FTRACE
./scripts/config --disable CONFIG_KPROBES

make ARCH=riscv CROSS_COMPILE=riscv64-linux-gnu- olddefconfig

echo "=== Verifying critical options survived ==="
grep -qE "^CONFIG_DRM_SIMPLEDRM=y" .config || { echo "ERROR: CONFIG_DRM_SIMPLEDRM not enabled"; exit 1; }
grep -qE "^CONFIG_SYSFB_SIMPLEFB=y" .config || { echo "ERROR: CONFIG_SYSFB_SIMPLEFB not enabled"; exit 1; }
grep -qE "^CONFIG_R8169=" .config || { echo "ERROR: CONFIG_R8169 missing (RVVM's NIC driver)"; exit 1; }
grep -qE "^CONFIG_I2C_OCORES=y" .config || { echo "ERROR: CONFIG_I2C_OCORES not enabled"; exit 1; }
grep -qE "^CONFIG_I2C_HID=y" .config || { echo "ERROR: CONFIG_I2C_HID not enabled"; exit 1; }
grep -qE "^CONFIG_I2C_HID_OF=y" .config || { echo "ERROR: CONFIG_I2C_HID_OF not enabled (this is the glue driver for RVVM's DT-described hid-over-i2c input devices)"; exit 1; }

echo "=== Verifying perf-hardening options were dropped (soft check - informational only) ==="
for opt in CONFIG_AUDIT CONFIG_IMA CONFIG_EVM CONFIG_SECURITY_APPARMOR; do
  grep -qE "^# ${opt} is not set" .config || echo "WARN: $opt still enabled after olddefconfig (a dependency may have forced it back on) - not blocking the build"
done

echo "=== Building kernel + modules + dtbs (this takes a while, even cross-compiled natively) ==="
make -j"$JOBS" ARCH=riscv CROSS_COMPILE=riscv64-linux-gnu- Image modules dtbs

echo "=== Installing modules into rootfs ==="
# IMPORTANT: do this BEFORE removing any old modules directory. The new
# kernel uses the identical Debian version string (LOCALVERSION preserved
# above), so if you ever add a "remove old modules" step, it MUST run
# before modules_install, not after — running it after deletes the modules
# you just built. (This bit us once; see git history / phase1-spike-results.md.)
make -j"$JOBS" ARCH=riscv CROSS_COMPILE=riscv64-linux-gnu- INSTALL_MOD_PATH="$ROOTFS" modules_install

echo "=== Installing kernel image + config + System.map ==="
cp "$KSRC"/arch/riscv/boot/Image "$ROOTFS"/boot/vmlinux-"$KVER"
cp "$KSRC"/.config "$ROOTFS"/boot/config-"$KVER"
cp "$KSRC"/System.map "$ROOTFS"/boot/System.map-"$KVER"

echo "=== depmod + initramfs regeneration ==="
mountpoint -q "$ROOTFS"/dev || mount --bind /dev "$ROOTFS"/dev
mountpoint -q "$ROOTFS"/proc || mount --bind /proc "$ROOTFS"/proc
mountpoint -q "$ROOTFS"/sys || mount --bind /sys "$ROOTFS"/sys
chroot "$ROOTFS" depmod -a "$KVER"
chroot "$ROOTFS" update-initramfs -c -k "$KVER"

echo "=== Done. Kernel $KVER installed into $ROOTFS ==="
ls -la "$ROOTFS"/boot/
echo "$KVER" > "$KBUILD_DIR"/kver.txt
echo "Next: run make-disk-image.sh to produce the final disk.img."
