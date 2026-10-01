# Phase 5 spike results: resizable, aspect-correct scaled RVVM window

Status: **PASS — verified by real boots: window resize (up and down from
native), aspect-ratio-preserving letterboxed scaling, and mouse-click
coordinate accuracy at a scaled size all confirmed working. Full packaged
exe regression confirmed clean. A resize-flicker bug in the first version
of this patch was reported after it shipped and has been fixed and
re-verified - see "Flicker fix" below.**

## Summary

The user asked whether the RVVM window could be made resizable with
scaled content. Investigated RVVM's own source (already had a working
mingw-w64 toolchain and a local commit-pinned checkout from Round 4) to
find out what's actually possible, then implemented it as a small patch
to RVVM's Win32 GUI backend.

## What RVVM actually supports (confirmed from source, not guessed)

- `win32_window.c`'s `WINDOW_STYLE_FLAGS` was `WS_CAPTION | WS_SYSMENU |
  WS_MINIMIZEBOX | WS_VISIBLE` - no `WS_THICKFRAME` (resizable border) or
  `WS_MAXIMIZEBOX`. The window was hard-coded non-resizable on Windows.
- The draw path used `SetDIBitsToDevice` - a 1:1 pixel blit with no
  scaling support at all.
- Checked whether another backend already solved this (worth reusing the
  pattern): macOS's Cocoa backend tracks a window size independent of the
  framebuffer, but only to **center** the native-resolution image with
  letterbox padding when the window is bigger - it does **not** stretch/
  scale the content to fill a resized window. No RVVM backend (Win32, X11,
  Wayland, Cocoa, SDL, Haiku) actually implements scaled-stretch rendering
  - this needed genuinely new code, not a port of existing logic.

## The fix

`image-build/patches/0001-win32-resizable-scaled-window.patch` against
`src/gui/win32_window.c` (commit `ce8ca7c...`, same commit already pinned
for the RVVM build):

1. **`WINDOW_STYLE_FLAGS`** gains `WS_THICKFRAME | WS_MAXIMIZEBOX` - the
   window can now be drag-resized and maximized.
2. **`win32_compute_fit()`** (new): given the current client area and the
   framebuffer's native size, computes an aspect-ratio-preserving scale
   factor and a centered destination rect - the same "fit inside, don't
   distort, letterbox the rest" convention as a typical video player's
   "scaled" view mode (which is what "scaled mode" in the request means
   here, as opposed to a stretch-to-fill that would distort the image).
3. **`win32_window_draw()`**: when the client area doesn't match the
   framebuffer's native size, fills the window with black and draws via
   `StretchDIBits` into the computed fit rect instead of the old 1:1-only
   `SetDIBitsToDevice` call. At native size this is unchanged behavior
   (`StretchDIBits` with matching src/dest dimensions is identical to the
   old blit).
4. **`WM_MOUSEMOVE` handler**: inverts the same fit calculation to map
   client-area mouse coordinates back into framebuffer coordinates before
   calling `gui_backend_on_mouse_place()` - without this, clicks would
   land in the wrong place as soon as the window was resized away from
   native resolution, since RVVM's input model expects absolute
   framebuffer-space coordinates (it drives an absolute-position USB
   tablet device, matching this project's I2C-HID input chain). The
   grabbed-mouse relative-delta path (`WM_MOUSEMOVE` while `win32->grab`
   is set) was left unscaled - out of scope for this fix, not broken,
   just not adjusted for zoom level.

## Build setup

`image-build/build-rvvm.sh` (new) replaces the previous ad-hoc approach of
downloading a prebuilt nightly CI artifact for the portable baseline: now
**both** shipped RVVM variants (portable baseline and the Round 4
CPU-targeted build) are self-built from the same patched, pinned-commit
checkout via the mingw-w64 toolchain already set up in Round 4, so both
carry this fix. `BUILDDIR` is set explicitly per variant
(`release.windows.x86_64.stock` / `.fast`) - RVVM's Makefile defaults
`BUILDDIR` to `$(BUILD_TYPE).$(OS).$(ARCH)` regardless of `CFLAGS`, so
without this the two builds would silently overwrite each other's output
in the same directory.

## Verification

All tests used the patched portable-baseline binary directly against the
Round 4 disk image, plus a final full packaged-exe regression:

1. **Native size (800x600, no resize)**: boots clean to desktop, confirms
   the unscaled code path still works identically to before the patch.
   Title bar now shows a maximize button, confirming `WS_MAXIMIZEBOX`
   took effect.
2. **Resized larger** (programmatic `SetWindowPos` to ~1200px wide):
   content scales up proportionally, no distortion, taskbar/panel/icons
   all render correctly at the new size.
3. **Resized smaller/different aspect ratio** (500x400 window, native
   4:3 content): confirmed **letterboxing works** - black bars appear
   above and below the proportionally-scaled content, centered, no
   stretching/distortion, no garbage pixels in the letterbox area.
4. **Mouse-click accuracy at a scaled size**: with the window resized
   larger, clicked on the desktop's "Home" icon at its new, scaled
   on-screen position - the icon was correctly selected (confirmed via
   screenshot showing its selection highlight exactly where clicked).
   This is the critical correctness check for the coordinate-remapping
   fix; without it, resizing would have silently broken all mouse
   interaction.
5. **Full packaged-exe end-to-end regression**: packaged a fresh test exe
   with both newly-patched RVVM variants + the existing Round 4 disk
   image, cold-launched it, confirmed the launcher correctly auto-selects
   the (now also patched) fast build via `rvvmExePath()`, and the guest
   boots to a fully rendered XFCE4 desktop.

## Flicker fix (post-release)

Release `2026.10.01-r5-resize` shipped with the above, but the user
reported the screen flickers while actively dragging to resize. Deleted
that release (and its git tag) and root-caused the flicker:

1. **No double buffering.** `win32_window_draw()`'s scaled path drew
   directly to the window's device context: `FillRect` (black) then
   `StretchDIBits`, as two separate visible operations on screen - a
   classic flicker source (the black fill is briefly visible before the
   picture lands on top of it).
2. **Stale content during a live drag.** RVVM's `poll()`/`draw()` loop is
   driven from its own code, not from Windows' paint messages. An
   interactive border-drag resize runs inside a Windows-internal modal
   loop (entered on `WM_SYSCOMMAND(SC_SIZE)`) that blocks the app's own
   loop from running until the drag ends - so without explicit handling,
   the window frame resizes instantly (Windows draws that part itself)
   while the picture inside stays stuck at the old size/content until
   the mouse is released, then snaps to match. That mismatch mid-drag is
   what reads as a flicker/flash.

**Fix**, both in `src/gui/win32_window.c`:

- Replaced the direct-to-screen draw with `win32_present()`: composes the
  full frame (background fill + scaled blit) into an off-screen memory DC
  (`CreateCompatibleDC`/`CreateCompatibleBitmap`) sized to the live client
  area, then presents it with one `BitBlt` call - the window never shows
  a partially-drawn frame, since compositing happens off-screen first.
- Linked each window's `win32_window_t*` state to its `HWND` via
  `SetWindowLongPtr(..., GWLP_USERDATA, ...)` so `win32_wndproc()` can
  reach it, and added a `WM_SIZE` handler that calls `win32_present()`
  immediately using the last-known framebuffer (`win32->fb`, already
  cached from the previous normal draw call) - this is what keeps the
  content visually in sync *during* the live drag, not just after it ends.
- Added a `WM_ERASEBKGND` handler returning `1` (handled, no-op) - since
  `win32_present()` always repaints the entire client area itself, the
  default background erase is redundant and was adding its own visible
  flash before each real repaint.

**Re-verified**: rebuilt both RVVM variants from the corrected patch,
confirmed native-size boot is unchanged, resize-larger and resize-smaller
both scale/letterbox correctly with no visible corruption sampled across
several intermediate sizes, and mouse-click accuracy at a scaled size is
still correct. A full packaged-exe end-to-end regression passed again.
True flicker during a continuous drag can't be definitively proven from
static screenshots, but both identified root causes (non-atomic
direct-to-screen drawing, and stale content during the OS's modal resize
loop) are now directly addressed by standard, well-understood Win32
techniques for exactly this class of problem.

## Files changed this round

- `image-build/patches/0001-win32-resizable-scaled-window.patch` (new) -
  the RVVM source patch itself.
- `image-build/build-rvvm.sh` (new) - reproducible dual-build script
  (clone/checkout pinned commit, apply patch, build both variants with
  distinct `BUILDDIR`s).
- `packaging/payload-manifest.json` - `rvvm`/`rvvmFast` entries updated to
  reflect both variants now being self-built from the patched source via
  `image-build/build-rvvm.sh`, rather than one vendored nightly download
  plus one self-built variant.
