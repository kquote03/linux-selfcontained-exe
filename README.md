# Self-contained Linux teaching lab exe

A single Windows `.exe` that boots a Debian 13 ("trixie") riscv64 guest with an
XFCE4 desktop, no installation, no admin rights, and no prior setup — built to
let an entire lab of students try Linux without walking through an OS install.

- **Guest:** Debian 13 riscv64 + XFCE4 (minimal set), autologin, internet access.
- **Emulator:** [RVVM](https://github.com/LekKit/RVVM), chosen over QEMU — see
  `docs/architecture.md` for why, and `docs/risk-register.md` for the tradeoffs
  accepted by that choice.
- **Host requirement:** any 64-bit Windows PC (SSE2 baseline). No hardware
  virtualization is used or required — the target hosts are themselves VMs
  without nested virtualization exposed, so everything runs under software
  CPU emulation regardless of guest architecture.
- **Session model:** ephemeral. Every launch boots an identical, pristine
  environment; nothing persists after the window is closed.

## Status

**Working end-to-end.** A packaged exe (launcher + RVVM + firmware + the
built Debian/XFCE4 disk image) has been built and confirmed, on this
project's real dev machine (no hardware virtualization), to boot cold from
a double-click all the way to an autologin'd, rendered XFCE4 desktop. See
`docs/phase0-spike-results.md` (emulator feasibility), `docs/phase1-spike-results.md`
(guest image + the display-driver fix that was needed), and
`docs/architecture.md`'s "Packaging into the single exe" section for the
full picture, including what's still open (mouse/keyboard interaction
inside the running desktop hasn't been confirmed by a human with real
hardware input; performance on genuinely weak target hardware is
unmeasured).

To build it yourself: `image-build/build-rootfs.sh` →
`image-build/build-kernel.sh` → `image-build/make-disk-image.sh` →
`image-build/compress-disk-image.sh`, then build `launcher/` for
`windows/amd64` and run `packaging/package.ps1`. See the scripts' own
comments and `docs/architecture.md` for details.

## Repo layout

- `launcher/` — Go source for the runtime launcher exe (extraction, ephemeral
  scratch-copy management, launching the emulator, cleanup).
- `image-build/` — scripts to build the customized Debian riscv64 + XFCE4
  disk image, run on Linux/WSL (not at student runtime).
- `packaging/` — scripts that concatenate the launcher + payload into the
  final single distributable exe, plus the pinned-version manifest.
- `spike/` — Phase 0 feasibility spike scripts (boot/network/display/HID
  validation on RVVM before the full image build).
- `docs/` — architecture notes, risk register, spike results.
- `THIRD_PARTY_NOTICES/` — license/attribution texts for bundled third-party
  binaries (RVVM, OpenSBI, Debian's kernel packages).

Large binaries (disk images, RVVM binary, kernel, firmware) are never
committed to git — see `packaging/payload-manifest.json` and the `fetch-*.sh`
scripts, which pin exact versions/hashes and rebuild them deterministically.
