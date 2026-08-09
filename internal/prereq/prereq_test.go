package prereq

import (
	"testing"
)

func TestCheckAllPresent(t *testing.T) {
	missing := Check([]string{"sh", "echo"})
	if len(missing) != 0 {
		t.Errorf("expected no missing tools, got %v", missing)
	}
}

func TestCheckSomeMissing(t *testing.T) {
	missing := Check([]string{"sh", "this-tool-does-not-exist-xyz"})
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing tool, got %d: %v", len(missing), missing)
	}
	if missing[0] != "this-tool-does-not-exist-xyz" {
		t.Errorf("expected 'this-tool-does-not-exist-xyz', got %q", missing[0])
	}
}

func TestCheckEmpty(t *testing.T) {
	missing := Check(nil)
	if len(missing) != 0 {
		t.Errorf("expected no missing for nil input, got %v", missing)
	}
}
