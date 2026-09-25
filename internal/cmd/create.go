package cmd

import (
	"fmt"
	"os"
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

		res, err := config.Resolve(dir, name, configFile)
		if err != nil {
			return err
		}
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}

		fmt.Printf("Creating cluster(s) from profile %q...\n", name)
		kindCfgPath, cleanup, err := prepareKindConfig(filepath.Join(p.Dir, "kind-config.yaml"), res.Config, profileStateDir(name))
		if err != nil {
			return err
		}
		defer cleanup()

		return runProfileFunc(p, "create", res, kindCfgPath)
	},
}

func init() {
	createCmd.Flags().StringVar(&configFile, "config", "", "path to profile config override file")
	rootCmd.AddCommand(createCmd)
}
