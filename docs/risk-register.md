# Risk register

| Risk | Status | Notes |
|---|---|---|
| RVVM's NVMe/RTL8169 device model may not be reachable by a stock Debian riscv64 kernel/initramfs | **Open — gating Phase 0** | Debian's `initramfs-tools` default (`MODULES=most`) plausibly covers this, but no one has documented it working. Must confirm via `spike/boot-test.ps1` before the full image build. |
| RVVM's exact `-k`/initrd/cmdline flag semantics for direct kernel boot | **Open — gating Phase 0** | Confirmed flags: `-i/-image` (NVMe default, or `-nvme`/`-ata`), `-k/-kernel`, `-m`, `-s`, `-res`, `-nogui`, `-nonet`, `-portfwd`. Initrd-passing and cmdline flag names were not confirmed by documentation research; must be found empirically (`rvvm --help` / source) during the spike. |
| RVVM has no pinned stable release; v0.6 is explicitly marked superseded by the maintainer | **Accepted, mitigated** | Use a specific nightly CI artifact, pinned by commit hash in `packaging/payload-manifest.json`. Never track "latest". |
| RVVM is a small, largely one-maintainer project with thin docs | **Accepted** | QEMU is the documented fallback (see `docs/architecture.md` "Fallback seam"). Swap is scoped to `launcher/rvvm.go` + `image-build/chroot-config/initramfs-modules.conf` only. |
| Full XFCE4 desktop performance under pure software CPU emulation on genuinely weak host hardware | **Open** | Compositor disabled by default. No hard performance target set yet; must be measured on representative hardware and documented in `docs/phase0-spike-results.md` / a later perf note. |
| Unsigned multi-GB exe triggers Windows SmartScreen; RVVM's usermode networking may trigger a Windows Firewall prompt | **Accepted** | Out of scope to code-sign for now. Document for instructors as an expected click-through. |
| Large binary assets (disk image, RVVM binary, kernel, firmware) must not bloat the git repo | **Mitigated** | Never committed; `.gitignore`'d. Rebuilt deterministically from `packaging/payload-manifest.json` (pinned URLs + SHA256) via `fetch-*.sh` scripts. |
