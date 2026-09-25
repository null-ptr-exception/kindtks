package kindconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type testMount struct {
	HostPath      string `yaml:"hostPath"`
	ContainerPath string `yaml:"containerPath"`
	ReadOnly      bool   `yaml:"readOnly"`
}

type testNode struct {
	Role        string            `yaml:"role"`
	Labels      map[string]string `yaml:"labels"`
	ExtraMounts []testMount       `yaml:"extraMounts"`
}

type testCluster struct {
	Networking              map[string]any `yaml:"networking"`
	ContainerdConfigPatches []string       `yaml:"containerdConfigPatches"`
	Nodes                   []testNode     `yaml:"nodes"`
}

const multiNodeConfig = `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
nodes:
  - role: control-plane
    labels:
      topology.kubernetes.io/zone: dc1
  - role: worker
  - role: worker
`

const authPatch = `[plugins."io.containerd.grpc.v1.cri".registry.configs."registry.corp.com".auth]
  username = "u"
  password = "p"
`

func writeKindConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kind-config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readCluster(t *testing.T, path string) testCluster {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var c testCluster
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatalf("patched config is not valid YAML: %v\n%s", err, data)
	}
	return c
}

func hasConfigPathPatch(c testCluster) bool {
	for _, p := range c.ContainerdConfigPatches {
		if strings.Contains(p, `config_path = "/etc/containerd/certs.d"`) {
			return true
		}
	}
	return false
}

func TestPatch_NothingToDo(t *testing.T) {
	path := writeKindConfig(t, multiNodeConfig)

	got, cleanup, err := Patch(path, Options{})
	defer cleanup()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != path {
		t.Errorf("expected original path, got %q", got)
	}
}

func TestPatch_MountsOnEveryNode(t *testing.T) {
	path := writeKindConfig(t, multiNodeConfig)

	got, cleanup, err := Patch(path, Options{CertsDir: "/state/gen1/certs.d"})
	defer cleanup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == path {
		t.Fatal("expected a temp file, got the original path")
	}

	c := readCluster(t, got)
	if len(c.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(c.Nodes))
	}
	for i, n := range c.Nodes {
		want := testMount{HostPath: "/state/gen1/certs.d", ContainerPath: ContainerCertsDir, ReadOnly: true}
		if len(n.ExtraMounts) != 1 || n.ExtraMounts[0] != want {
			t.Errorf("node %d: extraMounts = %+v, want [%+v]", i, n.ExtraMounts, want)
		}
	}
	if c.Nodes[0].Labels["topology.kubernetes.io/zone"] != "dc1" {
		t.Error("node labels should be preserved")
	}
	if c.Networking["disableDefaultCNI"] != true {
		t.Error("networking should be preserved")
	}
	if !hasConfigPathPatch(c) {
		t.Errorf("expected config_path patch, got %v", c.ContainerdConfigPatches)
	}
}

func TestPatch_NoNodesAddsControlPlane(t *testing.T) {
	path := writeKindConfig(t, "kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\n")

	got, cleanup, err := Patch(path, Options{CertsDir: "/state/x/certs.d"})
	defer cleanup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c := readCluster(t, got)
	if len(c.Nodes) != 1 || c.Nodes[0].Role != "control-plane" {
		t.Fatalf("expected one control-plane node, got %+v", c.Nodes)
	}
	if len(c.Nodes[0].ExtraMounts) != 1 || c.Nodes[0].ExtraMounts[0].ContainerPath != ContainerCertsDir {
		t.Errorf("expected certs.d mount, got %+v", c.Nodes[0].ExtraMounts)
	}
}

func TestPatch_KeepsExistingMountsAndPatches(t *testing.T) {
	path := writeKindConfig(t, `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri"]
    sandbox_image = "pause:3.9"
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: /data
        containerPath: /data
`)

	got, cleanup, err := Patch(path, Options{CertsDir: "/state/x/certs.d", AuthPatch: authPatch})
	defer cleanup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c := readCluster(t, got)
	mounts := c.Nodes[0].ExtraMounts
	if len(mounts) != 2 || mounts[0].ContainerPath != "/data" || mounts[1].ContainerPath != ContainerCertsDir {
		t.Errorf("expected existing mount kept and certs.d appended, got %+v", mounts)
	}
	if len(c.ContainerdConfigPatches) != 3 {
		t.Fatalf("expected 3 patches (existing, config_path, auth), got %v", c.ContainerdConfigPatches)
	}
	if !strings.Contains(c.ContainerdConfigPatches[0], "sandbox_image") {
		t.Error("existing patch should stay first")
	}
	if !hasConfigPathPatch(c) {
		t.Error("expected config_path patch")
	}
	if !strings.Contains(c.ContainerdConfigPatches[2], `username = "u"`) {
		t.Error("expected auth patch last")
	}
}

func TestPatch_RejectsConflictingMount(t *testing.T) {
	path := writeKindConfig(t, `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
    extraMounts:
      - hostPath: /elsewhere
        containerPath: /etc/containerd/certs.d/
`)

	_, cleanup, err := Patch(path, Options{CertsDir: "/state/x/certs.d"})
	defer cleanup()

	if err == nil || !strings.Contains(err.Error(), "nodes[1]") {
		t.Fatalf("expected conflict error naming nodes[1], got %v", err)
	}
}

func TestPatch_AuthOnly(t *testing.T) {
	path := writeKindConfig(t, multiNodeConfig)

	got, cleanup, err := Patch(path, Options{AuthPatch: authPatch})
	defer cleanup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c := readCluster(t, got)
	if hasConfigPathPatch(c) {
		t.Error("auth-only must not set config_path")
	}
	for i, n := range c.Nodes {
		if len(n.ExtraMounts) != 0 {
			t.Errorf("node %d: auth-only must not add mounts, got %+v", i, n.ExtraMounts)
		}
	}
	if len(c.ContainerdConfigPatches) != 1 || !strings.Contains(c.ContainerdConfigPatches[0], `password = "p"`) {
		t.Errorf("expected only the auth patch, got %v", c.ContainerdConfigPatches)
	}
}

func TestPatch_CleanupRemovesTempFile(t *testing.T) {
	path := writeKindConfig(t, multiNodeConfig)

	got, cleanup, err := Patch(path, Options{CertsDir: "/state/x/certs.d"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleanup()

	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Errorf("expected temp file removed, stat err = %v", err)
	}
}
