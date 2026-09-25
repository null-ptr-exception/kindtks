package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadExisting(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("images:\n  cilium: corp/cilium:v1\n"), 0644)

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Images["cilium"] != "corp/cilium:v1" {
		t.Errorf("expected 'corp/cilium:v1', got %q", cfg.Images["cilium"])
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load should not error for missing file: %v", err)
	}
	if len(cfg.Images) != 0 {
		t.Errorf("expected no images, got %v", cfg.Images)
	}
}

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte(""), 0644)

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Images) != 0 {
		t.Errorf("expected no images, got %v", cfg.Images)
	}
}

func TestMerge(t *testing.T) {
	base := &Config{Images: map[string]string{
		"cilium": "quay.io/cilium/cilium:v1.13.10",
		"vault":  "docker.io/hashicorp/vault:2.0.3",
	}}
	override := &Config{Images: map[string]string{
		"cilium": "corp.example.com/cilium:v1.13.10",
	}}

	merged := Merge(base, override)

	if merged.Images["cilium"] != "corp.example.com/cilium:v1.13.10" {
		t.Errorf("expected override for cilium, got %q", merged.Images["cilium"])
	}
	if merged.Images["vault"] != "docker.io/hashicorp/vault:2.0.3" {
		t.Errorf("expected base for vault, got %q", merged.Images["vault"])
	}
}

func TestMergeNilMaps(t *testing.T) {
	base := &Config{}
	override := &Config{
		Images: map[string]string{"cilium": "corp/cilium:v1"},
	}

	merged := Merge(base, override)

	if merged.Images["cilium"] != "corp/cilium:v1" {
		t.Errorf("expected cilium from override, got %q", merged.Images["cilium"])
	}
}

func TestMergeNilOverride(t *testing.T) {
	base := &Config{
		Images: map[string]string{"cilium": "quay.io/cilium:v1"},
	}
	override := &Config{}

	merged := Merge(base, override)

	if merged.Images["cilium"] != "quay.io/cilium:v1" {
		t.Errorf("expected base cilium preserved, got %q", merged.Images["cilium"])
	}
}

func TestLoadRegistries(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte(`registries:
  quay.io:
    hosts: |
      server = "https://quay.io"
      [host."http://zot:5000/v2/quay.io"]
        capabilities = ["pull", "resolve"]
        override_path = true
  registry.corp.com:
    auth:
      username: svc
      password: secret
`), 0644)

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !strings.Contains(cfg.Registries["quay.io"].Hosts, `override_path = true`) {
		t.Errorf("unexpected quay.io hosts: %q", cfg.Registries["quay.io"].Hosts)
	}
	auth := cfg.Registries["registry.corp.com"].Auth
	if auth == nil || auth.Username != "svc" || auth.Password != "secret" {
		t.Errorf("unexpected auth: %+v", auth)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("imagez:\n  cilium: x\n"), 0644)

	if _, err := Load(configFile); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestLoadRejectsInvalidHostsTOML(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("registries:\n  quay.io:\n    hosts: \"server = \"\n"), 0644)

	_, err := Load(configFile)
	if err == nil || !strings.Contains(err.Error(), "quay.io") {
		t.Fatalf("expected TOML error naming quay.io, got %v", err)
	}
}

func TestLoadRejectsBadRegistryName(t *testing.T) {
	for _, name := range []string{`""`, `a/b`, `..`, `a..b`} {
		dir := t.TempDir()
		configFile := filepath.Join(dir, "config.yaml")
		os.WriteFile(configFile, []byte("registries:\n  "+name+":\n    hosts: \"x = 1\"\n"), 0644)

		if _, err := Load(configFile); err == nil {
			t.Errorf("expected error for registry name %s", name)
		}
	}
}

func TestMergeRegistriesPerField(t *testing.T) {
	base := &Config{Registries: map[string]*Registry{
		"quay.io":  {Hosts: "base-hosts"},
		"ghcr.io":  {Hosts: "ghcr-hosts"},
		"corp.com": {Auth: &RegistryAuth{Username: "a", Password: "a"}},
	}}
	override := &Config{Registries: map[string]*Registry{
		"quay.io":  {Auth: &RegistryAuth{Username: "u", Password: "p"}},
		"ghcr.io":  {Hosts: "override-hosts"},
		"corp.com": {Auth: &RegistryAuth{Username: "b", Password: "b"}},
	}}

	merged := Merge(base, override)

	if r := merged.Registries["quay.io"]; r.Hosts != "base-hosts" || r.Auth == nil || r.Auth.Username != "u" {
		t.Errorf("quay.io should keep base hosts and take override auth, got %+v", r)
	}
	if merged.Registries["ghcr.io"].Hosts != "override-hosts" {
		t.Errorf("ghcr.io hosts should be overridden, got %q", merged.Registries["ghcr.io"].Hosts)
	}
	if merged.Registries["corp.com"].Auth.Username != "b" {
		t.Errorf("corp.com auth should be overridden")
	}
}

func TestMergeDoesNotModifyInputs(t *testing.T) {
	base := &Config{Registries: map[string]*Registry{"quay.io": {Hosts: "base-hosts"}}}
	override := &Config{Registries: map[string]*Registry{"quay.io": {Hosts: "override-hosts"}}}

	Merge(base, override)

	if base.Registries["quay.io"].Hosts != "base-hosts" {
		t.Error("Merge must not modify base")
	}
}

func TestLoadRejectsRegistryAuth(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("registryAuth:\n  r.io:\n    username: u\n    password: p\n"), 0644)

	_, err := Load(configFile)
	if err == nil || !strings.Contains(err.Error(), "registryAuth") {
		t.Fatalf("expected error mentioning registryAuth, got %v", err)
	}
}
