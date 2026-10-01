package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// config holds the overridable RVVM launch settings. Defaults are
// conservative for weak, non-accelerated lab hardware; an instructor can
// raise them on faster fleets by editing config.ini next to the exe -
// no rebuild needed.
type config struct {
	RAM                  string // e.g. "1G"
	Cores                string // e.g. "1"
	Resolution           string // e.g. "1024x768"
	ExtraArgs            string // raw extra RVVM CLI flags, split on whitespace and appended as-is
	SetHighPerfPowerPlan bool   // opt-in: switch the Windows power plan to High Performance before launching RVVM
}

// Defaults target genuinely weak lab hardware (dual-core hosts with as
// little as 2GB total RAM) - RVVM's own built-in default is "256M", so 1G
// here is still generous headroom above that baseline while leaving the
// other ~1GB of host RAM for Windows and the emulator process itself.
// Cores stays at 2 (RVVM's own default is 1): an empirical A/B boot test
// during Round 3 tuning showed -smp 1 reaching the desktop in ~3-4
// minutes vs. ~60-90s for -smp 2 on the same hardware - RVVM's docs give
// no guidance either way, but the measured result favors 2 cores even on
// a 2-core host, so that's the default (see docs/phase3-spike-results.md).
// Resolution dropped to 800x600 in Round 4: a real target machine's CPU-Z
// report showed a fully non-accelerated Cirrus Logic VGA display chain
// with zero GPU acceleration at any layer, so fewer pixels means less
// software blit work; visually confirmed XFCE still renders cleanly at
// this size. RAM was reconsidered but kept at 1G in Round 4 after
// confirming zram shows 0B used at an idle desktop - there's no swap
// pressure at the current default to relieve by raising it (see
// docs/phase4-spike-results.md). An instructor on faster/slower fleets
// can override any of these in config.ini - no rebuild needed.
func defaultConfig() config {
	return config{
		RAM:                  "1G",
		Cores:                "2",
		Resolution:           "800x600",
		ExtraArgs:            "",
		SetHighPerfPowerPlan: false,
	}
}

// loadConfig reads config.ini next to the running exe if present, applying
// any keys found on top of the defaults. Missing file or unreadable file
// is not an error - it just means "use defaults".
func loadConfig() config {
	cfg := defaultConfig()

	exePath, err := os.Executable()
	if err != nil {
		return cfg
	}
	iniPath := filepath.Join(filepath.Dir(exePath), "config.ini")

	f, err := os.Open(iniPath)
	if err != nil {
		return cfg
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		switch key {
		case "ram":
			cfg.RAM = val
		case "cores":
			cfg.Cores = val
		case "resolution":
			cfg.Resolution = val
		case "extra_args":
			cfg.ExtraArgs = val
		case "set_high_perf_power_plan":
			cfg.SetHighPerfPowerPlan = val == "1" || strings.EqualFold(val, "true")
		}
	}
	return cfg
}
