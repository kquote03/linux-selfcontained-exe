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
  `iputils-ping`; `systemd-zram-generator` (compressed RAM-backed swap,
  Round 3 weak-hardware tuning).
- Disable the XFCE compositor by default (weak/emulated hardware).
- Networking via `systemd-networkd` with plain DHCP.
- Autologin via `lightdm` (`autologin-user=student`, in `sudo` group — "no
  admin rights" refers to the Windows host, not the guest).
- `initramfs-tools`: curated `MODULES=list` (just `nvme`+`ext4`, the only
  modules actually needed to mount root — NIC/input load post-boot via
  normal udev regardless of initrd contents) instead of Debian's default
  `MODULES=most`, which produced a 156MB initrd. See
  `docs/phase2-spike-results.md` for the before/after size and the
  fallback if a pruned boot ever fails (`MODULES=most` is a one-line
  revert, no kernel rebuild needed).
- Disable `apt-daily*.timer`, `bluetooth`, `cups`, `ModemManager`,
  `avahi-daemon`, `man-db.timer`, plus (Round 3) `tracker-miner-fs`/
  `tracker-extract`, `at-spi-dbus-bus`, `packagekit` if present.
- zram swap (~half of guest RAM, zstd-compressed), volatile `journald`
  storage (`Storage=volatile`, 16M cap), and `scheduler=none` + reduced
  readahead on the guest's virtual NVMe device — all Round 3 weak-hardware
  tuning, see `docs/phase3-spike-results.md`.
- Image format: GPT-partitioned single ext4 filesystem with an extlinux
  bootloader config (`image-build/make-disk-image.sh` writes
  `/boot/extlinux/extlinux.conf`). RVVM is invoked with `-i <image>` (not
  `-k`), so firmware/U-Boot/extlinux load the kernel+initrd exactly like a
  normal PC boot chain — matches the Phase 0 description above, not the
  bare direct-kernel-payload path. Compress the shipped copy with
  `zstd -19`.
- No inbound SSH port-forwarding by default (not a stated requirement) — kept
  as an opt-in config flag.

## Packaging into the single exe

**Status: PASSED — built, packaged, and fully end-to-end tested with the
real multi-GB image.** `launcher/` (Go source) + `packaging/package.ps1`
produce a working single-file exe. Verified twice: once with a small
fake-content payload (extraction, caching, checksum-verified decompression
integrity, RVVM invocation, clean-exit cleanup, stale-run sweep after a
simulated crash), and once for real - the actual 8GB XFCE4 disk image,
zstd-compressed to 1.25GB, packaged into a 1.28GB single exe, double-clicked
cold, and confirmed to boot all the way to the rendered XFCE4 desktop with
autologin. See `docs/phase1-evidence/05-final-packaged-exe-boot.png`. Rough
timing observed: ~1-2 minutes for first-run disk image decompression
(8GB output from a 1.25GB compressed payload) plus ~60-90s guest boot to
desktop - call it "a few minutes, give it time" for instructor-facing
expectations. This figure predates the Round 3/4 weak-hardware tuning
passes (`docs/phase3-spike-results.md`, `docs/phase4-spike-results.md`)
and was measured on a dev machine, not genuinely weak target hardware -
real-world timing on the actual deployment targets is still unmeasured.
Round 4 shrunk the shipped disk image from 8GB to 4GB (sized against
measured rootfs usage, ~2.2GB) after a real target machine's CPU-Z report
showed only ~12GB free disk on a representative deployment machine -
cuts per-launch decompression/disk-write time, though the *compressed*
`disk.img.zst` stayed about the same size (~1.07GB either way, since
zstd already compressed the old image's empty padding down to nearly
nothing - the real content dominates either way).

- Launcher: a compiled Go executable (not NSIS/Inno/7z SFX) — the runtime
  needs real logic (per-run scratch copy, crash-cleanup sweep, child-process
  lifecycle) that installer tools aren't shaped for. Built with
  `-ldflags="-s -w -H windowsgui"` (no console window; a native `MessageBox`
  in `launcher/winerr.go` reports fatal errors instead, since there's no
  console to print to).
- Payload too large for `go:embed`; `packaging/package.ps1` concatenates the
  compiled launcher with a zip payload (`tools/rvvm_x86_64.exe`,
  `tools/librvvm.dll`, `tools/fw_payload.bin`, `disk.img.zst`, `VERSION`,
  plus optionally `tools/fast/rvvm_x86_64.exe` + `tools/fast/librvvm.dll` -
  a CPU-targeted RVVM build added in Round 4, see
  `docs/phase4-spike-results.md`. Both RVVM variants are self-built from a
  pinned, patched source checkout via `image-build/build-rvvm.sh` (Round
  5 adds `image-build/patches/0001-win32-resizable-scaled-window.patch` -
  a drag-resizable, aspect-ratio-preserving scaled RVVM window, since the
  stock RVVM Win32 GUI backend only ever supported a fixed-size,
  non-resizable window; see `docs/phase5-spike-results.md`) rather than a
  downloaded prebuilt nightly artifact) plus a 64-byte trailing footer
  (`launcher/payload.go` parses it via seek — magic bytes, payload
  offset/size, format version).
- Runtime flow (`launcher/main.go`): locate payload via footer → extract
  static tool artifacts to a cached `%LOCALAPPDATA%\LinuxLab\tools\<version>`
  dir (skipped if already present, keyed by the `VERSION` string baked into
  the payload) → sweep any stale `run-*` dirs left by a crashed prior launch
  (best-effort `RemoveAll`; a dir whose `disk.img` is still open by a live
  RVVM process simply fails to delete and is left for next time — relies on
  Windows' own file locking rather than reimplementing PID liveness checks)
  → decompress a fresh copy of the pristine disk image into a new per-run
  scratch dir every launch (zstd, via `github.com/klauspost/compress/zstd`)
  → pick between the portable baseline RVVM build and a CPU-targeted one
  (`launcher/rvvm.go`'s `rvvmExePath`, gated on `launcher/cpufeatures.go`'s
  `fastRVVMSupported` - AVX2/FMA3/AES runtime detection via
  `golang.org/x/sys/cpu`, Round 4) → launch RVVM as a child process with
  conservative, `config.ini`-overridable defaults (`launcher/config.go`;
  RAM default 1G, 2 cores, 800x600 — tuned in Round 3/4 against a real
  target machine's measured hardware profile, see
  `docs/phase3-spike-results.md` and `docs/phase4-spike-results.md`, plus
  a fixed `-nosound`/`-nogpu` since this appliance has no audio/GPU
  requirement, plus an opt-in `set_high_perf_power_plan` flag and an
  `extra_args` passthrough for
  further RVVM flag tuning without a launcher rebuild) → on exit, delete
  the per-run scratch dir.
- No admin rights: only touches `%LOCALAPPDATA%`, spawns a plain child
  process, no registry/service/`Program Files` writes.
- Ship `THIRD_PARTY_NOTICES/` for RVVM (GPL-3.0/MPL-2.0), OpenSBI (BSD-2), and
  Debian's kernel packages (GPL-2.0). Pinned versions/commits recorded in
  `packaging/payload-manifest.json`.
- **Deploying on weak hardware** (Round 3): on genuinely dual-core lab
  machines, set `set_high_perf_power_plan=true` in `config.ini` (or
  manually switch Windows to the "High performance" power plan) — RVVM has
  a maintainer-confirmed issue where a throttled CPU governor
  disproportionately slows the guest (LekKit/RVVM#138). Defaults already
  target this profile (1G guest RAM, `-nogpu`); `cores` can be lowered from
  the default of 2 via `config.ini` if ever needed, though empirical
  testing in `docs/phase3-spike-results.md` found 2 cores booted
  noticeably faster than 1, even on a host with only 2 physical cores.
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
