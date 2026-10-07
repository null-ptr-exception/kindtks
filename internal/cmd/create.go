package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/prereq"
	"github.com/rophy/kindtks/internal/profile"
	"github.com/rophy/kindtks/internal/regauth"
	"github.com/spf13/cobra"
)

var configFile string
var skipAuthCheck bool

var createCmd = &cobra.Command{
	Use:          "create <profile>",
	Short:        "Create cluster(s) from a profile",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
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

		flow := &credentialFlow{cfg: res.Config, out: os.Stderr}
		if !skipAuthCheck {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			flow.check = func(reg, user, pass string) (regauth.Result, error) {
				ep := res.Config.AuthEndpoint(reg)
				return regauth.Check(ctx, regauth.Endpoint{URL: ep.URL, SkipVerify: ep.SkipVerify},
					user, pass, res.Config.PullScopes(reg))
			}
		}
		if len(res.Config.PromptRegistries()) > 0 {
			if t, err := openTerminal(); err == nil {
				defer t.Close()
				flow.term = t
			}
		}
		if err := flow.run(); err != nil {
			return err
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
	createCmd.Flags().BoolVar(&skipAuthCheck, "skip-auth-check", false, "do not check registry credentials against the registries")
	createCmd.Flags().StringVar(&configFile, "config", "", "path to profile config override file")
	rootCmd.AddCommand(createCmd)
}
