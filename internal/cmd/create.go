package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/prereq"
	"github.com/rophy/kindtks/internal/profile"
	"github.com/spf13/cobra"
)

var configFile string

var createCmd = &cobra.Command{
	Use:   "create <profile>",
	Short: "Create cluster(s) from a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		dir := profileDir()

		p, err := profile.Load(dir, name)
		if err != nil {
			return err
		}

		if missing := prereq.Check(p.Requires); len(missing) > 0 {
			return fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
		}

		cfg, err := loadProfileConfig(p)
		if err != nil {
			return err
		}

		fmt.Printf("Creating cluster(s) from profile %q...\n", name)
		return runProfileFunc(p, "create", cfg)
	},
}

func loadProfileConfig(p *profile.Profile) (*config.Config, error) {
	defaultCfgPath := filepath.Join(p.Dir, "config.yaml")
	base, err := config.Load(defaultCfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading profile defaults: %w", err)
	}

	if configFile == "" {
		return base, nil
	}

	override, err := config.Load(configFile)
	if err != nil {
		return nil, fmt.Errorf("loading config file: %w", err)
	}

	return config.Merge(base, override), nil
}

func runProfileFunc(p *profile.Profile, funcName string, cfg *config.Config) error {
	script := fmt.Sprintf("source %q && %s", p.Path, funcName)
	c := exec.Command("bash", "-e", "-c", script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".local", "share", "kindtks")

	env := append(os.Environ(),
		"KINDTKS_DATA_DIR="+dataDir,
		"KINDTKS_CHARTS_DIR="+filepath.Join(dataDir, "charts"),
		"KINDTKS_PROFILE_NAME="+p.Name,
		"KINDTKS_PROFILE_DIR="+p.Dir,
	)

	if cfg != nil {
		for key, image := range cfg.Images {
			envKey := "IMAGE_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
			env = append(env, envKey+"="+image)
		}
	}

	c.Env = env
	return c.Run()
}

func init() {
	createCmd.Flags().StringVar(&configFile, "config", "", "path to profile config override file")
	rootCmd.AddCommand(createCmd)
}
