package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/profile"
)

func TestRunProfileFunc_ExportsProfileConfig(t *testing.T) {
	profilesDir := t.TempDir()
	dir := filepath.Join(profilesDir, "demo")
	os.MkdirAll(dir, 0755)
	out := filepath.Join(t.TempDir(), "out")
	os.WriteFile(filepath.Join(dir, "install.sh"), []byte(`REQUIRES=""
create() {
  env > "$OUT.env"
  cat "$KINDTKS_PROFILE_CONFIG" > "$OUT.json"
}
delete() { :; }
`), 0644)
	t.Setenv("OUT", out)

	p, err := profile.Load(profilesDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	res := &config.Resolved{
		Config:  &config.Config{Images: map[string]string{"cilium": "quay.io/cilium/cilium:v1"}},
		Profile: map[string]any{"gateway": map[string]any{"extraHosts": []any{"*.example.net"}}},
	}

	if err := runProfileFunc(p, "create", res, "/tmp/kind.yaml"); err != nil {
		t.Fatal(err)
	}

	envData, _ := os.ReadFile(out + ".env")
	env := string(envData)
	exe, _ := os.Executable()
	for _, want := range []string{"KINDTKS_BIN=" + exe, "IMAGE_CILIUM=quay.io/cilium/cilium:v1", "KINDTKS_KIND_CONFIG=/tmp/kind.yaml"} {
		if !strings.Contains(env, want+"\n") {
			t.Errorf("env missing %q", want)
		}
	}

	var got map[string]any
	data, _ := os.ReadFile(out + ".json")
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("profile config is not JSON: %v (%s)", err, data)
	}
	if got["gateway"].(map[string]any)["extraHosts"].([]any)[0] != "*.example.net" {
		t.Errorf("unexpected profile config %s", data)
	}

	var cfgPath string
	for _, line := range strings.Split(env, "\n") {
		if v, ok := strings.CutPrefix(line, "KINDTKS_PROFILE_CONFIG="); ok {
			cfgPath = v
		}
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("profile config %q should be removed after the run", cfgPath)
	}
}
