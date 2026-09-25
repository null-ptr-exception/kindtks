package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLSafe_NumbersRenderUnquoted(t *testing.T) {
	v := map[string]any{
		"replicas": json.Number("3"),
		"ratio":    json.Number("1.5"),
		"name":     "3", // a string that looks numeric must stay quoted
		"nested": map[string]any{
			"count": json.Number("42"),
		},
		"list": []any{json.Number("1"), "two"},
	}

	out, err := yaml.Marshal(yamlSafe(v))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)

	for _, want := range []string{"replicas: 3\n", "ratio: 1.5\n", `name: "3"` + "\n", "count: 42\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"1"`) {
		t.Errorf("integer in list should not be quoted, got:\n%s", got)
	}
}
