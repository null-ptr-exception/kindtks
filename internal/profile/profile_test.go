package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	script := `REQUIRES="kind helm kubectl"

create() {
  echo "creating"
}

delete() {
  echo "deleting"
}
`
	err := os.WriteFile(filepath.Join(dir, "testprofile.sh"), []byte(script), 0644)
	if err != nil {
		t.Fatal(err)
	}

	p, err := Load(dir, "testprofile")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if p.Name != "testprofile" {
		t.Errorf("expected name 'testprofile', got %q", p.Name)
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

func TestListAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "alpha.sh"), []byte(`REQUIRES="kind"`), 0644)
	os.WriteFile(filepath.Join(dir, "beta.sh"), []byte(`REQUIRES="kind"`), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a profile"), 0644)

	names, err := ListAll(dir)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 profiles, got %d: %v", len(names), names)
	}
}
