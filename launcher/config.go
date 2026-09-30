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
	RAM        string // e.g. "2G"
	Cores      string // e.g. "2"
	Resolution string // e.g. "1024x768"
	ExtraArgs  string // raw extra RVVM CLI flags, split on whitespace and appended as-is
}

func defaultConfig() config {
	return config{
		RAM:        "2G",
		Cores:      "2",
		Resolution: "1024x768",
		ExtraArgs:  "",
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
		}
	}
	return cfg
}
