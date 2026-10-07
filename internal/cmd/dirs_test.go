package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KINDTKS_PROFILES_DIR", "")
	if got, want := profileDir(), filepath.Join(home, ".local", "share", "kindtks", "profiles"); got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
	t.Setenv("KINDTKS_PROFILES_DIR", "/p")
	if got := profileDir(); got != "/p" {
		t.Errorf("override: got %q", got)
	}
}

func TestChartsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KINDTKS_CHARTS_DIR", "")
	if got, want := chartsDir(), filepath.Join(home, ".local", "share", "kindtks", "charts"); got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
	t.Setenv("KINDTKS_CHARTS_DIR", "/c")
	if got := chartsDir(); got != "/c" {
		t.Errorf("override: got %q", got)
	}
}

func TestPrependToolsDir_Once(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	prependToolsDir()
	prependToolsDir()
	want := filepath.Join(home, ".local", "share", "kindtks", "bin") + ":/usr/bin:/bin"
	if got := os.Getenv("PATH"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNoInstallCommand(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "install" {
			t.Fatal("install command still registered")
		}
	}
}
