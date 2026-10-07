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

var deleteCmd = &cobra.Command{
	Use:          "delete <profile>",
	Short:        "Delete cluster(s) created by a profile",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkMounts(dockerReachable); err != nil {
			return err
		}
		name := args[0]
		dir, err := profileDir()
		if err != nil {
			return err
		}

		p, err := profile.Load(dir, name)
		if err != nil {
			return err
		}

		if missing := prereq.Check(p.Requires); len(missing) > 0 {
			return fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
		}

		fmt.Printf("Deleting cluster(s) from profile %q...\n", name)
		res, err := config.Resolve(dir, name, "")
		if err != nil {
			return err
		}
		if err := runProfileFunc(p, "delete", res, filepath.Join(p.Dir, "kind-config.yaml")); err != nil {
			return err
		}
		return os.RemoveAll(profileStateDir(name))
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
