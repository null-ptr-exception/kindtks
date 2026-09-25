package config

import (
	"os"
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
