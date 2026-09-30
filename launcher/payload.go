package main

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// Footer layout (64 bytes, little-endian), written at the very end of the
// packaged exe by packaging/package.ps1:
//
//	magic          [8]byte  "LXLABFT1"
//	payloadOffset  uint64   byte offset from start of file where the zip payload begins
//	payloadSize    uint64   size in bytes of the zip payload
//	version        uint32   footer format version, currently 1
//	reserved       [36]byte padding, reserved for future use
const (
	footerSize  = 64
	footerMagic = "LXLABFT1"
)

type footer struct {
	payloadOffset uint64
	payloadSize   uint64
	version       uint32
}

// openPayload locates and opens the zip payload appended to the running
// executable. Returns the zip reader and the underlying file (caller must
// close the file once done with the reader).
func openPayload() (*zip.Reader, *os.File, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("locating own executable: %w", err)
	}

	f, err := os.Open(exePath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening own executable: %w", err)
	}

	ft, err := readFooter(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}

	sr := io.NewSectionReader(f, int64(ft.payloadOffset), int64(ft.payloadSize))
	zr, err := zip.NewReader(sr, int64(ft.payloadSize))
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("reading payload zip (offset=%d size=%d): %w", ft.payloadOffset, ft.payloadSize, err)
	}

	return zr, f, nil
}

func readFooter(f *os.File) (*footer, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat own executable: %w", err)
	}
	if info.Size() < footerSize {
		return nil, errors.New("executable too small to contain a payload footer - this exe was run without being packaged")
	}

	buf := make([]byte, footerSize)
	if _, err := f.ReadAt(buf, info.Size()-footerSize); err != nil {
		return nil, fmt.Errorf("reading footer: %w", err)
	}

	if string(buf[0:8]) != footerMagic {
		return nil, errors.New("no payload footer found - this exe was run without being packaged (expected magic " + footerMagic + ")")
	}

	ft := &footer{
		payloadOffset: binary.LittleEndian.Uint64(buf[8:16]),
		payloadSize:   binary.LittleEndian.Uint64(buf[16:24]),
		version:       binary.LittleEndian.Uint32(buf[24:28]),
	}
	if ft.version != 1 {
		return nil, fmt.Errorf("unsupported payload footer version %d (this launcher only understands version 1)", ft.version)
	}
	if ft.payloadOffset+ft.payloadSize != uint64(info.Size())-footerSize {
		return nil, errors.New("payload footer offsets don't match file size - exe may be corrupted or truncated")
	}

	return ft, nil
}
