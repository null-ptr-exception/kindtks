package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gen1Defaults = `images:
  cilium: quay.io/cilium/cilium:v1.13.10
gateway:
  extraHosts: []
`

func writeProfile(t *testing.T, profilesDir, name, defaults, schema string) {
	t.Helper()
	dir := filepath.Join(profilesDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "install.sh"), []byte("create() { :; }\ndelete() { :; }\n"), 0644)
	if defaults != "" {
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(defaults), 0644)
	}
	if schema != "" {
		os.WriteFile(filepath.Join(dir, "config.schema.json"), []byte(schema), 0644)
	}
}

func setupProfiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeProfile(t, dir, "gen1", gen1Defaults, testProfileSchema)
	return dir
}

func TestResolve_DefaultsOnly(t *testing.T) {
	res, err := Resolve(setupProfiles(t), "gen1", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.Images["cilium"] != "quay.io/cilium/cilium:v1.13.10" {
		t.Errorf("unexpected images %v", res.Config.Images)
	}
	if _, ok := res.Profile["gateway"]; !ok {
		t.Errorf("profile section should include gateway: %v", res.Profile)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("unexpected warnings %v", res.Warnings)
	}
}

func TestResolve_UserOverridesProfileSection(t *testing.T) {
	user := writeFile(t, "user.yaml", `profiles:
  gen1:
    images:
      cilium: registry.example.com/cilium:v1
    gateway:
      extraHosts: ["*.example.net"]
`)
	res, err := Resolve(setupProfiles(t), "gen1", user)
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.Images["cilium"] != "registry.example.com/cilium:v1" {
		t.Errorf("image not overridden: %v", res.Config.Images)
	}
	hosts := res.Profile["gateway"].(map[string]any)["extraHosts"].([]any)
	if len(hosts) != 1 || hosts[0] != "*.example.net" {
		t.Errorf("extraHosts not replaced: %v", hosts)
	}
}

func TestResolve_Registries(t *testing.T) {
	user := writeFile(t, "user.yaml", `registries:
  quay.io:
    hosts: |
      server = "https://quay.io"
  registry.example.com:
    auth: {username: u, password: p}
`)
	res, err := Resolve(setupProfiles(t), "gen1", user)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Config.Registries["quay.io"].Hosts, "https://quay.io") {
		t.Errorf("hosts not decoded: %+v", res.Config.Registries["quay.io"])
	}
	if a := res.Config.Registries["registry.example.com"].Auth; a == nil || a.Username != "u" || a.Password != "p" {
		t.Errorf("auth not decoded: %+v", a)
	}
}

func TestResolve_Errors(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"top-level images":       {"images: {cilium: x}\n", "profiles.<profile>.images"},
		"registryAuth":           {"registryAuth: {}\n", "registries.<registry>.auth"},
		"unknown top-level key":  {"bogus: 1\n", "bogus"},
		"unknown profile option": {"profiles:\n  gen1:\n    bogus: 1\n", "profiles.gen1"},
		"bad extra host":         {"profiles:\n  gen1:\n    gateway: {extraHosts: [\"BAD\"]}\n", "profiles.gen1.gateway.extraHosts[0]"},
		"invalid hosts TOML":     {"registries:\n  quay.io:\n    hosts: \"server = \"\n", "quay.io"},
		"unsafe registry name":   {"registries:\n  a..b:\n    hosts: \"x = 1\"\n", "a..b"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve(setupProfiles(t), "gen1", writeFile(t, "user.yaml", c.yaml))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error mentioning %q, got %v", c.want, err)
			}
		})
	}
}

func TestResolve_InvalidDefaultsRejected(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "gen1", "bogus: 1\n", testProfileSchema)
	if _, err := Resolve(dir, "gen1", ""); err == nil {
		t.Fatal("defaults violating the profile schema must be rejected")
	}
}

func TestResolve_ProfileWithoutSchemaAcceptsOnlyImages(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "plain", "images:\n  x: img:1\n", "")
	if _, err := Resolve(dir, "plain", ""); err != nil {
		t.Fatalf("images-only defaults should be valid: %v", err)
	}
	user := writeFile(t, "user.yaml", "profiles:\n  plain:\n    gateway: {}\n")
	if _, err := Resolve(dir, "plain", user); err == nil {
		t.Fatal("non-images option should be rejected for a profile without schema")
	}
}

func TestResolve_OtherInstalledProfileValidatedWithItsOwnDefaults(t *testing.T) {
	dir := setupProfiles(t)
	otherSchema := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["name"],
  "properties": {"name": {"type": "string"}, "extra": {"type": "string"}}
}`
	writeProfile(t, dir, "other", "name: from-defaults\n", otherSchema)
	user := writeFile(t, "user.yaml", "profiles:\n  other:\n    extra: x\n")
	if _, err := Resolve(dir, "gen1", user); err != nil {
		t.Fatalf("other profile's own defaults should supply required `name`: %v", err)
	}
}

func TestResolve_OtherInstalledProfileValidated(t *testing.T) {
	dir := setupProfiles(t)
	writeProfile(t, dir, "other", "", "")
	user := writeFile(t, "user.yaml", "profiles:\n  other:\n    bogus: 1\n")
	_, err := Resolve(dir, "gen1", user)
	if err == nil || !strings.Contains(err.Error(), "profiles.other") {
		t.Fatalf("want error for profiles.other, got %v", err)
	}
}

func TestResolve_UninstalledProfileWarns(t *testing.T) {
	user := writeFile(t, "user.yaml", "profiles:\n  ghost:\n    anything: 1\n")
	res, err := Resolve(setupProfiles(t), "gen1", user)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "profiles.ghost: profile not installed") {
		t.Errorf("unexpected warnings %v", res.Warnings)
	}
}
