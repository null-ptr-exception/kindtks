package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

// dataDir holds the bundled tools, profiles and charts that the installer
// copies out of the kindtks image.
func dataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "kindtks")
}

// profileDir is $KINDTKS_PROFILES_DIR when set (tests, development), else
// the installed profiles.
func profileDir() string {
	if dir := os.Getenv("KINDTKS_PROFILES_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(dataDir(), "profiles")
}

// chartsDir is exported to profile scripts as KINDTKS_CHARTS_DIR.
func chartsDir() string {
	if dir := os.Getenv("KINDTKS_CHARTS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(dataDir(), "charts")
}

// prependToolsDir puts the bundled kind, kubectl and helm first on PATH, so
// REQUIRES checks and profile scripts use the versions kindtks was tested with.
func prependToolsDir() {
	bin := filepath.Join(dataDir(), "bin")
	path := os.Getenv("PATH")
	if first, _, _ := strings.Cut(path, string(os.PathListSeparator)); first == bin {
		return
	}
	os.Setenv("PATH", bin+string(os.PathListSeparator)+path)
}
