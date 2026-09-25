package cmd

import (
	"encoding/json"
	"os"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/profile"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var showSchema bool

var configCmd = &cobra.Command{
	Use:   "config <profile>",
	Short: "Show default configuration (or its JSON schema) for a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		dir := profileDir()

		p, err := profile.Load(dir, name)
		if err != nil {
			return err
		}

		if showSchema {
			schema, err := config.ProfileSchema(p.Dir)
			if err != nil {
				return err
			}
			combined, err := config.CombinedSchema(name, schema)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(combined)
		}

		res, err := config.Resolve(dir, name, "")
		if err != nil {
			return err
		}
		out, err := yaml.Marshal(yamlSafe(map[string]any{"profiles": map[string]any{name: res.Profile}}))
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(out)
		return err
	},
}

// yamlSafe walks a value decoded with json.Number (as config.Resolve
// produces), converting numbers to int64 or float64 so yaml.Marshal renders
// them unquoted instead of as strings.
func yamlSafe(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = yamlSafe(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = yamlSafe(val)
		}
		return out
	default:
		return v
	}
}

func init() {
	configCmd.Flags().BoolVar(&showSchema, "schema", false, "print the JSON schema instead of the defaults")
	rootCmd.AddCommand(configCmd)
}
