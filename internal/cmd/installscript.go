package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed installer.sh
var installerScript string

var buildIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var installScriptCmd = &cobra.Command{
	Use:          "install-script",
	Short:        "Print the installer for this kindtks image (pipe it to bash)",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := os.Getenv("KINDTKS_BUILD_ID")
		switch {
		case id == "":
			return errors.New("KINDTKS_BUILD_ID is not set; run this from the kindtks image: docker run --rm <image> install-script | bash")
		case id == "dev":
			return errors.New("this image was built without a build ID; rebuild it with --build-arg BUILD_ID=<unique id>")
		case !buildIDPattern.MatchString(id):
			return fmt.Errorf("invalid build ID %q", id)
		}
		fmt.Fprint(cmd.OutOrStdout(), strings.ReplaceAll(installerScript, "__KINDTKS_BUILD_ID__", id))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installScriptCmd)
}
