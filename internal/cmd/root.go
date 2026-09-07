package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func SetVersion(v string) {
	rootCmd.Version = v
}

var rootCmd = &cobra.Command{
	Use:   "kindtks",
	Short: "kindtks - an opinionated customization of kind kubernetes with many thanks",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
		if _, err := os.Stat("/.dockerenv"); err == nil {
			fmt.Println()
			fmt.Println("Install to your host:")
			fmt.Println("  docker run --rm \\")
			fmt.Println("    -v \"$HOME/.local/bin:/.local/bin\" \\")
			fmt.Println("    -v \"$HOME/.local/share:/.local/share\" \\")
			fmt.Println("    kindtks install")
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
