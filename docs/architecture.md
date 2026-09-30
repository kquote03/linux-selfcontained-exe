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

**Status: PASSED — GO.** See `docs/phase0-spike-results.md` for full results
and `docs/phase0-evidence/` for screenshots. Summary: boot chain, NVMe
storage, RTL8169 networking (DHCP + real outbound internet via ping/curl),
GUI display, and keyboard input were all confirmed working on RVVM nightly
`v0.7-git-gce8ca7c` with no workarounds needed. The earlier "no initrd flag"
concern turned out not to apply, since this project boots a fully-installed
OS via `fw_payload.bin` (OpenSBI+U-Boot) + extlinux, which loads the initrd
itself — the same as a normal PC boot chain. RVVM remains the emulator; the
QEMU fallback seam below is kept as documentation but is not expected to be
needed.

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

**Status: PASSED — full XFCE4 desktop confirmed rendering.** See
`docs/phase1-spike-results.md` and `docs/phase1-evidence/`. Two real
problems were found and fixed along the way: a plymouth boot-splash hang
(fixed by removing plymouth — not needed for a kiosk appliance), and a
missing `CONFIG_DRM_SIMPLEDRM`/`CONFIG_SYSFB_SIMPLEFB` in Debian's stock
kernel, which meant Linux never took over the display U-Boot hands off
(fixed by cross-compiling a matching-version kernel with those options
enabled — see `image-build/build-kernel.sh`). Both fixes are now baked into
the scripted pipeline below, not manual steps.

Built via cross-arch debootstrap + qemu-user-static chroot on a Linux/WSL
box — not an interactive install inside the emulator — to avoid paying
unaccelerated riscv64 boot time on every image iteration, and to keep the
build fully scriptable. Run in order: `image-build/build-rootfs.sh` →
`image-build/build-kernel.sh` → `image-build/make-disk-image.sh`.

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

**Status: PASSED — built, packaged, and end-to-end tested.** `launcher/`
(Go source) + `packaging/package.ps1` produce a working single-file exe;
verified with a real (small, fake-content) payload: extraction, caching,
decompression integrity (checksum-verified), RVVM invocation, clean exit
cleanup, and stale-run sweep after a simulated crash all confirmed working.
Not yet re-tested with the real multi-GB disk image end-to-end (should be
mechanically identical - the code path doesn't care about payload size -
but worth doing once before distributing to students).

- Launcher: a compiled Go executable (not NSIS/Inno/7z SFX) — the runtime
  needs real logic (per-run scratch copy, crash-cleanup sweep, child-process
  lifecycle) that installer tools aren't shaped for. Built with
  `-ldflags="-s -w -H windowsgui"` (no console window; a native `MessageBox`
  in `launcher/winerr.go` reports fatal errors instead, since there's no
  console to print to).
- Payload too large for `go:embed`; `packaging/package.ps1` concatenates the
  compiled launcher with a zip payload (`tools/rvvm_x86_64.exe`,
  `tools/librvvm.dll`, `tools/fw_payload.bin`, `disk.img.zst`, `VERSION`)
  plus a 64-byte trailing footer (`launcher/payload.go` parses it via seek —
  magic bytes, payload offset/size, format version).
- Runtime flow (`launcher/main.go`): locate payload via footer → extract
  static tool artifacts to a cached `%LOCALAPPDATA%\LinuxLab\tools\<version>`
  dir (skipped if already present, keyed by the `VERSION` string baked into
  the payload) → sweep any stale `run-*` dirs left by a crashed prior launch
  (best-effort `RemoveAll`; a dir whose `disk.img` is still open by a live
  RVVM process simply fails to delete and is left for next time — relies on
  Windows' own file locking rather than reimplementing PID liveness checks)
  → decompress a fresh copy of the pristine disk image into a new per-run
  scratch dir every launch (zstd, via `github.com/klauspost/compress/zstd`)
  → launch RVVM as a child process with conservative, `config.ini`-overridable
  defaults (`launcher/config.go`) → on exit, delete the per-run scratch dir.
- No admin rights: only touches `%LOCALAPPDATA%`, spawns a plain child
  process, no registry/service/`Program Files` writes.
- Ship `THIRD_PARTY_NOTICES/` for RVVM (GPL-3.0/MPL-2.0), OpenSBI (BSD-2), and
  Debian's kernel packages (GPL-2.0). Pinned versions/commits recorded in
  `packaging/payload-manifest.json`.
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
