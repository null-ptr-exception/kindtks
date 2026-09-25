package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Every shipped profile's defaults must validate against its own schema.
func TestShippedProfilesResolve(t *testing.T) {
	entries, err := os.ReadDir("../../profiles")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			if _, err := Resolve("../../profiles", e.Name(), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The combined schema (common schema plus the real profile schema) must
// validate each shipped profile's own defaults.
func TestShippedProfilesValidateAgainstCombinedSchema(t *testing.T) {
	entries, err := os.ReadDir("../../profiles")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("../../profiles", name)
			schema, err := ProfileSchema(dir)
			if err != nil {
				t.Fatal(err)
			}
			combined, err := CombinedSchema(name, schema)
			if err != nil {
				t.Fatal(err)
			}
			data, err := jsonMarshal(combined)
			if err != nil {
				t.Fatal(err)
			}
			defaults, err := LoadDocument(filepath.Join(dir, "config.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			doc := map[string]any{"profiles": map[string]any{name: defaults}}
			if err := validate("combined.json", data, doc, ""); err != nil {
				t.Fatalf("combined schema rejected %s defaults: %v", name, err)
			}
		})
	}
}

func TestGen1AcceptsExtraHosts(t *testing.T) {
	user := writeFile(t, "user.yaml", "profiles:\n  gen1:\n    gateway:\n      extraHosts: [\"*.example.net\", \"app.example.com\"]\n")
	if _, err := Resolve("../../profiles", "gen1", user); err != nil {
		t.Fatal(err)
	}
	bad := writeFile(t, "bad.yaml", "profiles:\n  gen1:\n    gateway:\n      extraHosts: [\"not a host\"]\n")
	if _, err := Resolve("../../profiles", "gen1", bad); err == nil {
		t.Fatal("invalid host must be rejected")
	}
}
