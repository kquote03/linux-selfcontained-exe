# Phase 0 spike results

Status: **PASS — GO**

Run live on this project's own dev machine (a real Windows 11 host, x86_64,
no hardware virtualization used or available — matching the target profile),
using RVVM nightly build `v0.7-git-gce8ca7c` (Windows x86_64), the
maintainer-recommended `fw_payload.bin` (OpenSBI + U-Boot, from
[LekKit/archriscv-scriptlet](https://github.com/LekKit/archriscv-scriptlet/releases/download/2026.01/fw_payload.bin)),
and the prebuilt [Arch Linux RISC-V test image](https://github.com/LekKit/archriscv-scriptlet/releases/download/2026.01/archriscv-2026-01-07-4g.zip)
recommended in RVVM's own wiki (chosen for 0a/0b since it's a ready-made,
maintainer-validated generic-distro image — same boot chain shape as the
Debian image this project will build, without needing the full XFCE image
build first).

Command used: `rvvm fw_payload.bin -i archriscv-2026-01-07-4g.img -m 2G -smp 2 -res 1024x768 -portfwd tcp/127.0.0.1:2022=22`

## 0a — Boot + network

**Result: PASS**, confirmed twice (headless serial-console runs) plus once
more in the GUI run below.

- Boot chain: OpenSBI → U-Boot 2025.01 → U-Boot's own NVMe driver detects
  the attached disk (`Scanning nvme 0:1...`) → finds and parses
  `/boot/extlinux/extlinux.conf` → loads `/boot/vmlinuz-linux` +
  `/boot/initramfs-linux.img` (a completely standard modular kernel +
  initramfs pair) → kernel boots, mounts ext4 root on `nvme0n1p1` by UUID,
  reaches a login prompt.
  - **This resolves the biggest open risk from the design research**: the
    earlier concern was that RVVM's CLI has no `-initrd` flag, so a stock
    modular kernel package (which needs an initramfs to load its storage
    driver before mounting root) might not boot. That concern only applies
    to the bare `-k`/direct-kernel-payload path. For a real installed OS
    booted via `fw_payload.bin` (OpenSBI+U-Boot), **U-Boot's own extlinux
    boot loader reads and hands off the initrd itself**, exactly like GRUB
    would on a normal PC — RVVM's CLI never needs to know about the initrd
    at all. Debian's stock `linux-image-riscv64` package (modular, relying
    on `initramfs-tools`) will boot the same way. No custom kernel build is
    needed **for the boot/storage chain specifically**. (Update from Phase
    1: a custom kernel build turned out to be necessary anyway, for an
    unrelated reason — the stock kernel lacks the display driver needed for
    the GUI to render at all. See `docs/phase1-spike-results.md`.)
- Boot-to-login time: **~26 seconds** of kernel time (plus a few seconds of
  OpenSBI/U-Boot overhead) — well within a tolerable classroom wait, on pure
  software CPU emulation with no acceleration.
- Storage driver: NVMe bound and mounted without any probe failures.
- Network driver: NIC (`enp0s1`, RTL8169) came up automatically via DHCP —
  see `docs/phase0-evidence/05-dhcp-ip-addr-192-168-0-100.png` — no manual
  configuration needed.
- Outbound internet, from inside the guest:
  - `ping -c 3 8.8.8.8` → **3 packets transmitted, 3 received, 0% packet
    loss** (`docs/phase0-evidence/06-ping-8888-0pct-loss.png`).
  - `curl -sI https://deb.debian.org` → **`HTTP/2 200`**, real headers from
    Debian's actual mirror infrastructure — DNS resolution, TLS handshake,
    and HTTP/2 all working through RVVM's usermode NAT
    (`docs/phase0-evidence/07-curl-https-debian-org-http2-200.png`).
- Notes/issues hit: automating guest console interaction from this
  sandboxed shell environment was itself the main friction point (see
  below) — not a finding about RVVM or the target deployment.

## 0b — Display + HID

**Result: PASS.**

- RVVM's GUI window opened and correctly rendered the guest's framebuffer
  console (`docs/phase0-evidence/01-display-boot-login-prompt.png`).
- Keyboard input reaches the guest reliably **once the RVVM window has real
  Win32 foreground focus** — confirmed by typing the login username/password
  and shell commands via simulated keystrokes and seeing them echoed and
  executed correctly
  (`docs/phase0-evidence/02-hid-keyboard-username-typed.png`,
  `03-login-successful-root-shell.png`).
- **Tooling note (not a product risk):** RVVM's console/UART does not read
  guest input from a redirected process stdin/pipe/fifo — it reads real
  keyboard input via the GUI window, the same as any normal desktop
  application. Every attempt to script input through a named pipe piped
  into RVVM's stdin (the natural way to automate a CLI tool) silently did
  nothing; switching to sending actual Win32 keystrokes at the focused
  window worked immediately and reliably. This has zero bearing on the
  real deployment (a student will type into the actual window with actual
  keyboard focus, exactly like the working path here) — it only means
  future automated testing of this project should drive the GUI window
  (e.g. via SendKeys/UI automation, as done here) rather than piping into
  RVVM's stdin.

## Go/no-go decision

**Decision: GO — proceed with RVVM as planned, no fallback to QEMU needed.**

Rationale: both gating sub-spikes passed cleanly. The one risk flagged as
"biggest open risk" in the design phase (no discrete initrd flag in RVVM's
CLI) turned out not to matter for this project's actual boot path, since the
project always intended to use a fully-installed OS with a normal
bootloader, not a bare direct-kernel boot — and that path (`fw_payload.bin`
+ U-Boot + extlinux) handles the initrd exactly like a normal PC would.
Storage, networking, display, and HID all work out of the box with no
workarounds. `docs/risk-register.md` has been updated to reflect this.

Full XFCE4-specific behavior (compositor performance, Xorg framebuffer
driver, full desktop responsiveness) is not yet tested — that's normal
Phase 1/2 image-build work, not a Phase 0 gate, since Phase 0's job was only
to de-risk the storage/network/display/HID fundamentals before investing in
the full image build.
