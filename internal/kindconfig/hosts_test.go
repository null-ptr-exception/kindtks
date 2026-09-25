package kindconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteHostsDir_WritesFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs.d")
	files := map[string]string{
		"docker.io":            "server = \"https://registry-1.docker.io\"\n",
		"registry.airgap:5000": "server = \"http://registry.airgap:5000\"\n",
	}

	if err := WriteHostsDir(dir, files); err != nil {
		t.Fatalf("WriteHostsDir: %v", err)
	}

	for reg, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, reg, "hosts.toml"))
		if err != nil {
			t.Fatalf("reading %s: %v", reg, err)
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", reg, got, want)
		}
	}
}

func TestWriteHostsDir_RemovesStaleEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs.d")
	stale := filepath.Join(dir, "old.example.com", "hosts.toml")
	os.MkdirAll(filepath.Dir(stale), 0755)
	os.WriteFile(stale, []byte("stale"), 0644)

	if err := WriteHostsDir(dir, map[string]string{"quay.io": "x = 1\n"}); err != nil {
		t.Fatalf("WriteHostsDir: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("expected stale entry removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "quay.io", "hosts.toml")); err != nil {
		t.Errorf("expected quay.io hosts.toml: %v", err)
	}
}
