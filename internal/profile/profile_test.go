package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	profileDir := filepath.Join(dir, "testprofile")
	os.MkdirAll(profileDir, 0755)

	script := `REQUIRES="kind helm kubectl"

create() {
  echo "creating"
}

delete() {
  echo "deleting"
}
`
	os.WriteFile(filepath.Join(profileDir, "install.sh"), []byte(script), 0644)

	p, err := Load(dir, "testprofile")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if p.Name != "testprofile" {
		t.Errorf("expected name 'testprofile', got %q", p.Name)
	}
	if p.Dir != profileDir {
		t.Errorf("expected dir %q, got %q", profileDir, p.Dir)
	}
	if len(p.Requires) != 3 {
		t.Fatalf("expected 3 requires, got %d", len(p.Requires))
	}
	if p.Requires[0] != "kind" || p.Requires[1] != "helm" || p.Requires[2] != "kubectl" {
		t.Errorf("unexpected requires: %v", p.Requires)
	}
}

func TestLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestLoadMissingInstallSh(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "emptyprofile"), 0755)
	_, err := Load(dir, "emptyprofile")
	if err == nil {
		t.Fatal("expected error for profile dir without install.sh")
	}
}

func TestListAll(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "alpha"), 0755)
	os.WriteFile(filepath.Join(dir, "alpha", "install.sh"), []byte(`REQUIRES="kind"`), 0644)
	os.MkdirAll(filepath.Join(dir, "beta"), 0755)
	os.WriteFile(filepath.Join(dir, "beta", "install.sh"), []byte(`REQUIRES="kind"`), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a profile"), 0644)

	names, err := ListAll(dir)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 profiles, got %d: %v", len(names), names)
	}
}
