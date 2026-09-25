package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDocument_Missing(t *testing.T) {
	doc, err := LoadDocument(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || len(doc) != 0 {
		t.Fatalf("want empty doc, got %v, %v", doc, err)
	}
}

func TestLoadDocument_Empty(t *testing.T) {
	doc, err := LoadDocument(writeFile(t, "c.yaml", ""))
	if err != nil || len(doc) != 0 {
		t.Fatalf("want empty doc, got %v, %v", doc, err)
	}
}

func TestLoadDocument_NormalizesValues(t *testing.T) {
	doc, err := LoadDocument(writeFile(t, "c.yaml", "a:\n  n: 3\n  list: [x, true]\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := doc["a"].(map[string]any)
	if n, ok := a["n"].(json.Number); !ok || n.String() != "3" {
		t.Errorf("want json.Number 3, got %#v", a["n"])
	}
	list := a["list"].([]any)
	if list[0] != "x" || list[1] != true {
		t.Errorf("unexpected list %#v", list)
	}
}

func TestLoadDocument_RejectsNonMapping(t *testing.T) {
	_, err := LoadDocument(writeFile(t, "c.yaml", "- a\n- b\n"))
	if err == nil || !strings.Contains(err.Error(), "mapping") {
		t.Fatalf("want mapping error, got %v", err)
	}
}

func TestLoadDocument_RejectsNonStringKey(t *testing.T) {
	_, err := LoadDocument(writeFile(t, "c.yaml", "1: a\n"))
	if err == nil || !strings.Contains(err.Error(), "mapping key 1 is not a string") {
		t.Fatalf("want non-string key error, got %v", err)
	}
}
