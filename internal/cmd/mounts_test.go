package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func okDocker() error { return nil }

func inImage(t *testing.T) string {
	t.Helper()
	t.Setenv("KINDTKS_IMAGE", "ghcr.io/null-ptr-exception/kindtks:0.6.0")
	t.Setenv("XDG_STATE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestCheckMounts_SkippedOutsideImage(t *testing.T) {
	t.Setenv("KINDTKS_IMAGE", "")
	t.Setenv("HOME", "/")
	if err := checkMounts(func() error { return errors.New("no docker") }); err != nil {
		t.Fatal(err)
	}
}

func TestCheckMounts_AllPresent(t *testing.T) {
	home := inImage(t)
	os.MkdirAll(filepath.Join(home, ".kube"), 0755)
	os.MkdirAll(filepath.Join(home, ".local", "state", "kindtks"), 0755)
	if err := checkMounts(okDocker); err != nil {
		t.Fatal(err)
	}
}

func TestCheckMounts_Failures(t *testing.T) {
	home := inImage(t)
	err := checkMounts(func() error { return errors.New("permission denied") })
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	for _, want := range []string{
		filepath.Join(home, ".kube"),
		filepath.Join(home, ".local", "state", "kindtks"),
		"docker socket",
		"permission denied",
		"docker run --rm ghcr.io/null-ptr-exception/kindtks:0.6.0",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestCheckMounts_RootHome(t *testing.T) {
	inImage(t)
	t.Setenv("HOME", "/")
	err := checkMounts(okDocker)
	if err == nil || !strings.Contains(err.Error(), `-e HOME="$HOME"`) {
		t.Fatalf("want HOME hint, got %v", err)
	}
}

func TestCheckConfigVisible(t *testing.T) {
	inImage(t)
	err := checkConfigVisible("/nonexistent/my.yaml")
	if err == nil || !strings.Contains(err.Error(), "mount") {
		t.Fatalf("want mount hint, got %v", err)
	}
	f := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(f, nil, 0644)
	if err := checkConfigVisible(f); err != nil {
		t.Fatal(err)
	}
	if err := checkConfigVisible(""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KINDTKS_IMAGE", "")
	if err := checkConfigVisible("/nonexistent/my.yaml"); err != nil {
		t.Fatalf("outside image the normal missing-file error applies later, got %v", err)
	}
}
