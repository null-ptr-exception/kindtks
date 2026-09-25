package kindconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestWriteHostsDir_PreservesDirInode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs.d")

	if err := WriteHostsDir(dir, map[string]string{"docker.io": "a = 1\n"}); err != nil {
		t.Fatalf("WriteHostsDir: %v", err)
	}
	fi1, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat after first write: %v", err)
	}

	if err := WriteHostsDir(dir, map[string]string{"quay.io": "b = 2\n"}); err != nil {
		t.Fatalf("WriteHostsDir: %v", err)
	}
	fi2, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat after second write: %v", err)
	}

	if !os.SameFile(fi1, fi2) {
		t.Error("expected certs.d dir inode to survive re-write (bind mounts rely on it)")
	}
	if _, err := os.Stat(filepath.Join(dir, "docker.io")); !os.IsNotExist(err) {
		t.Errorf("expected stale docker.io removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "quay.io", "hosts.toml")); err != nil {
		t.Errorf("expected quay.io hosts.toml: %v", err)
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

func TestWriteHostsDir_RejectsBadRegistryName(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../x", "a/b", "a\\b", "a..b/.."} {
		dir := filepath.Join(t.TempDir(), "certs.d")
		err := WriteHostsDir(dir, map[string]string{name: "x = 1\n"})
		if err == nil {
			t.Errorf("expected error for registry name %q", name)
			continue
		}
		if name != "" && !strings.Contains(err.Error(), fmt.Sprintf("%q", name)) {
			t.Errorf("expected error naming bad registry %q, got %v", name, err)
		}
	}
}

func TestWriteHostsDir_Perms(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "certs.d")

	if err := WriteHostsDir(dir, map[string]string{"docker.io": "a = 1\n"}); err != nil {
		t.Fatalf("WriteHostsDir: %v", err)
	}

	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0700 {
		t.Errorf("dir perm = %o, want 0700", perm)
	}
	fi, err = os.Stat(filepath.Join(dir, "docker.io"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0700 {
		t.Errorf("registry dir perm = %o, want 0700", perm)
	}
	fi, err = os.Stat(filepath.Join(dir, "docker.io", "hosts.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0600 {
		t.Errorf("hosts.toml perm = %o, want 0600", perm)
	}
}
