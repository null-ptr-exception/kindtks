package main

import (
	"os"
	"strings"
	"testing"
)

func TestVersionComesFromVersionFile(t *testing.T) {
	want, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if got := buildVersion(); got != strings.TrimSpace(string(want)) {
		t.Errorf("buildVersion() = %q, want %q", got, strings.TrimSpace(string(want)))
	}
}
