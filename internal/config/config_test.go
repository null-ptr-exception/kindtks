package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExisting(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("registry: corp-registry.internal/\n"), 0644)

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Registry != "corp-registry.internal/" {
		t.Errorf("expected 'corp-registry.internal/', got %q", cfg.Registry)
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load should not error for missing file: %v", err)
	}
	if cfg.Registry != "" {
		t.Errorf("expected empty registry, got %q", cfg.Registry)
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
	if cfg.Registry != "" {
		t.Errorf("expected empty registry, got %q", cfg.Registry)
	}
}
