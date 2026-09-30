# Phase 2 spike results: keyboard/mouse fix + performance pass

Status: **PASS — keyboard and mouse confirmed working via kernel-log and
UI-automation evidence; disk image and initramfs pruning validated by a real
boot. Two items (the `-nogpu` experiment and a full visual regression of the
final packaged exe) were not completed this session — see "What's not yet
validated" below.**

## Summary

Round 1 shipped a working display but non-functional keyboard/mouse inside
the running XFCE4 session, plus a heavier-than-necessary boot (156MB
initrd, several security-hardening kernel features irrelevant to a
single-user ephemeral teaching VM). This round root-caused and fixed the
input bug — through two wrong hypotheses before finding the real cause —
and made several performance changes, validating each with real boots
under RVVM rather than assumption.

## The input bug: two wrong hypotheses, then the real cause

### Hypothesis 1 (wrong): synthetic-input-testing limitation

Round 1's `docs/phase1-spike-results.md` guessed that `SetCursorPos`/
`mouse_event` and `Alt+F2` simply didn't work well as *test* automation
against RVVM, since RVVM reads keyboard via real Win32 window focus, not
stdin. Plausible-sounding, but never actually verified — the one keyboard
interaction that had worked (typing a login password) was on a completely
different kernel (Phase 0's prebuilt Arch test image), never carried into
this project's own custom kernel build.

### Hypothesis 2 (wrong): missing Goldfish input driver

Investigating for Round 2, `goldfish_rtc` was confirmed working (RTC clock
sets correctly at boot), and the kernel source tree has
`drivers/input/keyboard/goldfish_events.c` — the Android-emulator "Goldfish"
family's generic input driver. The natural (but wrong) inference: RVVM's
keyboard/mouse must be another Goldfish device, and the built kernel had
`CONFIG_KEYBOARD_GOLDFISH_EVENTS` unset. Enabled it, rebuilt the kernel,
rebooted — no goldfish_events bind message ever appeared in dmesg, and
keyboard/mouse still didn't work. Wrong.

### The real cause: HID-over-I2C, not Goldfish at all

Rather than guess a third time, dumped RVVM's actual generated device tree:

```
rvvm_x86_64.exe fw_payload.bin -i disk.img -m 2G -smp 2 -nogui -dumpdtb dump.dtb
dtc -I dtb -O dts dump.dtb
```

The device tree contains **no Goldfish input node at all**. Instead:

```dts
i2c@10030000 {
    compatible = "opencores,i2c-ocores";
    ...
    i2c@8 { compatible = "hid-over-i2c"; ... }   /* keyboard */
    i2c@9 { compatible = "hid-over-i2c"; ... }   /* mouse */
    i2c@a { compatible = "hid-over-i2c"; ... }   /* mouse */
};
```

RVVM exposes keyboard/mouse as three **HID-over-I2C** devices on an
OpenCores I2C controller. Checking the kernel's `drivers/hid/i2c-hid/Kconfig`
confirmed the exact gap: Debian's stock config has `CONFIG_I2C_OCORES=m`,
`CONFIG_HID=m`, `CONFIG_I2C_HID=m` — but **`CONFIG_I2C_HID_OF` was never
enabled**. That's the specific glue driver that binds an I2C-HID *core*
transport to a device-tree-described `hid-over-i2c` node (its sibling,
`CONFIG_I2C_HID_ACPI`, does the same for ACPI-described devices — irrelevant
here, since ACPI is disabled on this platform: `dmesg` shows
`ACPI: Interpreter disabled.`). Without `I2C_HID_OF`, none of the three
input devices ever bind, no matter what state `I2C_HID`/`I2C_OCORES`/`HID`
are in.

**Fix** (`image-build/build-kernel.sh`): enabled, all built-in (not
modules, for the same reason as `DRM_SIMPLEDRM` — the lightdm greeter needs
keyboard/mouse immediately, with no modprobe/udev timing dependency):

```
CONFIG_I2C_OCORES=y
CONFIG_HID=y
CONFIG_HID_GENERIC=y
CONFIG_I2C_HID=y
CONFIG_I2C_HID_OF=y
```

`CONFIG_KEYBOARD_GOLDFISH_EVENTS` (the wrong Round-2-interim fix) was
removed from the enable list — it's harmless to leave compiled in, but
doesn't correspond to anything RVVM actually exposes, so keeping it would
mislead future readers of the build script.

### Verification

**Kernel-level (dmesg), independent of any UI automation:**

```
[   28.926335] input: hid-over-i2c 0001:0001 as .../0-0009/.../input1
[   28.933548] input: hid-over-i2c 0001:0001 as .../0-0008/.../input2
[   28.940065] input: hid-over-i2c 0001:0001 as .../0-000a/.../input0
[   28.988889] hid-generic ...: input,hidraw0: I2C HID v1.00 Mouse ... on 0-000a
[   29.000287] hid-generic ...: input,hidraw1: I2C HID v1.00 Mouse ... on 0-0009
[   29.237734] hid-generic ...: input,hidraw2: I2C HID v1.00 Keyboard ... on 0-0008
```

All three devices (2 mice, 1 keyboard) bind successfully at boot.

**UI-automation level**, against the live XFCE4 desktop, using Win32
`SetForegroundWindow` + `SetCursorPos`/`mouse_event`/`SendKeys` targeted at
the RVVM window (screenshots in `docs/phase2-evidence/`):

1. `01-mouse-click-cursor-moves.png` — clicking near the top-left panel
   moves the guest cursor to the click coordinates (previously, before the
   fix, the cursor never moved at all).
2. `02-mouse-doubleclick-icon-selected.png` — a single click on the "Home"
   desktop icon produces XFCE's selection/info tooltip ("Home / Size: 4.1
   kB / Last modified..."); a follow-up double-click opens a Thunar file
   manager window.
3. `03-keyboard-altf2-typed-xfce4-terminal.png` — `Alt+F2` opens XFCE's
   Application Finder dialog, and typing `xfce4-terminal` appears correctly
   in the text field.
4. `04-keyboard-enter-launched-terminal.png` — pressing `Enter` launches
   `xfce4-terminal`, landing at a working shell prompt
   (`student@linux-lab:~$`).

This is strictly stronger evidence than Round 1 had: both an independent
kernel-level signal and a full mouse+keyboard UI interaction chain, not
just a single login-screen keystroke test.

**Mouse note:** even with this confirmed, real physical-mouse behavior on
a student's own machine should still get a quick human sanity check the
first time this ships to a classroom — synthetic Win32 mouse injection and
a live human moving a real USB mouse are not perfectly identical input
paths, even though both are confirmed working here.

## Performance changes

All changes applied together in the same kernel rebuild as the input fix
(`image-build/build-kernel.sh`, `image-build/make-disk-image.sh`,
`image-build/chroot-config/setup-system.sh`):

1. **Kernel-level hardening removed** (not needed for a single-user
   ephemeral teaching VM with no adversarial threat model):
   `CONFIG_AUDIT`, `CONFIG_IMA`, `CONFIG_EVM`, `CONFIG_SECURITY_APPARMOR`,
   `CONFIG_FTRACE`, `CONFIG_KPROBES` — all confirmed disabled in the built
   `.config` (`# CONFIG_X is not set`).
2. **`mitigations=off`** added to the kernel command line
   (`make-disk-image.sh`'s `extlinux.conf` `APPEND` line) — disables
   Spectre/Meltdown-class CPU mitigations, irrelevant on a single-tenant
   emulated CPU with no cross-VM isolation boundary to protect.
3. **Initramfs pruned from `MODULES=most` to a curated list**
   (`nvme`+`ext4`, which pulls in `jbd2`/`mbcache` automatically).
   **Before:** 156MB. **After: 18,026,016 bytes (~17.2 MiB)** — roughly an
   88% reduction, all of it decompressed and loaded fresh on every single
   student launch.

   **Validated by an actual boot**, not just built: with the pruned
   initramfs, `dmesg` shows the NVMe controller probing, root mounting
   cleanly (`EXT4-fs (nvme0n1p1): mounted filesystem ... r/w with ordered
   data mode`), and the boot proceeding all the way to a rendered XFCE4
   desktop with working input (see verification evidence above). **Adopted
   as the default — not reverted.** If a future kernel/rootfs change ever
   needs a module this list doesn't cover, the one-line fallback described
   in `setup-system.sh`'s comment (revert to `MODULES=most`, re-run
   `update-initramfs`, no kernel rebuild needed) still applies.
4. **`-nosound`** added to `launcher/rvvm.go`'s fixed RVVM launch args — no
   stated audio requirement anywhere in this project, and this flag was
   already confirmed to exist via `rvvm_x86_64.exe --help`.
5. **`extra_args` config key** added (`launcher/config.go`, `config.ini`) —
   an optional, whitespace-split string appended to RVVM's argument list
   after the fixed flags, letting an instructor experiment with further
   RVVM tuning (e.g. `-jitcache`) without a launcher rebuild. Purely
   additive; doesn't change any current default behavior.

## What's not yet validated

- **The `-nogpu` flag experiment.** The plan called for testing whether
  `-nogpu` (confirmed to exist via `--help`) is compatible with this
  project's simple-framebuffer display path, and adopting it as a default
  only if the display still renders with it on. **Not performed this
  session** — ran out of safe opportunities to launch another RVVM instance
  before hitting a host memory constraint (see below). `launcher/rvvm.go`
  does **not** currently pass `-nogpu`; this remains a possible future
  optimization, not a regression.
- **A full visual regression check of the final packaged exe.** The
  packaged exe (`LinuxLab-r2.exe`, built via `packaging/package.ps1` with
  the new launcher + new kernel/rootfs + `disk.img.zst`) was cold-launched
  once and confirmed to extract its payload and launch `rvvm_x86_64.exe`
  successfully (a visible window appeared) — this validates the
  launcher/packaging mechanics (zip extraction, zstd decompression, footer
  parsing, argument passing) end-to-end with the new binaries. The RVVM
  process was killed by the host's own low-memory protection before a
  desktop screenshot could be captured from *this specific launch*.
  This is a narrower gap than it sounds: the identical `disk.img.zst` that
  went into this package was independently and fully verified moments
  earlier — direct RVVM invocation, same kernel, same rootfs, boot to a
  rendered desktop with confirmed keyboard+mouse (the evidence in
  `docs/phase2-evidence/`). What's unverified is only the *packaging step
  itself* combined with a full visual boot, not the guest image's
  correctness. Should be re-run (a plain double-click launch of the final
  exe, watched to the desktop) when memory headroom allows — see the dev
  machine note below.
- **Boot-to-desktop timing on genuinely weak/representative hardware.**
  Same caveat as Phase 1: this session's dev machine isn't representative
  of target lab hardware. No new timing measurement was taken this round.
- **Dev machine is memory-constrained (7.9GB total RAM).** Running WSL
  kernel builds, a `~1.1GB` packaged exe extracting an 8GB disk image, and
  RVVM's own guest RAM allocation concurrently pushed the host into a
  low-memory condition twice this session, triggering Claude Code's
  background-process low-memory protection (killed a background poll, then
  later killed the packaged-exe test process itself). This is a host
  resource constraint, not a defect in the shipped product — a typical lab
  PC most students would run this on is not expected to be this tight, but
  it's worth keeping in mind when re-running verification on this
  particular dev machine: avoid running a WSL kernel build and an RVVM
  boot test at the same time.

## Files changed this round

- `image-build/build-kernel.sh` — I2C-HID chain enabled built-in
  (superseding the earlier, incorrect Goldfish-driver attempt);
  AUDIT/IMA/EVM/APPARMOR/FTRACE/KPROBES disabled; verification checks
  updated to match.
- `image-build/make-disk-image.sh` — `mitigations=off` added to the kernel
  cmdline.
- `image-build/chroot-config/setup-system.sh` — `MODULES=list` with
  `nvme`+`ext4` instead of `MODULES=most`.
- `launcher/rvvm.go` — `-nosound` added to fixed args; optional
  `ExtraArgs` appended after them.
- `launcher/config.go` — new `extra_args` config key.
- `docs/risk-register.md`, `docs/phase1-spike-results.md` — corrected to
  reflect the real root cause (this file supersedes the interim,
  incorrect Goldfish diagnosis recorded mid-session in those files).
