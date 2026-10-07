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
		fmt.Print(readme())
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
