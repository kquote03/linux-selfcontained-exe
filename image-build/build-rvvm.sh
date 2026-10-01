#!/bin/bash
# Cross-compiles RVVM for Windows x86_64 via mingw-w64, from a patched
# checkout of the exact commit pinned in packaging/payload-manifest.json.
# Produces TWO variants, both self-built from the same patched source so
# both carry the Round 5 resizable/scaled-window fix
# (image-build/patches/0001-win32-resizable-scaled-window.patch):
#
#   release.windows.x86_64/        - portable baseline, no special CFLAGS,
#                                     works on any x86_64 Windows host.
#   release.windows.x86_64.fast/   - CPU-targeted (AVX2/FMA3/AES, no BMI1/
#                                     BMI2 - see docs/phase4-spike-results.md),
#                                     auto-selected at runtime by
#                                     launcher/cpufeatures.go when safe.
#
# Prerequisites (apt-get install on the WSL/Linux build host):
#   mingw-w64 git
#
# Run as a regular user or root. Idempotent: re-running reuses the
# existing clone and re-applies/re-checks the patch.
set -euo pipefail

RVVM_COMMIT="ce8ca7c00ba4058e5f26811057573b3ff23e9316"
SRC_DIR="${SRC_DIR:-/root/rvvm-src/RVVM}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PATCH_FILE="$SCRIPT_DIR/patches/0001-win32-resizable-scaled-window.patch"
OUT_DIR="${OUT_DIR:-/root/rvvm-build-out}"

if [ ! -d "$SRC_DIR" ]; then
  echo "=== Cloning RVVM ==="
  mkdir -p "$(dirname "$SRC_DIR")"
  git clone https://github.com/LekKit/RVVM.git "$SRC_DIR"
fi

cd "$SRC_DIR"
git fetch origin "$RVVM_COMMIT"
git checkout -f "$RVVM_COMMIT"
git clean -fdx

echo "=== Applying Round 5 win32 resizable/scaled-window patch ==="
patch -p1 --forward < "$PATCH_FILE"

# BUILDDIR defaults to $(BUILD_TYPE).$(OS).$(ARCH) regardless of CFLAGS -
# both variants would silently overwrite the same directory without an
# explicit, distinct BUILDDIR per build.
echo "=== Building portable baseline (no special CFLAGS) ==="
make CC=x86_64-w64-mingw32-gcc BUILDDIR=release.windows.x86_64.stock -j"${JOBS:-2}"

echo "=== Building CPU-targeted variant (AVX2/FMA3/AES, no BMI1/BMI2) ==="
make CC=x86_64-w64-mingw32-gcc BUILDDIR=release.windows.x86_64.fast \
     CFLAGS="-mavx -mavx2 -mfma -maes -msse4.2" USE_LOCK_DEBUG=0 -j"${JOBS:-2}"

mkdir -p "$OUT_DIR/stock" "$OUT_DIR/fast"
cp release.windows.x86_64.stock/rvvm_x86_64.exe release.windows.x86_64.stock/librvvm.dll "$OUT_DIR/stock/"
cp release.windows.x86_64.fast/rvvm_x86_64.exe release.windows.x86_64.fast/librvvm.dll "$OUT_DIR/fast/"

echo "=== Done. Outputs in $OUT_DIR/{stock,fast}/ ==="
ls -la "$OUT_DIR"/stock "$OUT_DIR"/fast
