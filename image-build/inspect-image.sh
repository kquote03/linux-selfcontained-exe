#!/bin/bash
# Debugging utility: mounts a built disk.img read-only and dumps the logs
# most useful for diagnosing a boot/display hang (Xorg, lightdm). Used
# during Phase 1 to find the plymouth hang and the missing simple-framebuffer
# driver — see docs/phase1-spike-results.md.
#
# Usage: ./inspect-image.sh /path/to/disk.img
set -euo pipefail

IMG="${1:?Usage: $0 /path/to/disk.img}"
MNT="/root/build/inspect_mnt"

losetup -D || true
mkdir -p "$MNT"
LOOP=$(losetup -fP --show "$IMG")
echo "Loop: $LOOP"
mount -o ro "${LOOP}p1" "$MNT"

echo "--- Xorg log ---"
tail -c 4000 "$MNT"/var/log/Xorg.0.log 2>&1 || echo "NO XORG LOG"
echo "--- lightdm log ---"
tail -c 3000 "$MNT"/var/log/lightdm/lightdm.log 2>&1 || echo "NO LIGHTDM LOG"
echo "--- lightdm seat0 greeter log ---"
tail -c 2000 "$MNT"/var/log/lightdm/seat0-greeter.log 2>&1 || echo "NO GREETER LOG"

umount "$MNT"
losetup -d "$LOOP"
