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

- Mouse/keyboard-shortcut interaction inside the XFCE4 session specifically.
  Synthetic mouse clicks (`SetCursorPos`+`mouse_event`) and the `Alt+F2`
  app-finder shortcut both had no visible effect when tested against the
  running desktop, despite login-screen keyboard input (typing the
  username/password) working reliably. This looks like a synthetic-input
  injection limitation of this test setup — RVVM likely expects relative
  mouse deltas or real window-focused hardware input rather than absolute
  `SetCursorPos` injection, and Alt+F2 may simply not be bound by default in
  this minimal XFCE install. A real user with a physical mouse/keyboard on
  the actual window is expected to have no such issue; this needs a human
  to confirm rather than further scripted automation.

  **Correction (Round 2):** this hypothesis was wrong. Initially
  root-caused (incorrectly) to a missing `CONFIG_KEYBOARD_GOLDFISH_EVENTS`
  driver, on the assumption that RVVM's input follows the same
  Android-emulator "Goldfish" device family as `goldfish_rtc` (which does
  work). That assumption was itself wrong and superseded almost immediately
  — see the second correction below. The login-screen keyboard input that
  *did* work was on a completely different kernel (the prebuilt Arch test
  image from Phase 0), never actually exercised on our own build until this
  was investigated.

  **Correction 2 (Round 2, same session):** the Goldfish hypothesis was
  also wrong. Dumping RVVM's actual generated device tree (`rvvm
  -dumpdtb`) shows no Goldfish input node at all — RVVM exposes
  keyboard/mouse as three `hid-over-i2c` devices on an OpenCores I2C
  controller. The real missing piece was `CONFIG_I2C_HID_OF`, the
  device-tree glue driver for HID-over-I2C (distinct from
  `CONFIG_I2C_HID_ACPI`, which is for ACPI-described devices and doesn't
  apply here). See `docs/phase2-spike-results.md` for the full
  investigation and verification evidence.
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
