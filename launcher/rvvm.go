package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// runRVVM launches RVVM as a child process with the given tools/disk paths
// and config, and blocks until it exits. librvvm.dll is resolved by
// Windows via the standard "same directory as the exe" DLL search rule, so
// no special working-directory handling is needed as long as it sits next
// to rvvm_x86_64.exe in toolsDir (which extractTools guarantees).
func runRVVM(toolsDir, diskImagePath string, cfg config) error {
	exePath := filepath.Join(toolsDir, "rvvm_x86_64.exe")
	firmwarePath := filepath.Join(toolsDir, "fw_payload.bin")

	args := []string{
		firmwarePath,
		"-i", diskImagePath,
		"-m", cfg.RAM,
		"-smp", cfg.Cores,
		"-res", cfg.Resolution,
		"-nosound", // no audio requirement for this teaching appliance
	}
	if strings.TrimSpace(cfg.ExtraArgs) != "" {
		args = append(args, strings.Fields(cfg.ExtraArgs)...)
	}

	cmd := exec.Command(exePath, args...)
	cmd.Dir = toolsDir

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting RVVM: %w", err)
	}
	// A non-zero exit from RVVM itself (e.g. the student closed its window,
	// or the guest shut down) is normal and not a launcher failure, so it's
	// intentionally not returned as an error here - only a failure to start
	// the process at all is something the student needs to see a dialog for.
	_ = cmd.Wait()
	return nil
}
