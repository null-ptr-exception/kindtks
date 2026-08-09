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

		fmt.Printf("Creating cluster(s) from profile %q...\n", name)
		return runProfileFunc(p, "create")
	},
}

func runProfileFunc(p *profile.Profile, funcName string) error {
	script := fmt.Sprintf("source %q && %s", p.Path, funcName)
	c := exec.Command("bash", "-e", "-c", script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	cfg, _ := config.Load(config.DefaultPath())
	registry := ""
	if cfg != nil {
		registry = cfg.Registry
	}

	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".local", "share", "kindtks")

	c.Env = append(os.Environ(),
		"KINDTKS_DATA_DIR="+dataDir,
		"KINDTKS_CHARTS_DIR="+filepath.Join(dataDir, "charts"),
		"KINDTKS_PROFILE_NAME="+p.Name,
		"KINDTKS_PROFILE_DIR="+p.Dir,
		"KINDTKS_REGISTRY="+registry,
	)
	return c.Run()
}

func init() {
	rootCmd.AddCommand(createCmd)
}
