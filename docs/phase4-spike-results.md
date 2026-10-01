# Phase 4 spike results: tuning against a real target machine's CPU-Z data

Status: **PASS — all changes verified by real boots (shrunk-image boot
test, resolution/zram checks, RVVM self-build A/B test, full packaged-exe
end-to-end regression). Actual speedup magnitude on the real target
machine was NOT re-measured this session — see "What's not yet
validated" below.**

## Summary

Round 3 tuned against a *guessed* hardware profile. The user then supplied
`cpuz-report.txt`, a real CPU-Z report from a representative deployment
machine, replacing that guess with ground truth. This round re-tuned
against the real profile and added a new lever (a CPU-targeted RVVM
self-build, auto-selected at runtime) that Round 3 had explicitly deferred.

## What the CPU-Z report actually showed

- **Hypervisor:** Xen HVM domU (BIOS vendor "Xen", non-UEFI, dated 2017).
  The real stack is Xen → Windows 10 → RVVM's RISC-V JIT - three layers.
- **CPU:** exactly 2 real cores (APIC topology confirms separate cores,
  not SMT threads of one core), host silicon Intel Xeon E5-2650 v4
  (Broadwell-EP). **Turbo Mode: not supported**, fixed 22x multiplier -
  the vCPU runs at a flat, non-boosting 2195 MHz with no visible P-state
  range.
- **Instruction sets exposed to the guest:** SSE4.2/AVX/AVX2/FMA3/AES/TSX
  present. **No BMI1/BMI2 listed** - masked by Xen even though the
  underlying Broadwell silicon has them.
- **RAM: ~6GB total** (CPU-Z Chipset "Memory Size 6136 MBytes", the live
  reading - not the stale DMI table's "5 GB", and not the ~2GB originally
  assumed in Round 3).
- **Disk: only ~12.4GB free** (50GB Xen PV disk at 25.4% available).
- **Display:** Cirrus Logic VGA - fully non-accelerated, software-only at
  every layer of the display chain.

## Changes made and verified

### 1. Disk image shrunk: 8GB → 4GB

Rebuilt (not resized in-place) from the unchanged Round 3 rootfs via
`image-build/make-disk-image.sh`'s new `IMG_SIZE` env var (default now
`4G`), sized against the actual measured rootfs usage (`du -sh
/root/build/rootfs` → **2.2G**) plus margin. **Verified by direct boot**:
the shrunk image boots clean to the XFCE4 desktop in ~70s, no regression.

**Correction to the original plan's expectation**: shrinking the raw
image size did *not* meaningfully shrink the *compressed* `disk.img.zst`
(still ~1.07GB, same as the old 8GB image's compressed size) - zstd
compresses the mostly-empty padding in the old 8GB image down to nearly
nothing already, so the compressed size is dominated by the ~2.2GB of
real rootfs content either way. The real, confirmed benefit is **per-
launch decompression/disk-write time and scratch-disk footprint** (4GB
written per launch instead of 8GB), not download size - worth doing given
the ~12GB-free disk constraint, but the win is narrower than first assumed.

### 2. Resolution default: 1024x768 → 800x600

Given the fully non-accelerated Cirrus Logic VGA display chain, fewer
pixels means less software blit work. **Visually verified**: booted to
desktop at 800x600, confirmed XFCE's panel/icons/app menu/dock all render
and operate correctly with no layout breakage.

### 3. RAM default: kept at 1G (not raised)

Round 3 set 1G assuming only ~2GB host RAM; with ~6GB confirmed, there was
room to consider raising it. **Tested, not assumed**: at an idle XFCE
desktop with the 1G default, `cat /proc/swaps` inside the guest showed
**zram at 0B used** (`free -h`: 949Mi total, 289Mi used, 659Mi available).
There's no swap pressure at idle to relieve by raising the default, so 1G
stays - an instructor whose students run heavier workloads (the Round 2
note that Firefox ran "albeit slowly" at 2G) can still raise it via
`config.ini`.

### 4. RVVM self-build with CPU-targeted flags, auto-selected at runtime

Previously deferred in Round 3 as future work; built this round.

- Installed `mingw-w64` in WSL (`apt-get install mingw-w64` - not
  unusual, no special toolchain acquisition needed).
- Cloned RVVM at the **exact same pinned commit** already used for the
  portable build (`ce8ca7c00ba4058e5f26811057573b3ff23e9316`), so this is
  purely a compiler-flags experiment, not a code change.
- Built via `make CC=x86_64-w64-mingw32-gcc CFLAGS="-mavx -mavx2 -mfma -maes -msse4.2" USE_LOCK_DEBUG=0`.
  Deliberately **did not** use a named `-march=haswell`/`-march=broadwell`
  preset - both bake in BMI1/BMI2 by default, which the real target
  machine's CPU-Z report shows are masked. A preset would have compiled
  in instructions that SIGILL-crash on that exact machine. Hand-picking
  only the confirmed-present extensions avoids that.
- **Ships both binaries in the same exe, chosen at runtime** - not
  distributed as a separate download. `launcher/cpufeatures.go` checks
  `cpu.X86.HasAVX2 && HasFMA3 && HasAES` (via `golang.org/x/sys/cpu`);
  `launcher/rvvm.go`'s `rvvmExePath()` picks `tools/fast/rvvm_x86_64.exe`
  when true and present, otherwise falls back to the portable
  `tools/rvvm_x86_64.exe`. This is strictly safer than a separate "fast"
  exe a student could download onto the wrong machine - the single exe
  self-adapts and can never crash from a CPU mismatch. Both binaries sit
  in separate subdirectories (`tools/` and `tools/fast/`) since both
  export an identically-named `librvvm.dll` that Windows resolves from
  the launching exe's own directory - they can't share a folder.
- **Verified it actually runs**: no immediate crash on the real test
  machine profile's confirmed instruction set, booted clean to desktop.
- **Measured, not assumed, whether it helps**: a controlled one-shot A/B
  (same disk image, same RAM/cores/flags, only the RVVM binary differed),
  polling each run's serial log for the "login:" prompt as a precise,
  reproducible milestone:

  | | Wall time to login | CPU time consumed |
  |---|---|---|
  | Stock (portable nightly) | 41.34s | 51.09s |
  | Fast (AVX2/FMA3/AES, `USE_LOCK_DEBUG=0`) | 41.53s | 49.67s |

  Wall-clock time is effectively unchanged (within measurement noise).
  CPU time is about **3% lower** for the optimized build - a real,
  measured improvement, but a modest one, not dramatic. Plausibly mostly
  attributable to `USE_LOCK_DEBUG=0` (a confirmed on-by-default
  locking/debug overhead in RVVM's own build config) rather than the
  AVX2/FMA3 codegen itself, since boot exercises relatively little
  FP-heavy code. **Adopted anyway**: shipping it costs under 1MB of
  package size and carries zero compatibility risk (runtime-detected,
  safe fallback), so even a modest, mostly-free CPU-time win is worth
  keeping. Interactive/sustained-use workloads were not benchmarked and
  may show a different (larger or smaller) effect than this boot-only
  measurement.

### 5. Documented: the power-plan flag may do little on Xen HVM profiles like this one

Round 3's opt-in `set_high_perf_power_plan` flag addresses RVVM's
maintainer-confirmed governor-throttling issue
([LekKit/RVVM#138](https://github.com/LekKit/RVVM/issues/138)). This
machine's CPU-Z data shows "Turbo Mode: not supported" with a single fixed
multiplier - strongly suggesting the vCPU's frequency is already pinned
by the Xen host (dom0), not scaled by Windows' own ACPI power plan at all
on this profile. The flag is kept (still potentially useful on other
deployment profiles with real P-state ranges), but expectations on a Xen
HVM guest like this one should be tempered - there may be no guest-visible
frequency range to toggle between in the first place.

## What's not yet validated

- **Actual speedup magnitude on the real target machine.** Every change
  this round was verified for *correctness* on this dev machine (boots
  clean, zram/resolution/binary-selection all behave as expected, no
  crashes), but the dev machine isn't the Xen HVM target profile itself.
  The RVVM self-build's modest ~3% CPU-time improvement was measured on
  dev hardware, not confirmed on the real 2-core Xen guest. **Needs the
  user's own hands-on test**, same pattern as every prior round's caveat.
- **Interactive/sustained-use RVVM self-build comparison.** Only a boot-
  to-login-prompt A/B was measured; real desktop/app usage wasn't
  benchmarked and could show a different-sized effect.
- **The power-plan flag's actual effect**, on this or any other profile -
  still unverified either way, just better-documented as to why it might
  not matter here specifically.

## Files changed this round

- `image-build/make-disk-image.sh` - `IMG_SIZE` env var, default `4G`
  (was hardcoded `8G`).
- `launcher/config.go` - `Resolution` default `800x600` (was `1024x768`);
  `RAM` default re-confirmed at `1G` with evidence, not just left alone.
- `launcher/cpufeatures.go` (new) - `fastRVVMSupported()` CPU-feature
  check via `golang.org/x/sys/cpu`.
- `launcher/rvvm.go` - `rvvmExePath()` picks the fast or stock RVVM
  binary at runtime.
- `launcher/extract.go` - `optionalToolFiles` for the fast-build entries
  (non-fatal if absent from an older/test payload); extraction preserves
  the `fast/` subdirectory structure instead of flattening to basenames.
- `packaging/package.ps1` - optional `-RvvmFastExe`/`-RvvmFastDll`
  parameters, embedded at `tools/fast/` in the payload zip.
- `packaging/payload-manifest.json` - new `rvvmFast` entry documenting
  the self-built variant's commit, CFLAGS, and measured A/B result.
- `docs/risk-register.md`, `docs/architecture.md` - updated to reflect
  the new disk image size, resolution/RAM defaults, and dual-RVVM setup.
