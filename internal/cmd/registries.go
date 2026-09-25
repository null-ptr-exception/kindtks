package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/kindconfig"
)

// stateDir is kindtks' XDG state dir. It is kept apart from dataDir because
// `kindtks install` runs in Docker as root and may leave dataDir root-owned.
func stateDir() string {
	if xdg := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "kindtks")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "kindtks")
}

// profileStateDir holds files a profile's clusters need at runtime, such as
// the registry host files mounted into Kind nodes.
func profileStateDir(profile string) string {
	return filepath.Join(stateDir(), profile)
}

// prepareKindConfig writes the registry host files under stateDir and returns
// the kind config to use: the original path when no registry settings apply,
// otherwise a patched temp copy removed by the returned cleanup func.
func prepareKindConfig(kindCfgPath string, cfg *config.Config, stateDir string) (string, func(), error) {
	noop := func() {}
	certsDir := filepath.Join(stateDir, "certs.d")

	if regs := cfg.PrivateRegistries(); len(regs) > 0 {
		fmt.Printf("Trusting private registries: %s\n", strings.Join(regs, ", "))
	}

	files := cfg.HostsFiles()
	if len(files) == 0 {
		if err := os.RemoveAll(certsDir); err != nil {
			return "", noop, err
		}
		certsDir = ""
	} else {
		if err := kindconfig.WriteHostsDir(certsDir, files); err != nil {
			return "", noop, fmt.Errorf("writing registry host files: %w", err)
		}
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Printf("Configuring registry hosts: %s\n", strings.Join(names, ", "))
	}

	if regs := cfg.AuthRegistries(); len(regs) > 0 {
		fmt.Printf("Configuring registry auth: %s\n", strings.Join(regs, ", "))
	}

	path, cleanup, err := kindconfig.Patch(kindCfgPath, kindconfig.Options{
		CertsDir:  certsDir,
		AuthPatch: cfg.AuthPatch(),
	})
	if err != nil {
		return "", noop, fmt.Errorf("patching kind config for registries: %w", err)
	}
	return path, cleanup, nil
}
