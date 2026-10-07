package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var projectReadme string

func SetProjectReadme(s string) {
	projectReadme = s
}

// readme returns the project README with the run command pointing at the
// image kindtks is running from.
func readme() string {
	if img := os.Getenv("KINDTKS_IMAGE"); img != "" {
		return strings.ReplaceAll(projectReadme, imageRef, img)
	}
	return projectReadme
}

var helpCmd = &cobra.Command{
	Use:   "help [profile]",
	Short: "Show documentation for the project or a specific profile",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			fmt.Print(readme())
			return nil
		}

		name := args[0]
		dir, err := profileDir()
		if err != nil {
			return err
		}
		readmePath := filepath.Join(dir, name, "README.md")
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
