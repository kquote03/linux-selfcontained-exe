# Phase 1 spike results: guest image build + display fix

Status: **PASS — full XFCE4 desktop confirmed rendering**

## Summary

Built the first real customized Debian 13 riscv64 + XFCE4 image
(`image-build/`) via cross-arch debootstrap in WSL, and booted it under RVVM.
Two real problems surfaced and were fixed; see `docs/risk-register.md` for
the full entries.

## Problem 1: Plymouth hang

`lightdm` hung indefinitely at "Quitting Plymouth" and never even attempted
to start Xorg (no `Xorg.0.log` was ever created). Root-caused by mounting
the actual disk image built during the failed boot and reading
`/var/log/lightdm/lightdm.log` directly.

**Fix:** `apt-get purge plymouth plymouth-label libplymouth5` — now baked
into `image-build/chroot-config/setup-system.sh`. Not needed for a
kiosk-style lab appliance anyway.

## Problem 2: Debian's stock kernel has no simple-framebuffer driver

After removing plymouth, the GUI window still froze at U-Boot's last frame
— the OS booted fine underneath (confirmed via serial console: full systemd
boot, networking, even lightdm reaching "Started Light Display Manager"),
but nothing ever appeared in the RVVM window.

Root cause, found by inspecting `/boot/config-*` in the built rootfs:

```
# CONFIG_SYSFB_SIMPLEFB is not set
# CONFIG_FB_SIMPLE is not set
# CONFIG_DRM_SIMPLEDRM is not set
```

U-Boot hands off a `simple-framebuffer` device-tree node so a generic OS
kernel can pick up the display without a hardware-specific driver — but
Debian's riscv64 kernel doesn't enable the driver that consumes it. RVVM has
no PCI GPU device, so there's no alternative driver path either.

**Fix:** cross-compiled a matching kernel (Linux 6.12.111, the exact
upstream version Debian's package is based on) with `CONFIG_DRM_SIMPLEDRM=y`
and `CONFIG_SYSFB_SIMPLEFB=y` forced built-in (not modules — had to also
force `CONFIG_DRM=y` since Debian's default has DRM itself as a module,
which caps submodules at `=m`). Full process and scripts:
`image-build/trim-ethernet-vendors.sh`, `image-build/install-custom-kernel.sh`,
cross-toolchain via `crossbuild-essential-riscv64` on the WSL host (native
cross-compile, not emulated — an emulated build would have taken
prohibitively long).

Also trimmed several huge, irrelevant driver trees to keep cross-compile
time reasonable (amdgpu/radeon/nouveau GPU drivers, all NIC vendors except
Realtek, legacy SCSI/FC HBAs, the wireless stack) — see risk register for
the full list.

## Result

`docs/phase1-evidence/01-xfce4-desktop-rendering-success.png`: a full XFCE4
desktop — panel with clock and "student" username (confirming autologin
worked), desktop icons (Home, File System, Trash), Debian wallpaper, and the
application dock — rendering correctly inside the RVVM GUI window on this
project's real dev machine (no hardware virtualization).

## What's not yet validated

- Mouse click precision in the guest (keyboard input is confirmed reliable
  via Phase 0 and this spike's autologin; mouse click testing here was
  imprecise due to automated-test coordinate guessing, not a product issue —
  not worth chasing further before a human tests with a real mouse).
- `curl`/`ping`/`apt update` from a terminal inside this specific XFCE4
  image (confirmed separately on the plain Arch test image in Phase 0; the
  same RVVM networking path is unchanged here, so expected to work, but not
  re-confirmed on this exact image).
- Boot-to-desktop timing on genuinely weak/representative hardware (this
  session's boot took roughly 60-90s to reach the desktop on the dev
  machine, which is not necessarily representative of target lab hardware).
- The known self-inflicted bug where `install-custom-kernel.sh` originally
  deleted its own freshly-installed modules (fixed — see the comment left in
  that script) is now corrected in the repo version.
