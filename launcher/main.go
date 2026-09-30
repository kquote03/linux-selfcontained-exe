package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		showErrorDialog(err)
		os.Exit(1)
	}
}

func run() error {
	zr, exeFile, err := openPayload()
	if err != nil {
		return fmt.Errorf("this exe doesn't look like a properly packaged Linux Lab build.\n\n%w", err)
	}
	defer exeFile.Close()

	base, err := baseDir()
	if err != nil {
		return err
	}

	version := payloadVersion(zr)
	toolsDir, err := extractTools(zr, base, version)
	if err != nil {
		return fmt.Errorf("extracting emulator files: %w", err)
	}

	runDir, err := newRunDir(base)
	if err != nil {
		return fmt.Errorf("preparing session directory: %w", err)
	}
	defer func() {
		os.RemoveAll(runDir)        // this run's own scratch dir
		sweepStaleRuns(base, runDir) // plus anything else left over, best-effort
	}()

	sweepStaleRuns(base, runDir) // also sweep on the way in, in case a prior run crashed

	diskImagePath, err := extractDiskImage(zr, runDir)
	if err != nil {
		return fmt.Errorf("preparing a fresh Linux session: %w", err)
	}

	cfg := loadConfig()

	return runRVVM(toolsDir, diskImagePath, cfg)
}
