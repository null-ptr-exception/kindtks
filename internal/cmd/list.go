package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rophy/kindtks/internal/profile"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		profileDir := profileDir()
		if _, err := os.Stat(profileDir); os.IsNotExist(err) {
			fmt.Println("No profiles found in", profileDir)
			return nil
		}
		names, err := profile.ListAll(profileDir)
		if err != nil {
			return fmt.Errorf("listing profiles: %w", err)
		}
		if len(names) == 0 {
			fmt.Println("No profiles found in", profileDir)
			return nil
		}
		for _, name := range names {
			p, err := profile.Load(profileDir, name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				continue
			}
			fmt.Printf("%-20s requires: %s\n", p.Name, joinOrNone(p.Requires))
		}
		return nil
	},
}

func profileDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "kindtks", "profiles")
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	result := ""
	for i, item := range items {
		if i > 0 {
			result += ", "
		}
		result += item
	}
	return result
}

func init() {
	rootCmd.AddCommand(listCmd)
}
