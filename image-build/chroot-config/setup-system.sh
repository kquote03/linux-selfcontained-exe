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
           e2scrub_all.timer NetworkManager.service NetworkManager-wait-online.service; do
  systemctl disable "$svc" 2>/dev/null || true
  systemctl mask "$svc" 2>/dev/null || true
done

# --- regenerate initramfs after all config changes ---
update-initramfs -u -k all
