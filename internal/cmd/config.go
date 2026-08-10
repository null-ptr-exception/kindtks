package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/profile"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config <profile>",
	Short: "Show default configuration for a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		dir := profileDir()

		p, err := profile.Load(dir, name)
		if err != nil {
			return err
		}

		cfgPath := filepath.Join(p.Dir, "config.yaml")
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("loading profile config: %w", err)
		}

		out, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(out)
		return err
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}
