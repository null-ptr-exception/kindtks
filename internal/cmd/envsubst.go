package cmd

import (
	"io"
	"os"
	"regexp"

	"github.com/spf13/cobra"
)

var envVarRef = regexp.MustCompile(`\$(?:\{([A-Za-z_][A-Za-z0-9_]*)\}|([A-Za-z_][A-Za-z0-9_]*))`)

// expandEnv replaces ${NAME} and $NAME with getenv(NAME), like GNU envsubst
// without arguments; every other $ is left as is.
func expandEnv(s string, getenv func(string) string) string {
	return envVarRef.ReplaceAllStringFunc(s, func(m string) string {
		sub := envVarRef.FindStringSubmatch(m)
		if sub[1] != "" {
			return getenv(sub[1])
		}
		return getenv(sub[2])
	})
}

var envsubstCmd = &cobra.Command{
	Use:          "envsubst",
	Short:        "Expand environment variables in stdin (for profile scripts)",
	Hidden:       true,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		in, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}
		_, err = io.WriteString(cmd.OutOrStdout(), expandEnv(string(in), os.Getenv))
		return err
	},
}

func init() {
	rootCmd.AddCommand(envsubstCmd)
}
