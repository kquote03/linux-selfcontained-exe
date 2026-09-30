# Third-party notices

This directory holds the license/attribution texts for third-party binaries
bundled inside the packaged exe. Populated by `packaging/package.ps1` from
the pinned sources recorded in `packaging/payload-manifest.json`.

- **RVVM** (https://github.com/LekKit/RVVM) — dual licensed: GPL-3.0 for the
  `rvvm-cli` end-user binary we bundle and run as an external process,
  MPL-2.0 for the `librvvm` core. Attribution + link to source required.
- **OpenSBI** (https://github.com/riscv-software-src/opensbi) — BSD-2-Clause.
- **Linux kernel** — the guest's `linux-image-riscv64` is Debian's own
  unmodified kernel package (GPL-2.0); source is available via Debian's
  standard source package infrastructure, not vendored here.
