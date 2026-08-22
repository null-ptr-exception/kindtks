package kindconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rophy/kindtks/internal/config"
)

const baseKindConfig = `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
nodes:
  - role: control-plane
`

func TestPatchForRegistries_NoRegistries(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "kind-config.yaml")
	os.WriteFile(cfgFile, []byte(baseKindConfig), 0644)

	path, cleanup, err := PatchForRegistries(cfgFile, nil, nil)
	defer cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != cfgFile {
		t.Errorf("expected original path, got %q", path)
	}
}

func TestPatchForRegistries_AddsPatches(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "kind-config.yaml")
	os.WriteFile(cfgFile, []byte(baseKindConfig), 0644)

	path, cleanup, err := PatchForRegistries(cfgFile, []string{"registry.airgap:5000"}, nil)
	defer cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == cfgFile {
		t.Error("expected a temp file path, got original")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading patched file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "containerdConfigPatches") {
		t.Error("patched config should contain containerdConfigPatches")
	}
	if !strings.Contains(content, "registry.airgap:5000") {
		t.Error("patched config should reference the registry")
	}
	if !strings.Contains(content, "insecure_skip_verify") {
		t.Error("patched config should contain insecure_skip_verify")
	}
	// Original fields preserved
	if !strings.Contains(content, "disableDefaultCNI") {
		t.Error("patched config should preserve original fields")
	}
}

func TestPatchForRegistries_ExistingPatches(t *testing.T) {
	configWithPatch := `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."existing:5000"]
    endpoint = ["http://existing:5000"]
nodes:
  - role: control-plane
`
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "kind-config.yaml")
	os.WriteFile(cfgFile, []byte(configWithPatch), 0644)

	path, cleanup, err := PatchForRegistries(cfgFile, []string{"registry.airgap:5000"}, nil)
	defer cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "existing:5000") {
		t.Error("should preserve existing patch")
	}
	if !strings.Contains(content, "registry.airgap:5000") {
		t.Error("should add new registry patch")
	}
}

func TestPatchForRegistries_AuthOnly(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "kind-config.yaml")
	os.WriteFile(cfgFile, []byte(baseKindConfig), 0644)

	auth := map[string]*config.RegistryAuth{
		"registry.corp.com": {Username: "user", Password: "pass"},
	}
	path, cleanup, err := PatchForRegistries(cfgFile, nil, auth)
	defer cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == cfgFile {
		t.Error("expected a temp file path, got original")
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "containerdConfigPatches") {
		t.Error("patched config should contain containerdConfigPatches")
	}
	if !strings.Contains(content, `username = "user"`) {
		t.Error("patched config should contain auth username")
	}
	if !strings.Contains(content, "disableDefaultCNI") {
		t.Error("patched config should preserve original fields")
	}
}

func TestPatchForRegistries_InsecureAndAuth(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "kind-config.yaml")
	os.WriteFile(cfgFile, []byte(baseKindConfig), 0644)

	auth := map[string]*config.RegistryAuth{
		"registry.airgap:5000": {Username: "user", Password: "pass"},
	}
	path, cleanup, err := PatchForRegistries(cfgFile, []string{"registry.airgap:5000"}, auth)
	defer cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "insecure_skip_verify") {
		t.Error("patched config should contain insecure config")
	}
	if !strings.Contains(content, `username = "user"`) {
		t.Error("patched config should contain auth config")
	}
	if !strings.Contains(content, "disableDefaultCNI") {
		t.Error("patched config should preserve original fields")
	}
}
