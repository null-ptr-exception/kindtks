package config

import (
	"os"
	"path/filepath"
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

func TestLoadRegistryAuth(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte(`images:
  cilium: registry.corp.com/cilium:v1
registryAuth:
  registry.corp.com:
    username: svc
    password: secret
`), 0644)

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.RegistryAuth == nil {
		t.Fatal("expected registryAuth to be loaded")
	}
	auth := cfg.RegistryAuth["registry.corp.com"]
	if auth == nil {
		t.Fatal("expected auth entry for registry.corp.com")
	}
	if auth.Username != "svc" || auth.Password != "secret" {
		t.Errorf("unexpected auth: %+v", auth)
	}
}

func TestMergeRegistryAuth(t *testing.T) {
	base := &Config{
		Images: map[string]string{"cilium": "quay.io/cilium/cilium:v1"},
		RegistryAuth: map[string]*RegistryAuth{
			"registry.a.com": {Username: "a", Password: "a-pass"},
		},
	}
	override := &Config{
		Images: map[string]string{},
		RegistryAuth: map[string]*RegistryAuth{
			"registry.a.com": {Username: "a-new", Password: "a-new-pass"},
			"registry.b.com": {Username: "b", Password: "b-pass"},
		},
	}

	merged := Merge(base, override)

	if merged.RegistryAuth["registry.a.com"].Username != "a-new" {
		t.Error("expected override to win for registry.a.com")
	}
	if merged.RegistryAuth["registry.b.com"] == nil {
		t.Error("expected registry.b.com from override")
	}
}
