# Architecture

This is the living copy of the approved project plan. For the original
plan-mode record, see the conversation history; this file is the version to
keep updated as decisions evolve during implementation.

## Context

The goal is a single Windows `.exe` that a student double-clicks to get a
fully booted Debian 13 "trixie" desktop with no installation, no admin
rights, and no prior setup — used to teach an entire lab the basics of Linux
without walking each student through an OS install. It must run on weak
x86_64 Windows PCs (SSE2 baseline — i.e. any 64-bit Windows machine) and give
the guest real internet access.

Two facts materially shaped the design:

1. **The target Windows hosts are themselves VMs with no nested
   virtualization exposed, and this can't be changed.** No hardware-
   accelerated emulation (WHPX/HAXM/KVM) is available for *any* guest
   architecture — an x86_64 guest gets no speed advantage over a RISC-V
   guest, since both run under pure software CPU emulation (TCG-class)
   either way. This is why a RISC-V guest is a reasonable choice rather than
   a self-inflicted handicap: without acceleration, "host arch matches guest
   arch" buys nothing.
2. **Debian 13 (trixie) is the first Debian release with official riscv64
   support** — Debian's own image-baking infrastructure (DQIB, cloud images)
   already targets riscv64.

Decisions made on that basis:

- **Emulator: [RVVM](https://github.com/LekKit/RVVM)**, chosen over QEMU.
  Accepted tradeoff: RVVM is a much smaller, thinner-documented, largely
  one-maintainer project, and its device model differs fundamentally from
  QEMU's (NVMe + RTL8169 instead of virtio-blk/virtio-net) — nobody has
  documented booting a full custom desktop image on it. See
  `docs/risk-register.md`. A feasibility spike (Phase 0) gates the rest of
  the build; QEMU is a documented, cheap-to-switch-to fallback if it fails.
- **Scope:** command-line/networking exercises only (curl, ping, apt, ssh) —
  no requirement for a usable GUI browser.
- **Distribution:** one large self-extracting exe (multi-GB is fine), no
  server/hosting needed at runtime.
- **Persistence:** ephemeral. Every launch boots an identical pristine
  environment; nothing survives after closing.

## Phase 0 — Feasibility spike

Before investing in the full XFCE4 image, validate on real target-shaped
hardware (a Windows VM with no nested virtualization):

- **0a — Boot + network (serial console only).** Boot Debian's DQIB riscv64
  prebuilt kernel/image under RVVM with OpenSBI `fw_jump.bin`, no display.
  DQIB is used because it uses the same `linux-image-riscv64` kernel package
  the real build will use. Validate: reaches a shell; `dmesg` shows
  storage/NIC drivers bind without probe failures; `ip addr` gets a DHCP
  lease from RVVM's usermode NAT; `ping`/`curl`/`apt update` succeed.
- **0b — Display + HID.** Same image, RVVM GUI window enabled, framebuffer
  console only (no Xorg yet). Validate: window renders console output;
  keyboard input reaches a getty prompt.
- **Go/no-go:** GO if both pass with no more than routine
  kernel-cmdline/module tweaking, and boot-to-shell time is tolerable for a
  classroom (measured, not assumed). NO-GO if the storage/NIC/framebuffer/HID
  path fundamentally doesn't work.
- **Fallback seam if NO-GO:** swap `launcher/rvvm.go` (CLI syntax) and
  `image-build/chroot-config/initramfs-modules.conf`
  (virtio_blk/virtio_net/virtio_gpu/virtio_input instead of nvme/r8169) to
  QEMU. The debootstrap pipeline, XFCE curation, autologin config, and
  packaging/launcher logic are emulator-agnostic and stay as-is.

## Guest image build pipeline

Built via cross-arch debootstrap + qemu-user-static chroot on a Linux/WSL
box — not an interactive install inside the emulator — to avoid paying
unaccelerated riscv64 boot time on every image iteration, and to keep the
build fully scriptable.

- `debootstrap --arch=riscv64 --variant=minbase --foreign trixie ...`,
  second-stage via `qemu-riscv64-static` in a chroot/`systemd-nspawn`.
- Install: `linux-image-riscv64`; curated XFCE (`xfce4` core +
  `xfce4-terminal` + `mousepad`, skipping `xfce4-goodies`, printing,
  Bluetooth, media players — no browser); `openssh-server`, `curl`,
  `iputils-ping`.
- Disable the XFCE compositor by default (weak/emulated hardware).
- Networking via `systemd-networkd` with plain DHCP.
- Autologin via `lightdm` (`autologin-user=student`, in `sudo` group — "no
  admin rights" refers to the Windows host, not the guest).
- `initramfs-tools`: start with `MODULES=most`; prune once Phase 0/1 confirm
  exactly which modules are needed.
- Disable `apt-daily*.timer`, `bluetooth`, `cups`, `ModemManager`,
  `avahi-daemon`, `man-db.timer`.
- Image format: single raw ext4 filesystem, no partition table, no
  bootloader (RVVM's `-k` direct-kernel-boot skips U-Boot/GRUB entirely).
  Compress the shipped copy with `zstd -19`/`xz -9`.
- No inbound SSH port-forwarding by default (not a stated requirement) — kept
  as an opt-in config flag.

## Packaging into the single exe

- Launcher: a compiled Go executable (not NSIS/Inno/7z SFX) — the runtime
  needs real logic (per-run scratch copy, crash-cleanup sweep, child-process
  lifecycle) that installer tools aren't shaped for.
- Payload too large for `go:embed`; concatenate the compiled launcher with a
  payload container plus a trailing footer the launcher parses via seek.
- Runtime flow: locate payload via footer → extract static tool artifacts to
  a cached `%LOCALAPPDATA%` tools dir → decompress a fresh copy of the
  pristine disk image into a new per-run scratch dir every launch → launch
  RVVM as a child process with conservative, overridable defaults → on exit,
  delete the per-run scratch dir → on every launch, sweep stale scratch dirs
  from crashed prior runs.
- No admin rights: only touches `%LOCALAPPDATA%`, spawns a plain child
  process, no registry/service/`Program Files` writes.
- Ship `THIRD_PARTY_NOTICES/` for RVVM (GPL-3.0/MPL-2.0), OpenSBI (BSD-2), and
  Debian's kernel packages (GPL-2.0). Record the pinned RVVM nightly-artifact
  commit hash in the manifest.
- Expect a Windows SmartScreen warning (unsigned exe) and possibly a Windows
  Firewall prompt (RVVM's usermode networking) on first run — documented for
  instructors, not blockers.

## Verification plan

1. Phase 0 spike scripts run on an actual no-nested-virt Windows VM matching
   the lab profile; record pass/fail and boot-time in
   `docs/phase0-spike-results.md`.
2. After the full image build: launch the packaged exe on the same VM
   profile, confirm autologin into XFCE4, run `curl`/`ping`/`apt update`.
3. Close and relaunch; confirm identical environment, no admin prompt or
   driver install needed.
4. Measure and document a realistic "cold start to usable desktop" time.
