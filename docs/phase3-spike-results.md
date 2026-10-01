# Phase 3 spike results: tuning for weak target hardware (2-core / 2GB-RAM host)

Status: **PASS — all adopted changes verified by real boots under RVVM
(kernel-log evidence, guest-side terminal verification, and a full
packaged-exe end-to-end regression). Actual speedup magnitude on the
real 2-core/2GB target hardware was NOT measured this session — see
"What's not yet validated" below.**

## Summary

Round 2 fixed input and made a first performance pass, but the user then
ran the packaged exe on real deployment targets — dual-core VMs reporting
as a virtualized 7th-gen Xeon, with only 2GB total host RAM — and reported
it was "a bit too slow." This round researched RVVM-specific and
general low-RAM-Linux tuning, then applied and verified a set of changes
targeting that specific weak-hardware profile.

## Research findings

Two research passes fed this round: one audited every resource/perf-
relevant line already in this repo, one researched RVVM's own source/docs/
issues plus general low-RAM Linux+XFCE tuning. Key findings:

- **RVVM maintainer-confirmed issue** ([LekKit/RVVM#138](https://github.com/LekKit/RVVM/issues/138),
  still open): on hosts using a non-"performance" CPU frequency-scaling
  governor/power plan, RVVM's time-slicing assumption breaks and the guest
  slows down disproportionately. This is a host-OS setting, not something
  RVVM itself can fix.
- `-nojit` (interpreter-only mode) is documented by RVVM itself as "slow,
  for debug purposes" — never a win for a sustained desktop session.
- RVVM's own help text (`rvvm_x86_64.exe --help`, this project's exact
  pinned nightly build) shows `-smp` **defaults to 1**, not 2 — this
  project's prior default of 2 cores was already above RVVM's own
  conservative baseline. There is no RVVM documentation on `-smp 1` vs `-smp
  2` behavior on a genuinely 2-core host, so this needed an empirical test
  (see below), not an assumption either way.
- A `-jitcache` flag was mentioned in general RVVM source references found
  during research, but **does not exist in this project's actual pinned
  build** (`v0.7-git-gce8ca7c`) — confirmed directly against this build's
  own `--help` output before attempting to wire it in. Not added.
- Standard low-RAM Linux/XFCE levers (zram instead of disk swap, volatile
  journald, masking unused indexing/accessibility/power daemons, IO
  scheduler/readahead tuning for a virtual disk) are well-established and
  low-risk.
- Self-building RVVM with `USE_LOCK_DEBUG=0` (a confirmed on-by-default
  debug/locking overhead in RVVM's own build flags) would shave a small
  amount of overhead, but requires standing up a new Windows RVVM build
  toolchain — this project currently only consumes a pinned nightly CI
  artifact. **Deferred as future work, not attempted this round.**

## Changes made and verified

### 1. Guest RAM default: 2G → 1G

`launcher/config.go` — the user's explicit instruction, leaving roughly
1GB of host RAM for Windows and the emulator process itself on a 2GB-RAM
target machine.

### 2. `-smp` default: tested empirically, kept at 2

The plan going in assumed `-smp 1` might reduce thread contention on a
genuinely 2-core host, since each RVVM vCPU is a dedicated host thread that
also does its own inline JIT compilation. **Tested directly** (same
kernel/rootfs, same 1G RAM, only `-smp` changed):

- `-smp 1`: desktop rendered at the ~3-4 minute mark.
- `-smp 2`: desktop rendered at the ~60-90 second mark.

`-smp 2` was unambiguously faster to boot in this test, not slower — the
contention hypothesis didn't hold up against a real measurement. **Default
kept at 2** (`launcher/config.go`). This was tested on this session's dev
machine, not the actual 2-core target hardware, so the gap could narrow
somewhat on genuinely fewer host cores, but there's no evidence favoring
`-smp 1` at all, so defaulting to RVVM's more-capable config is the safer
choice absent that evidence. An instructor can still override via
`config.ini`.

### 3. `-nogpu` adopted by default

Tested with a full boot to desktop, same kernel/rootfs/RAM/smp as the
baseline, `-nogpu` added: the desktop rendered identically (wallpaper,
panel, icons, working mouse/keyboard) — confirming RVVM's "GPU" concept is
separate from the simple-framebuffer/`DRM_SIMPLEDRM` path this project's
display depends on. Added to `launcher/rvvm.go`'s fixed args to drop
RVVM's unused virtual GPU device.

### 4. Opt-in Windows "High performance" power plan

`launcher/config.go`/`launcher/rvvm.go` — new `SetHighPerfPowerPlan`
config key (`set_high_perf_power_plan` in `config.ini`), default `false`.
When enabled, best-effort runs `powercfg /setactive <High performance
GUID>` before launching RVVM, swallowing any failure (e.g. no admin
rights) rather than failing the launch. Directly addresses the
maintainer-confirmed RVVM#138 governor issue above. Kept **opt-in**
rather than forced on for every user, since it's a host-wide Windows
setting change — an instructor deploying to known-weak, dedicated lab
machines can turn it on; a student running this on their own personal
laptop shouldn't have their power plan silently changed.

### 5. Guest-side zram, volatile journald, IO scheduler tuning

`image-build/chroot-config/setup-system.sh` (+ `systemd-zram-generator`
added to the package list in `image-build/build-rootfs.sh`):

- **zram**: compressed RAM-backed swap (`/etc/systemd/zram-generator.conf`,
  zstd compression) instead of a disk-backed swap file — cheaper than swap
  against RVVM's emulated NVMe. **Verified live**: boot log shows
  `zram: Added device: zram0` and `Adding 485884k swap on /dev/zram0.
  Priority:100` (systemd-zram-generator's own default sizing for a 1GB
  guest). Guest-side `cat /proc/swaps` confirmed `/dev/zram0` active,
  485884 size, 0 used, priority 100.
- **journald volatile storage**: `Storage=volatile`, `RuntimeMaxUse=16M`
  in `/etc/systemd/journald.conf` — no disk I/O for logging. **Verified
  live**: `grep -E 'Storage|RuntimeMaxUse' /etc/systemd/journald.conf`
  inside the booted guest shows both values applied.
- **IO scheduler/readahead**: a udev rule sets `scheduler=none` and
  `read_ahead_kb=128` on the guest's virtual NVMe device — no real seek
  cost to optimize for on an emulated disk. **Verified live**:
  `cat /sys/block/nvme0n1/queue/scheduler` inside the booted guest shows
  `[none] mq-deadline` (none selected).
- **Service masking extended**: tracker-miner/tracker-extract,
  at-spi-dbus-bus, and packagekit units added to the existing Round 2 mask
  list (bluetooth/cups/ModemManager/avahi/NetworkManager etc.).
  **Verified live**: `systemctl list-units --state=running | grep -Ei
  'tracker|at-spi|packagekit'` inside the booted guest returned no output
  — none of these are running (either successfully masked, or never
  present on this minimal `xfce4` install in the first place; either way,
  nothing to disable further).

### 6. Kernel: zram support confirmed/enabled

`image-build/build-kernel.sh` — `CONFIG_ZRAM`, `CONFIG_ZSMALLOC`,
`CONFIG_CRYPTO_ZSTD` force-enabled as modules (zram isn't needed before
root mount, so module is fine, unlike the display/input drivers which must
be built-in). Added to the script's existing hard-fail verification block.

## Verification method

Built via the same pipeline as Round 2 (`build-rootfs.sh` →
`build-kernel.sh` → `make-disk-image.sh` → `compress-disk-image.sh`),
reusing the existing Round 2 rootfs/kernel-source build state via an
incremental update (installing the new package, re-running the updated
`setup-system.sh`, rebuilding the kernel with the zram config additions)
rather than a full from-scratch rebuild.

Four separate boot tests were run directly against `rvvm_x86_64.exe` (not
just the packaged exe) to isolate variables:

1. **Baseline regression** (1G RAM, `-smp 1`): booted to desktop, confirmed
   zram/journald/IO-scheduler/masked-services all correct via a guest-side
   terminal (Win32 `SendKeys`/mouse-click automation, same technique as
   Round 2's input verification).
2. **`-smp 2` comparison**: same image, only `-smp` changed — booted
   dramatically faster (~60-90s vs ~3-4 min).
3. **`-nogpu` experiment**: added on top of the `-smp 2` config — display
   rendered identically.
4. **Final end-to-end regression**: the actual packaged exe
   (`packaging/package.ps1` output, combining the rebuilt `launcher.exe`
   with the real RVVM binaries/firmware/new disk image) was cold-launched
   from scratch — progress bar displayed correctly during extraction, and
   the guest booted to a fully rendered XFCE4 desktop with all Round 3
   defaults active (1G RAM, 2 cores, `-nogpu`, `-nosound`).

All kernel-log evidence (I2C-HID input devices binding, networking coming
up, zram device detected) was re-confirmed intact alongside the new
changes — the Round 2 input fix was not regressed by this round's changes.

## What's not yet validated

- **Actual speedup magnitude on the real 2-core/2GB target hardware.**
  This session's dev machine is not a 2-core/2GB machine, so while every
  change here is verified for *correctness* (nothing broke, zram/journald/
  scheduler/service changes took effect, boot completes, display and
  input still work), the real-world performance improvement on the user's
  actual deployment targets has not been measured. The `-smp`
  A/B comparison in particular was run on this dev machine and may not
  generalize exactly to a genuinely 2-core host — though there's no
  evidence at all favoring `-smp 1`, so the default change (keeping 2) is
  the safer call either way. **This needs the user's own hands-on test on
  the actual target machines** — same pattern as Round 2's mouse-input
  caveat.
- **`SetHighPerfPowerPlan` / RVVM#138 governor fix** — implemented and
  builds cleanly, but not verifiable from this dev machine (there's no way
  to reproduce a throttled-governor scenario here to confirm the `powercfg`
  call actually changes RVVM's observed speed). The `powercfg
  /setactive` command itself is a standard, well-documented Windows
  mechanism; the risk here is low, but the actual benefit on real hardware
  is unverified. Opt-in via `config.ini`'s `set_high_perf_power_plan=true`.
- **Self-building RVVM with `USE_LOCK_DEBUG=0`.** Deferred — would require
  a new Windows RVVM build toolchain this project doesn't currently have;
  flagged as a possible future optimization, not attempted.
- **Boot-to-desktop timing in absolute terms on the dev machine is not
  directly comparable to Round 2's "~60-90s" figure**, since Round 2's
  number was measured at 2G RAM/2 cores and this round's equivalent
  (1G RAM/2 cores/`-nogpu`) wasn't independently re-timed outside of the
  `-smp` A/B comparison above (which used 1G RAM throughout). No regression
  was observed, but a precise like-for-like timing comparison wasn't done.

## Files changed this round

- `launcher/config.go` — RAM default 2G→1G; `Cores` default confirmed at 2
  (tested, not just left unchanged); new `JitCache`-less decision recorded
  (flag doesn't exist in this build); new `SetHighPerfPowerPlan` config key.
- `launcher/rvvm.go` — `-nogpu` added to fixed args; opt-in
  `setHighPerfPowerPlan()` best-effort `powercfg` call.
- `image-build/build-rootfs.sh` — `systemd-zram-generator` added to the
  package install list.
- `image-build/chroot-config/setup-system.sh` — zram-generator config,
  volatile journald, IO scheduler/readahead udev rule, extended service
  mask list (tracker/at-spi/packagekit).
- `image-build/build-kernel.sh` — `CONFIG_ZRAM`/`CONFIG_ZSMALLOC`/
  `CONFIG_CRYPTO_ZSTD` enabled as modules, with hard-fail verification.
- `docs/risk-register.md`, `docs/architecture.md` — updated to reflect
  these changes (see those files for specifics).
