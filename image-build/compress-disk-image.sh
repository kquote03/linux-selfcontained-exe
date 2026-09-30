#!/bin/bash
# Compresses the built disk.img with zstd for shipping inside the packaged
# exe. Uses a high compression level since this runs once per image build,
# not per student launch (the launcher decompresses it fresh on every run -
# see launcher/extract.go).
set -euo pipefail
IMG="${1:-/root/build/disk.img}"
OUT="${2:-${IMG}.zst}"

echo "=== Compressing $IMG -> $OUT ==="
zstd -19 -T0 -f "$IMG" -o "$OUT"
ls -la "$IMG" "$OUT"
