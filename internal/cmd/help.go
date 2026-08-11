package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var projectReadme string

func SetProjectReadme(s string) {
	projectReadme = s
}

var helpCmd = &cobra.Command{
	Use:   "help [profile]",
	Short: "Show documentation for the project or a specific profile",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			fmt.Print(projectReadme)
			return nil
		}

		name := args[0]
		readmePath := filepath.Join(profileDir(), name, "README.md")
		data, err := os.ReadFile(readmePath)
		if err != nil {
			return fmt.Errorf("no README found for profile %q", name)
		}
		fmt.Print(string(data))
		return nil
	},
}

func init() {
	rootCmd.SetHelpCommand(helpCmd)
}
