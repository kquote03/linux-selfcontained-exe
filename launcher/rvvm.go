package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// highPerfPowerPlanGUID is Windows' built-in "High performance" power
// scheme. RVVM's maintainer has a confirmed open issue (LekKit/RVVM#138):
// on hosts using a non-"performance" CPU frequency-scaling governor/power
// plan, RVVM's time-slicing assumption breaks and the guest slows down
// disproportionately - this is the single highest-leverage fix available
// on throttled lab/virtualized hardware, and it's a host setting, not
// something RVVM itself can control.
const highPerfPowerPlanGUID = "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c"

// setHighPerfPowerPlan best-effort switches the active Windows power plan.
// Failures (e.g. no admin rights, policy-locked power settings) are
// swallowed rather than failing the launch - this is a speed optimization,
// not something the student should see an error dialog over.
func setHighPerfPowerPlan() {
	_ = exec.Command("powercfg", "/setactive", highPerfPowerPlanGUID).Run()
}

// rvvmExePath picks the CPU-targeted RVVM build (tools/fast/ - AVX2/FMA3/
// AES, see docs/phase4-spike-results.md) when the host CPU supports it and
// the payload actually included it, falling back to the portable stock
// build otherwise. Returns the exe path; the DLL sits alongside it.
func rvvmExePath(toolsDir string) string {
	if fastRVVMSupported() {
		fastExe := filepath.Join(toolsDir, "fast", "rvvm_x86_64.exe")
		if _, err := os.Stat(fastExe); err == nil {
			return fastExe
		}
	}
	return filepath.Join(toolsDir, "rvvm_x86_64.exe")
}

// runRVVM launches RVVM as a child process with the given tools/disk paths
// and config, and blocks until it exits. librvvm.dll is resolved by
// Windows via the standard "same directory as the exe" DLL search rule, so
// cmd.Dir is set to whichever directory holds the chosen exe (extractTools
// guarantees the matching DLL sits right next to it).
func runRVVM(toolsDir, diskImagePath string, cfg config) error {
	if cfg.SetHighPerfPowerPlan {
		setHighPerfPowerPlan()
	}

	exePath := rvvmExePath(toolsDir)
	firmwarePath := filepath.Join(toolsDir, "fw_payload.bin")

	args := []string{
		firmwarePath,
		"-i", diskImagePath,
		"-m", cfg.RAM,
		"-smp", cfg.Cores,
		"-res", cfg.Resolution,
		"-nosound", // no audio requirement for this teaching appliance
		"-nogpu",   // confirmed harmless to the simplefb/DRM display path in Round 3 testing; drops RVVM's unused virtual GPU device
	}
	if strings.TrimSpace(cfg.ExtraArgs) != "" {
		args = append(args, strings.Fields(cfg.ExtraArgs)...)
	}

	cmd := exec.Command(exePath, args...)
	cmd.Dir = filepath.Dir(exePath)

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
