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

// optionalToolFiles are extracted best-effort if present in the payload,
// without failing the whole launch if they're missing (e.g. an older or
// fast-build-less test package). The CPU-targeted RVVM build (AVX2/FMA3/
// AES, no BMI2 assumed - see docs/phase4-spike-results.md) is used instead
// of the stock rvvm_x86_64.exe/librvvm.dll above when fastRVVMSupported()
// reports the host CPU can run it (see rvvmExePath in rvvm.go). Kept in
// its own subdirectory since both builds export the same DLL filename and
// Windows resolves "librvvm.dll" from the launching exe's own directory -
// they can't sit side by side in the same folder.
var optionalToolFiles = []string{
	"tools/fast/rvvm_x86_64.exe",
	"tools/fast/librvvm.dll",
}

const diskImageEntry = "disk.img.zst"

// progressFunc reports extraction progress: percent is 0-100, status is a
// short human-readable phase description.
type progressFunc func(percent int, status string)

// findZipFile looks up a *zip.File by name, giving access to its recorded
// size without opening/reading it.
func findZipFile(zr *zip.Reader, name string) (*zip.File, error) {
	for _, f := range zr.File {
		if f.Name == name {
			return f, nil
		}
	}
	return nil, fmt.Errorf("entry not found in payload: %s", name)
}

func percentOf(done, total int64) int {
	if total <= 0 {
		return 0
	}
	p := int(done * 100 / total)
	if p > 100 {
		p = 100
	}
	return p
}

// countingReader tracks bytes read and invokes onRead after each read.
type countingReader struct {
	r      io.Reader
	n      int64
	onRead func(n int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n += int64(n)
		if c.onRead != nil {
			c.onRead(c.n)
		}
	}
	return n, err
}

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
func extractTools(zr *zip.Reader, base, version string, progress progressFunc) (string, error) {
	toolsDir := filepath.Join(base, "tools", version)
	marker := filepath.Join(toolsDir, ".complete")

	if _, err := os.Stat(marker); err == nil {
		return toolsDir, nil // already extracted for this version
	}

	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return "", fmt.Errorf("creating tools dir %s: %w", toolsDir, err)
	}

	var totalSize int64
	sizes := make(map[string]int64, len(toolFiles))
	for _, entry := range toolFiles {
		f, err := findZipFile(zr, entry)
		if err != nil {
			return "", fmt.Errorf("locating %s in payload: %w", entry, err)
		}
		sizes[entry] = int64(f.UncompressedSize64)
		totalSize += sizes[entry]
	}

	var done int64
	for _, entry := range toolFiles {
		if progress != nil {
			progress(percentOf(done, totalSize), "Extracting emulator files...")
		}
		// Preserve the path under "tools/" (not just the basename) so the
		// "fast/" subdirectory entries don't collide with the identically-
		// named stock rvvm_x86_64.exe/librvvm.dll at the tools dir root.
		relPath := strings.TrimPrefix(entry, "tools/")
		destPath := filepath.Join(toolsDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return "", fmt.Errorf("creating dir for %s: %w", entry, err)
		}
		if err := extractZipEntry(zr, entry, destPath); err != nil {
			return "", fmt.Errorf("extracting %s: %w", entry, err)
		}
		done += sizes[entry]
	}
	for _, entry := range optionalToolFiles {
		if _, err := findZipFile(zr, entry); err != nil {
			continue // not present in this payload - fine, stock build is used instead
		}
		relPath := strings.TrimPrefix(entry, "tools/")
		destPath := filepath.Join(toolsDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return "", fmt.Errorf("creating dir for %s: %w", entry, err)
		}
		if err := extractZipEntry(zr, entry, destPath); err != nil {
			return "", fmt.Errorf("extracting %s: %w", entry, err)
		}
	}

	if progress != nil {
		progress(100, "Extracting emulator files...")
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
func extractDiskImage(zr *zip.Reader, scratchDir string, progress progressFunc) (string, error) {
	f, err := findZipFile(zr, diskImageEntry)
	if err != nil {
		return "", fmt.Errorf("locating %s in payload: %w", diskImageEntry, err)
	}
	// disk.img.zst is stored uncompressed-by-zip (see packaging/package.ps1),
	// so UncompressedSize64 here is just the .zst file's own byte size - the
	// most reliable progress denominator available without decoding the
	// zstd stream, since compressed-bytes-read tracks closely enough with
	// decompression throughput for a progress bar.
	totalZstBytes := int64(f.UncompressedSize64)

	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("opening %s in payload: %w", diskImageEntry, err)
	}
	defer rc.Close()

	lastPercent := -1
	cr := &countingReader{r: rc, onRead: func(n int64) {
		if progress == nil {
			return
		}
		if p := percentOf(n, totalZstBytes); p != lastPercent {
			lastPercent = p
			progress(p, "Preparing your Linux session...")
		}
	}}

	dec, err := zstd.NewReader(cr)
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
	if progress != nil {
		progress(100, "Preparing your Linux session...")
	}
	return destPath, nil
}
