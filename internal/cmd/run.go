package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/profile"
)

// runProfileFunc sources the profile script and calls funcName with the
// kindtks environment.
func runProfileFunc(p *profile.Profile, funcName string, res *config.Resolved, kindCfgPath string) error {
	script := fmt.Sprintf("source %q && %s", p.Path, funcName)
	c := exec.Command("bash", "-e", "-c", script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	tmpDir, err := os.MkdirTemp("", "kindtks-profile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	profileCfg := filepath.Join(tmpDir, "profile-config.json")
	data, err := json.Marshal(res.Profile)
	if err != nil {
		return fmt.Errorf("encoding profile config: %w", err)
	}
	if err := os.WriteFile(profileCfg, data, 0600); err != nil {
		return err
	}
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating kindtks binary: %w", err)
	}

	env := append(os.Environ(),
		"KINDTKS_DATA_DIR="+dataDir(),
		"KINDTKS_CHARTS_DIR="+filepath.Join(dataDir(), "charts"),
		"KINDTKS_PROFILE_NAME="+p.Name,
		"KINDTKS_PROFILE_DIR="+p.Dir,
		"KINDTKS_KIND_CONFIG="+kindCfgPath,
		"KINDTKS_BIN="+bin,
		"KINDTKS_PROFILE_CONFIG="+profileCfg,
	)
	for key, image := range res.Config.Images {
		envKey := "IMAGE_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		env = append(env, envKey+"="+image)
	}

	c.Env = env
	return c.Run()
}
