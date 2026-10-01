#!/bin/sh
# Runs *inside* the riscv64 chroot (via qemu-riscv64 binfmt) during image build.
# Configures the teaching-lab appliance: autologin student user, DHCP
# networking, disabled compositor, and trimmed-down service set.
set -e

# --- remove plymouth ---
# Boot-splash IPC handshake (lightdm waiting on "Quitting Plymouth") hangs
# indefinitely under RVVM's framebuffer, blocking Xorg from ever starting.
# Not needed for a kiosk-style lab appliance anyway.
apt-get purge -y plymouth plymouth-label libplymouth5 2>/dev/null || true

# --- user account ---
id -u student >/dev/null 2>&1 || useradd -m -s /bin/bash -G sudo student
echo "student:student" | chpasswd
echo "root:root" | chpasswd
mkdir -p /etc/sudoers.d
echo "student ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/student
chmod 440 /etc/sudoers.d/student

# --- lightdm autologin ---
mkdir -p /etc/lightdm/lightdm.conf.d
cat > /etc/lightdm/lightdm.conf.d/50-autologin.conf <<'EOF'
[Seat:*]
autologin-user=student
autologin-user-timeout=0
autologin-session=xfce
EOF

# --- disable XFWM4 compositor by default (weak/emulated hardware) ---
mkdir -p /etc/skel/.config/xfce4/xfconf/xfce-perchannel-xml
cat > /etc/skel/.config/xfce4/xfconf/xfce-perchannel-xml/xfwm4.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<channel name="xfwm4" version="1.0">
  <property name="general" type="empty">
    <property name="use_compositing" type="bool" value="false"/>
  </property>
</channel>
EOF
# Apply the same skeleton to the already-created student home.
mkdir -p /home/student/.config/xfce4/xfconf/xfce-perchannel-xml
cp /etc/skel/.config/xfce4/xfconf/xfce-perchannel-xml/xfwm4.xml \
   /home/student/.config/xfce4/xfconf/xfce-perchannel-xml/xfwm4.xml
chown -R student:student /home/student/.config

# --- networking: systemd-networkd + resolved, plain DHCP ---
mkdir -p /etc/systemd/network
cat > /etc/systemd/network/20-wired.network <<'EOF'
[Match]
Name=en*

[Network]
DHCP=yes
EOF
systemctl enable systemd-networkd
rm -f /etc/resolv.conf
cat > /etc/resolv.conf <<'EOF'
nameserver 9.9.9.9
nameserver 1.1.1.1
EOF

# --- hostname / locale ---
echo "linux-lab" > /etc/hostname
cat > /etc/hosts <<'EOF'
127.0.0.1 localhost
127.0.1.1 linux-lab
EOF
echo "LANG=C.UTF-8" > /etc/default/locale

# --- ssh (installed, but not exposed via -portfwd by default; see docs/architecture.md) ---
systemctl enable ssh

# --- trim/disable services not needed on an ephemeral single-purpose box ---
for svc in bluetooth.service cups.service cups-browsed.service \
           ModemManager.service avahi-daemon.service \
           apt-daily.timer apt-daily-upgrade.timer man-db.timer \
           e2scrub_all.timer NetworkManager.service NetworkManager-wait-online.service \
           tracker-miner-fs-3.service tracker-extract-3.service tracker-miner-fs-control-3.service \
           tracker-xdg-portal-3.service tracker-writeback-3.service \
           at-spi-dbus-bus.service packagekit.service; do
  systemctl disable "$svc" 2>/dev/null || true
  systemctl mask "$svc" 2>/dev/null || true
done

# --- zram: compressed RAM-backed swap instead of a disk-backed swap file ---
# Round 3 (weak-hardware tuning): default guest RAM is only 1G, so a little
# swap headroom avoids OOM-killing XFCE under load. zram is far cheaper than
# disk swap against RVVM's emulated NVMe, and systemd-zram-generator's own
# default sizing (min(ram/2, 4096)) already lands at a sensible ~512M here.
cat > /etc/systemd/zram-generator.conf <<'EOF'
[zram0]
compression-algorithm = zstd
EOF

# --- journald: volatile (RAM-only) storage, no disk I/O for logging ---
sed -i 's/^#\?Storage=.*/Storage=volatile/' /etc/systemd/journald.conf
sed -i 's/^#\?RuntimeMaxUse=.*/RuntimeMaxUse=16M/' /etc/systemd/journald.conf

# --- I/O scheduler/readahead: tuned for RVVM's emulated NVMe, not real disk ---
# There's no seek cost to optimize for on an emulated block device, so
# "none" (no scheduling overhead) and a small readahead avoid wasted work.
cat > /etc/udev/rules.d/60-nvme-scheduler.rules <<'EOF'
ACTION=="add|change", KERNEL=="nvme[0-9]n[0-9]", ATTR{queue/scheduler}="none", ATTR{bdi/read_ahead_kb}="128"
EOF

# --- initramfs: curated module list instead of MODULES=most ---
# MODULES=most ships literally every module Debian's generic kernel has,
# most never needed pre-root-mount, producing a 150+MB initrd loaded on
# every single student launch. Only storage-path modules belong in the
# initrd; NIC/input load later via normal udev+depmod post-root regardless
# of initrd contents. Deliberately NOT using MODULES=dep: that mode infers
# the needed set from the *currently running* system's loaded modules,
# which inside this qemu-riscv64 chroot would reflect the build host's own
# state, not the target's - unreliable in a cross-build chroot. An explicit
# list is the only safe automatic option here. If a future boot fails with
# "Unable to mount root fs", revert MODULES= back to "most" below as a
# one-line fix - no kernel rebuild needed, just re-run update-initramfs.
sed -i 's/^MODULES=.*/MODULES=list/' /etc/initramfs-tools/initramfs.conf
cat > /etc/initramfs-tools/modules <<'EOF'
nvme
ext4
EOF

# --- regenerate initramfs after all config changes ---
update-initramfs -u -k all
