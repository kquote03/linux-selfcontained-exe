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

Early scaffold — see `docs/phase0-spike-results.md` for the current
feasibility spike status before any full image build work proceeds.

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
