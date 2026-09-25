package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rophy/kindtks/internal/config"
)

const testKindConfig = `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
`

func writeTestKindConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kind-config.yaml")
	os.WriteFile(path, []byte(testKindConfig), 0644)
	return path
}

func TestPrepareKindConfig_NothingToDo(t *testing.T) {
	kindCfg := writeTestKindConfig(t)
	stateDir := t.TempDir()
	stale := filepath.Join(stateDir, "certs.d", "old.example.com")
	os.MkdirAll(stale, 0755)

	got, cleanup, err := prepareKindConfig(kindCfg, &config.Config{}, stateDir)
	defer cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != kindCfg {
		t.Errorf("expected original kind config, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "certs.d")); !os.IsNotExist(err) {
		t.Errorf("expected stale certs.d removed, stat err = %v", err)
	}
}

func TestPrepareKindConfig_WritesHostsAndPatches(t *testing.T) {
	kindCfg := writeTestKindConfig(t)
	stateDir := t.TempDir()
	cfg := &config.Config{
		Images: map[string]string{"a": "registry.airgap:5000/foo:v1"},
		Registries: map[string]*config.Registry{
			"docker.io":         {Hosts: "server = \"https://registry-1.docker.io\"\n"},
			"registry.corp.com": {Auth: &config.RegistryAuth{Username: "u", Password: "p"}},
		},
	}

	got, cleanup, err := prepareKindConfig(kindCfg, cfg, stateDir)
	defer cleanup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	certsDir := filepath.Join(stateDir, "certs.d")
	for _, reg := range []string{"docker.io", "registry.airgap:5000"} {
		if _, err := os.Stat(filepath.Join(certsDir, reg, "hosts.toml")); err != nil {
			t.Errorf("expected hosts.toml for %s: %v", reg, err)
		}
	}
	if _, err := os.Stat(filepath.Join(certsDir, "registry.corp.com")); !os.IsNotExist(err) {
		t.Error("auth-only registry must not get a host file")
	}

	data, _ := os.ReadFile(got)
	content := string(data)
	if strings.Count(content, "hostPath: "+certsDir) != 2 {
		t.Errorf("expected certs.d mounted on both nodes:\n%s", content)
	}
	if !strings.Contains(content, `config_path = "/etc/containerd/certs.d"`) {
		t.Error("expected config_path patch")
	}
	if !strings.Contains(content, `registry.configs."registry.corp.com".auth`) {
		t.Error("expected auth patch")
	}
	if strings.Contains(content, "mirrors") || strings.Contains(content, ".tls]") {
		t.Error("legacy mirrors/tls patches must not be emitted")
	}
}
