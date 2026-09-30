package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// toolFiles are the payload entries extracted into the shared, cached tools
// directory. They're static across runs of the same packaged version, so
// unlike the disk image they are safe to reuse between launches instead of
// re-extracting every time.
var toolFiles = []string{
	"tools/rvvm_x86_64.exe",
	"tools/librvvm.dll",
	"tools/fw_payload.bin",
}

const diskImageEntry = "disk.img.zst"

// baseDir returns %LOCALAPPDATA%\LinuxLab, creating it if needed.
func baseDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", fmt.Errorf("LOCALAPPDATA environment variable is not set")
	}
	dir := filepath.Join(local, "LinuxLab")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	return dir, nil
}

// payloadVersion reads the VERSION file baked into the payload by the
// packaging script, used to namespace the tools cache so a newer packaged
// exe doesn't reuse a stale cached RVVM binary. Falls back to "dev" if
// absent (e.g. running an unpackaged/test build).
func payloadVersion(zr *zip.Reader) string {
	f, err := zr.Open("VERSION")
	if err != nil {
		return "dev"
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || len(b) == 0 {
		return "dev"
	}
	return strings.TrimSpace(string(b))
}

// extractTools ensures the static tool files are present in the cached
// tools directory for this payload version, extracting them only if not
// already there. Returns the tools directory path.
func extractTools(zr *zip.Reader, base, version string) (string, error) {
	toolsDir := filepath.Join(base, "tools", version)
	marker := filepath.Join(toolsDir, ".complete")

	if _, err := os.Stat(marker); err == nil {
		return toolsDir, nil // already extracted for this version
	}

	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", fmt.Errorf("creating tools dir %s: %w", toolsDir, err)
	}

	for _, entry := range toolFiles {
		if err := extractZipEntry(zr, entry, filepath.Join(toolsDir, filepath.Base(entry))); err != nil {
			return "", fmt.Errorf("extracting %s: %w", entry, err)
		}
	}

	if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
		return "", fmt.Errorf("writing completion marker: %w", err)
	}
	return toolsDir, nil
}

func extractZipEntry(zr *zip.Reader, name, destPath string) error {
	rc, err := zr.Open(name)
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return err
	}
	return nil
}

// extractDiskImage decompresses the pristine zstd-compressed disk image
// from the payload into a fresh file in the given scratch directory. A
// fresh decompress on every launch (rather than a cached copy that gets
// duplicated) is the simplest way to guarantee every session starts from
// truly pristine state with no separate integrity check needed.
func extractDiskImage(zr *zip.Reader, scratchDir string) (string, error) {
	rc, err := zr.Open(diskImageEntry)
	if err != nil {
		return "", fmt.Errorf("opening %s in payload: %w", diskImageEntry, err)
	}
	defer rc.Close()

	dec, err := zstd.NewReader(rc)
	if err != nil {
		return "", fmt.Errorf("initializing zstd decoder: %w", err)
	}
	defer dec.Close()

	destPath := filepath.Join(scratchDir, "disk.img")
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("creating %s: %w", destPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, dec); err != nil {
		return "", fmt.Errorf("decompressing disk image: %w", err)
	}
	return destPath, nil
}
