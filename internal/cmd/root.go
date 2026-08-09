package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "kindtks",
	Short: "Bootstrap customized Kind clusters with pre-defined profiles",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("kindtks - Bootstrap customized Kind clusters")
		fmt.Println()
		fmt.Println("Quick start:")
		fmt.Println("  kindtks list              List available profiles")
		fmt.Println("  kindtks create <profile>  Create cluster(s) from a profile")
		fmt.Println("  kindtks delete <profile>  Delete cluster(s) from a profile")
		fmt.Println()
		fmt.Println("Install instructions:")
		fmt.Println("  docker run --rm \\")
		fmt.Println("    -v ~/.local/bin:/.local/bin \\")
		fmt.Println("    -v ~/.local/share/kindtks:/.local/share/kindtks \\")
		fmt.Println("    kindtks install")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
