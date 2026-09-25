package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var valueRequired bool

var valueCmd = &cobra.Command{
	Use:          "value <path>",
	Short:        "Print a profile config value (for use in profile scripts)",
	Hidden:       true,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := os.Getenv("KINDTKS_PROFILE_CONFIG")
		if path == "" {
			return fmt.Errorf("KINDTKS_PROFILE_CONFIG is not set; `kindtks value` is meant to be run from a profile script")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		dec := json.NewDecoder(f)
		dec.UseNumber()
		var doc any
		if err := dec.Decode(&doc); err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		lines, found, err := valueLines(doc, args[0])
		if err != nil {
			return err
		}
		if !found && valueRequired {
			return fmt.Errorf("%s is not set", args[0])
		}
		for _, l := range lines {
			fmt.Fprintln(cmd.OutOrStdout(), l)
		}
		return nil
	},
}

// valueLines resolves a dot path in doc and renders it for shell use: a
// scalar as one line, an array of scalars as one line per element. A missing
// or null value is not found.
func valueLines(doc any, path string) ([]string, bool, error) {
	if path == "" {
		return nil, false, fmt.Errorf("empty path")
	}
	cur := doc
	for _, key := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		if cur, ok = obj[key]; !ok {
			return nil, false, nil
		}
	}

	switch v := cur.(type) {
	case nil:
		return nil, false, nil
	case map[string]any:
		return nil, true, fmt.Errorf("%s is an object; query one of its fields", path)
	case []any:
		lines := make([]string, 0, len(v))
		for i, item := range v {
			s, err := scalarString(item)
			if err != nil {
				return nil, true, fmt.Errorf("%s[%d]: %w", path, i, err)
			}
			lines = append(lines, s)
		}
		return lines, true, nil
	default:
		s, err := scalarString(v)
		if err != nil {
			return nil, true, fmt.Errorf("%s: %w", path, err)
		}
		return []string{s}, true, nil
	}
}

func scalarString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	case bool:
		return strconv.FormatBool(x), nil
	default:
		return "", fmt.Errorf("not a scalar value")
	}
}

func init() {
	valueCmd.Flags().BoolVar(&valueRequired, "required", false, "fail if the value is not set")
	rootCmd.AddCommand(valueCmd)
}
