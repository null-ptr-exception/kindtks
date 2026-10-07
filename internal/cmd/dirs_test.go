package cmd

import (
	"strings"
	"testing"
)

func TestProfileDir_FromEnv(t *testing.T) {
	t.Setenv("KINDTKS_PROFILES_DIR", "/profiles")
	dir, err := profileDir()
	if err != nil || dir != "/profiles" {
		t.Fatalf("got %q, %v", dir, err)
	}
}

func TestProfileDir_Unset(t *testing.T) {
	t.Setenv("KINDTKS_PROFILES_DIR", "")
	_, err := profileDir()
	if err == nil || !strings.Contains(err.Error(), "docker run --rm "+imageRef) {
		t.Fatalf("want hint to run the image, got %v", err)
	}
}

func TestReadme_SubstitutesImage(t *testing.T) {
	old := projectReadme
	t.Cleanup(func() { projectReadme = old })
	projectReadme = "run: docker run --rm " + imageRef + " list\n"

	t.Setenv("KINDTKS_IMAGE", "")
	if got := readme(); got != projectReadme {
		t.Errorf("outside image: got %q", got)
	}
	t.Setenv("KINDTKS_IMAGE", "ghcr.io/null-ptr-exception/kindtks:0.6.0")
	if got := readme(); got != "run: docker run --rm ghcr.io/null-ptr-exception/kindtks:0.6.0 list\n" {
		t.Errorf("inside image: got %q", got)
	}
}

func TestNoInstallCommand(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "install" {
			t.Fatal("install command still registered")
		}
	}
}
