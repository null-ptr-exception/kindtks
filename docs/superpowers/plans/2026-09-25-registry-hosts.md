# Per-registry hosts.toml Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `registryAuth` with a `registries` map whose entries can carry raw containerd `hosts.toml` content and/or credentials, and switch all registry configuration (including private-registry auto-trust) to containerd's `config_path` + `certs.d` mechanism.

**Architecture:** `internal/config` loads/validates/merges `registries` and computes the host files and auth patch. `internal/kindconfig` writes the host files to a persistent per-profile state dir and patches the kind config (a `config_path` containerd patch plus a read-only `extraMounts` entry on every node). `internal/cmd` wires both into `create` and removes the state dir on `delete`.

**Tech Stack:** Go 1.24, `gopkg.in/yaml.v3` (yaml.Node editing, strict decoding), `github.com/BurntSushi/toml` v1.6.0 (TOML syntax validation), bats for e2e.

**Spec:** `docs/superpowers/specs/2026-09-25-registry-hosts-design.md`

## Global Constraints

- containerd host dir inside nodes: `/etc/containerd/certs.d` (mounted read-only).
- containerd patch enabling it, verbatim:
  ```
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
  ```
- Never emit `registry.mirrors` or `registry.configs.<reg>.tls`; containerd rejects them when `config_path` is set.
- Credentials stay in `[plugins."io.containerd.grpc.v1.cri".registry.configs."<reg>".auth]`.
- Generated host files: `~/.local/state/kindtks/<profile>/certs.d/<registry>/hosts.toml` (`$XDG_STATE_HOME` overrides `~/.local/state`).
- No host files to write → no mount, no `config_path` patch; the profile's kind config is used unchanged.
- Commit messages: `<type>: <description>`, no AI attribution lines.
- Never put private hostnames/domains in committed content.
- Run Go commands with mise if `MISE_SHELL` is unset: `eval "$(~/.local/bin/mise activate bash)"`.

## File Structure

| File | Responsibility |
|---|---|
| `internal/config/config.go` (modify) | `Registry` type, `Registries` field, strict `Load`, `Validate`, `Merge` |
| `internal/config/registry.go` (modify) | `HostsFiles`, `AuthRegistries`, `AuthPatch`; drop `ContainerdPatch` |
| `internal/kindconfig/hosts.go` (create) | `WriteHostsDir` |
| `internal/kindconfig/registries.go` (create) | `Patch` + `Options` (yaml.Node editing) |
| `internal/kindconfig/patch.go` (delete) | old `PatchForRegistries` |
| `internal/cmd/registries.go` (create) | `profileStateDir`, `prepareKindConfig` |
| `internal/cmd/create.go`, `delete.go` (modify) | wiring |
| `README.md`, `profiles/gen1/README.md`, `test/e2e/airgap-gen1.bats` (modify) | new config format |
| `test/e2e/registry-hosts.bats` (create) | self-contained e2e with a pull-through mirror |

---

### Task 1: kindconfig — host files writer and kind config patcher

**Files:**
- Create: `internal/kindconfig/hosts.go`, `internal/kindconfig/registries.go`
- Test: `internal/kindconfig/hosts_test.go`, `internal/kindconfig/registries_test.go`

**Interfaces:**
- Consumes: nothing (plain strings/maps; independent of `internal/config`).
- Produces:
  - `func WriteHostsDir(dir string, files map[string]string) error`
  - `type Options struct { CertsDir string; AuthPatch string }`
  - `func Patch(kindConfigPath string, opts Options) (string, func(), error)`
  - `const ContainerCertsDir = "/etc/containerd/certs.d"`

The old `PatchForRegistries` in `patch.go` stays untouched in this task (removed in Task 3).

- [ ] **Step 1: Write failing tests for `WriteHostsDir`**

`internal/kindconfig/hosts_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/kindconfig/ -run TestWriteHostsDir -v`
Expected: FAIL, `undefined: WriteHostsDir`.

- [ ] **Step 3: Implement `WriteHostsDir`**

`internal/kindconfig/hosts.go`:

```go
package kindconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteHostsDir replaces dir with one <registry>/hosts.toml per entry in files.
func WriteHostsDir(dir string, files map[string]string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clearing %s: %w", dir, err)
	}
	for reg, content := range files {
		regDir := filepath.Join(dir, reg)
		if err := os.MkdirAll(regDir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(regDir, "hosts.toml"), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/kindconfig/ -run TestWriteHostsDir -v`
Expected: PASS.

- [ ] **Step 5: Write failing tests for `Patch`**

`internal/kindconfig/registries_test.go`:

```go
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
```

- [ ] **Step 6: Run to verify failure**

Run: `go test ./internal/kindconfig/ -run TestPatch_ -v`
Expected: FAIL, `undefined: Patch`, `undefined: Options`, `undefined: ContainerCertsDir`.

- [ ] **Step 7: Implement `Patch`**

`internal/kindconfig/registries.go`:

```go
package kindconfig

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ContainerCertsDir is where containerd reads per-registry hosts.toml files
// inside each Kind node.
const ContainerCertsDir = "/etc/containerd/certs.d"

const configPathPatch = `[plugins."io.containerd.grpc.v1.cri".registry]
  config_path = "/etc/containerd/certs.d"`

// Options describes the registry changes to apply to a kind config.
type Options struct {
	// CertsDir is a host directory of <registry>/hosts.toml files, mounted
	// read-only at ContainerCertsDir on every node. Empty means none.
	CertsDir string
	// AuthPatch is an extra containerdConfigPatches entry holding registry
	// credentials. Empty means none.
	AuthPatch string
}

// Patch applies opts to the kind config at kindConfigPath. It returns the
// original path when there is nothing to apply, otherwise a temp file with the
// patched config and a cleanup func that removes it.
func Patch(kindConfigPath string, opts Options) (string, func(), error) {
	noop := func() {}

	if opts.CertsDir == "" && opts.AuthPatch == "" {
		return kindConfigPath, noop, nil
	}

	data, err := os.ReadFile(kindConfigPath)
	if err != nil {
		return "", noop, fmt.Errorf("reading kind config: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", noop, fmt.Errorf("parsing kind config: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", noop, fmt.Errorf("kind config %s is not a YAML mapping", kindConfigPath)
	}
	root := doc.Content[0]

	var patches []string
	if opts.CertsDir != "" {
		if err := addCertsMounts(root, opts.CertsDir); err != nil {
			return "", noop, err
		}
		patches = append(patches, configPathPatch)
	}
	if opts.AuthPatch != "" {
		patches = append(patches, opts.AuthPatch)
	}
	appendPatches(root, patches)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", noop, fmt.Errorf("marshalling patched kind config: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "kindtks-*")
	if err != nil {
		return "", noop, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	tmpFile := filepath.Join(tmpDir, "kind-config.yaml")
	if err := os.WriteFile(tmpFile, out, 0644); err != nil {
		cleanup()
		return "", noop, err
	}

	return tmpFile, cleanup, nil
}

// addCertsMounts adds a read-only mount of certsDir at ContainerCertsDir to
// every node, creating kind's default single control-plane node when the
// config has no nodes.
func addCertsMounts(root *yaml.Node, certsDir string) error {
	nodes := mapGet(root, "nodes")
	if nodes == nil {
		cp := &yaml.Node{Kind: yaml.MappingNode}
		mapSet(cp, "role", str("control-plane"))
		nodes = &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{cp}}
		mapSet(root, "nodes", nodes)
	}

	for i, n := range nodes.Content {
		if n.Kind != yaml.MappingNode {
			return fmt.Errorf("kind config nodes[%d] is not a mapping", i)
		}
		mounts := mapGet(n, "extraMounts")
		if mounts == nil {
			mounts = &yaml.Node{Kind: yaml.SequenceNode}
			mapSet(n, "extraMounts", mounts)
		}
		for _, m := range mounts.Content {
			if cp := mapGet(m, "containerPath"); cp != nil && path.Clean(cp.Value) == ContainerCertsDir {
				return fmt.Errorf("kind config nodes[%d] already mounts %s", i, ContainerCertsDir)
			}
		}
		mount := &yaml.Node{Kind: yaml.MappingNode}
		mapSet(mount, "hostPath", str(certsDir))
		mapSet(mount, "containerPath", str(ContainerCertsDir))
		mapSet(mount, "readOnly", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		mounts.Content = append(mounts.Content, mount)
	}
	return nil
}

// appendPatches appends entries to containerdConfigPatches, creating it if needed.
func appendPatches(root *yaml.Node, patches []string) {
	if len(patches) == 0 {
		return
	}
	seq := mapGet(root, "containerdConfigPatches")
	if seq == nil {
		seq = &yaml.Node{Kind: yaml.SequenceNode}
		mapSet(root, "containerdConfigPatches", seq)
	}
	for _, p := range patches {
		seq.Content = append(seq.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Style: yaml.LiteralStyle,
			Value: p,
		})
	}
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mapSet appends key: val to mapping m. Callers only use it for absent keys.
func mapSet(m *yaml.Node, key string, val *yaml.Node) {
	m.Content = append(m.Content, str(key), val)
}

func str(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}
```

- [ ] **Step 8: Run all kindconfig tests**

Run: `go test ./internal/kindconfig/ -v`
Expected: PASS (new tests and the old `PatchForRegistries` tests).

- [ ] **Step 9: Commit**

```bash
git add internal/kindconfig/hosts.go internal/kindconfig/hosts_test.go internal/kindconfig/registries.go internal/kindconfig/registries_test.go
git commit -m "feat: add kind config patcher for containerd certs.d mounts"
```

---

### Task 2: config — `registries`, strict loading, validation, host files, auth patch

**Files:**
- Modify: `internal/config/config.go`, `internal/config/registry.go`, `go.mod`, `go.sum`
- Test: `internal/config/config_test.go`, `internal/config/registry_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `type Registry struct { Hosts string; Auth *RegistryAuth }` (yaml `hosts`, `auth`)
  - `Config.Registries map[string]*Registry` (yaml `registries`)
  - `func (c *Config) Validate() error`
  - `func (c *Config) HostsFiles() map[string]string`
  - `func (c *Config) AuthRegistries() []string` (sorted)
  - `func (c *Config) AuthPatch() string`

`Config.RegistryAuth` and `ContainerdPatch` stay for now so the old callers still compile; Task 3 removes them.

- [ ] **Step 1: Add the TOML dependency**

Run: `go get github.com/BurntSushi/toml@v1.6.0`
Expected: `go.mod` gains `github.com/BurntSushi/toml v1.6.0`.

- [ ] **Step 2: Write failing config tests**

Append to `internal/config/config_test.go`:

```go
func TestLoadRegistries(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte(`registries:
  quay.io:
    hosts: |
      server = "https://quay.io"
      [host."http://zot:5000/v2/quay.io"]
        capabilities = ["pull", "resolve"]
        override_path = true
  registry.corp.com:
    auth:
      username: svc
      password: secret
`), 0644)

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !strings.Contains(cfg.Registries["quay.io"].Hosts, `override_path = true`) {
		t.Errorf("unexpected quay.io hosts: %q", cfg.Registries["quay.io"].Hosts)
	}
	auth := cfg.Registries["registry.corp.com"].Auth
	if auth == nil || auth.Username != "svc" || auth.Password != "secret" {
		t.Errorf("unexpected auth: %+v", auth)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("imagez:\n  cilium: x\n"), 0644)

	if _, err := Load(configFile); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestLoadRejectsInvalidHostsTOML(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("registries:\n  quay.io:\n    hosts: \"server = \"\n"), 0644)

	_, err := Load(configFile)
	if err == nil || !strings.Contains(err.Error(), "quay.io") {
		t.Fatalf("expected TOML error naming quay.io, got %v", err)
	}
}

func TestLoadRejectsBadRegistryName(t *testing.T) {
	for _, name := range []string{`""`, `a/b`, `..`, `a..b`} {
		dir := t.TempDir()
		configFile := filepath.Join(dir, "config.yaml")
		os.WriteFile(configFile, []byte("registries:\n  "+name+":\n    hosts: \"x = 1\"\n"), 0644)

		if _, err := Load(configFile); err == nil {
			t.Errorf("expected error for registry name %s", name)
		}
	}
}

func TestMergeRegistriesPerField(t *testing.T) {
	base := &Config{Registries: map[string]*Registry{
		"quay.io":  {Hosts: "base-hosts"},
		"ghcr.io":  {Hosts: "ghcr-hosts"},
		"corp.com": {Auth: &RegistryAuth{Username: "a", Password: "a"}},
	}}
	override := &Config{Registries: map[string]*Registry{
		"quay.io":  {Auth: &RegistryAuth{Username: "u", Password: "p"}},
		"ghcr.io":  {Hosts: "override-hosts"},
		"corp.com": {Auth: &RegistryAuth{Username: "b", Password: "b"}},
	}}

	merged := Merge(base, override)

	if r := merged.Registries["quay.io"]; r.Hosts != "base-hosts" || r.Auth == nil || r.Auth.Username != "u" {
		t.Errorf("quay.io should keep base hosts and take override auth, got %+v", r)
	}
	if merged.Registries["ghcr.io"].Hosts != "override-hosts" {
		t.Errorf("ghcr.io hosts should be overridden, got %q", merged.Registries["ghcr.io"].Hosts)
	}
	if merged.Registries["corp.com"].Auth.Username != "b" {
		t.Errorf("corp.com auth should be overridden")
	}
}

func TestMergeDoesNotModifyInputs(t *testing.T) {
	base := &Config{Registries: map[string]*Registry{"quay.io": {Hosts: "base-hosts"}}}
	override := &Config{Registries: map[string]*Registry{"quay.io": {Hosts: "override-hosts"}}}

	Merge(base, override)

	if base.Registries["quay.io"].Hosts != "base-hosts" {
		t.Error("Merge must not modify base")
	}
}
```

Add `"strings"` to the imports of `config_test.go`.

- [ ] **Step 3: Write failing host-file/auth tests**

Append to `internal/config/registry_test.go`:

```go
func TestHostsFilesAutoTrust(t *testing.T) {
	cfg := &Config{Images: map[string]string{
		"a": "registry.airgap:5000/foo:v1",
		"b": "quay.io/cilium/cilium:v1.13.10",
	}}

	files := cfg.HostsFiles()

	if len(files) != 1 {
		t.Fatalf("expected one host file, got %v", files)
	}
	want := "server = \"http://registry.airgap:5000\"\n\n" +
		"[host.\"http://registry.airgap:5000\"]\n" +
		"  capabilities = [\"pull\", \"resolve\"]\n" +
		"  skip_verify = true\n"
	if files["registry.airgap:5000"] != want {
		t.Errorf("got:\n%s\nwant:\n%s", files["registry.airgap:5000"], want)
	}
}

func TestHostsFilesExplicitOverridesAutoTrust(t *testing.T) {
	cfg := &Config{
		Images: map[string]string{"a": "registry.airgap:5000/foo:v1"},
		Registries: map[string]*Registry{
			"registry.airgap:5000": {Hosts: "custom"},
			"docker.io":            {Hosts: "mirror"},
		},
	}

	files := cfg.HostsFiles()

	if files["registry.airgap:5000"] != "custom" || files["docker.io"] != "mirror" || len(files) != 2 {
		t.Errorf("unexpected files: %v", files)
	}
}

func TestHostsFilesAuthOnlyHasNoFile(t *testing.T) {
	cfg := &Config{Registries: map[string]*Registry{
		"registry.corp.com": {Auth: &RegistryAuth{Username: "u", Password: "p"}},
	}}

	if files := cfg.HostsFiles(); len(files) != 0 {
		t.Errorf("expected no host files, got %v", files)
	}
}

func TestAuthPatch(t *testing.T) {
	cfg := &Config{Registries: map[string]*Registry{
		"z.example.com": {Auth: &RegistryAuth{Username: "z", Password: `p@ss"w0rd\`}},
		"a.example.com": {Auth: &RegistryAuth{Username: "a", Password: "a"}},
		"docker.io":     {Hosts: "mirror"},
	}}

	if got := cfg.AuthRegistries(); len(got) != 2 || got[0] != "a.example.com" || got[1] != "z.example.com" {
		t.Errorf("AuthRegistries = %v", got)
	}

	patch := cfg.AuthPatch()
	if !strings.Contains(patch, `[plugins."io.containerd.grpc.v1.cri".registry.configs."z.example.com".auth]`) {
		t.Error("patch should contain z.example.com auth section")
	}
	if !strings.Contains(patch, `password = "p@ss\"w0rd\\"`) {
		t.Errorf("password should be escaped, got:\n%s", patch)
	}
	if strings.Index(patch, "a.example.com") > strings.Index(patch, "z.example.com") {
		t.Error("registries should be sorted")
	}
	if strings.Contains(patch, "docker.io") {
		t.Error("registries without auth must not appear")
	}
}

func TestAuthPatchEmpty(t *testing.T) {
	if patch := (&Config{}).AuthPatch(); patch != "" {
		t.Errorf("expected empty patch, got %q", patch)
	}
}
```

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/config/ -v`
Expected: FAIL to compile, `undefined: Registry`, `cfg.HostsFiles undefined`, etc.

- [ ] **Step 5: Implement config changes**

Replace `internal/config/config.go` with:

```go
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type RegistryAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Registry holds per-registry containerd settings.
type Registry struct {
	// Hosts is the raw content of certs.d/<registry>/hosts.toml.
	Hosts string        `yaml:"hosts,omitempty"`
	Auth  *RegistryAuth `yaml:"auth,omitempty"`
}

type Config struct {
	Images       map[string]string        `yaml:"images"`
	RegistryAuth map[string]*RegistryAuth `yaml:"registryAuth,omitempty"`
	Registries   map[string]*Registry     `yaml:"registries,omitempty"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks registry names and that hosts entries are valid TOML.
func (c *Config) Validate() error {
	for name, r := range c.Registries {
		if err := validateRegistryName(name); err != nil {
			return err
		}
		if r == nil || r.Hosts == "" {
			continue
		}
		var v map[string]any
		if err := toml.Unmarshal([]byte(r.Hosts), &v); err != nil {
			return fmt.Errorf("registries.%s.hosts is not valid TOML: %w", name, err)
		}
	}
	return nil
}

// validateRegistryName rejects names that are unsafe as a directory name.
func validateRegistryName(name string) error {
	if name == "" || name == "." || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid registry name %q: must be a host or host:port", name)
	}
	return nil
}

func Merge(base, override *Config) *Config {
	merged := &Config{
		Images:       make(map[string]string),
		RegistryAuth: make(map[string]*RegistryAuth),
		Registries:   make(map[string]*Registry),
	}
	for k, v := range base.Images {
		merged.Images[k] = v
	}
	for k, v := range override.Images {
		merged.Images[k] = v
	}
	for k, v := range base.RegistryAuth {
		merged.RegistryAuth[k] = v
	}
	for k, v := range override.RegistryAuth {
		merged.RegistryAuth[k] = v
	}
	for _, src := range []*Config{base, override} {
		for name, r := range src.Registries {
			if r == nil {
				continue
			}
			m := merged.Registries[name]
			if m == nil {
				m = &Registry{}
				merged.Registries[name] = m
			}
			if r.Hosts != "" {
				m.Hosts = r.Hosts
			}
			if r.Auth != nil {
				auth := *r.Auth
				m.Auth = &auth
			}
		}
	}
	return merged
}
```

Append to `internal/config/registry.go` (add `"sort"` to its imports):

```go
// HostsFiles returns the hosts.toml content to write per registry: an
// insecure (HTTP) config for each private registry found in images, replaced
// by the registry's explicit hosts entry when one is set.
func (c *Config) HostsFiles() map[string]string {
	files := make(map[string]string)
	for _, reg := range c.PrivateRegistries() {
		files[reg] = insecureHosts(reg)
	}
	for name, r := range c.Registries {
		if r != nil && r.Hosts != "" {
			files[name] = r.Hosts
		}
	}
	return files
}

func insecureHosts(reg string) string {
	url := "http://" + reg
	return fmt.Sprintf("server = %q\n\n[host.%q]\n  capabilities = [\"pull\", \"resolve\"]\n  skip_verify = true\n", url, url)
}

// AuthRegistries returns the sorted names of registries with credentials.
func (c *Config) AuthRegistries() []string {
	var names []string
	for name, r := range c.Registries {
		if r != nil && r.Auth != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// AuthPatch returns a containerd config patch with credentials for every
// registry that has auth, or "" when there are none.
func (c *Config) AuthPatch() string {
	var b strings.Builder
	for _, name := range c.AuthRegistries() {
		auth := c.Registries[name].Auth
		fmt.Fprintf(&b, "[plugins.\"io.containerd.grpc.v1.cri\".registry.configs.%q.auth]\n", name)
		fmt.Fprintf(&b, "  username = %q\n", auth.Username)
		fmt.Fprintf(&b, "  password = %q\n", auth.Password)
	}
	return b.String()
}
```

- [ ] **Step 6: Tidy and run tests**

Run: `go mod tidy && go test ./... `
Expected: PASS for all packages (old callers still compile against `RegistryAuth` / `ContainerdPatch`).

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/config/
git commit -m "feat: add registries config with hosts.toml and auth entries"
```

---

### Task 3: Wire into create/delete and remove the legacy registry path

**Files:**
- Create: `internal/cmd/registries.go`, `internal/cmd/registries_test.go`
- Modify: `internal/cmd/create.go:65-94`, `internal/cmd/delete.go:29-30`, `internal/config/config.go`, `internal/config/registry.go`, `internal/config/config_test.go`, `internal/config/registry_test.go`
- Delete: `internal/kindconfig/patch.go`, `internal/kindconfig/patch_test.go`

**Interfaces:**
- Consumes: `kindconfig.WriteHostsDir`, `kindconfig.Patch`, `kindconfig.Options` (Task 1); `Config.HostsFiles`, `Config.AuthPatch`, `Config.AuthRegistries`, `Config.PrivateRegistries` (Task 2).
- Produces:
  - `func profileStateDir(profile string) string` → `<dataDir>/state/<profile>`
  - `func prepareKindConfig(kindCfgPath string, cfg *config.Config, stateDir string) (string, func(), error)`

- [ ] **Step 1: Write failing tests**

`internal/cmd/registries_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -v`
Expected: FAIL, `undefined: prepareKindConfig`.

- [ ] **Step 3: Implement the helper**

`internal/cmd/registries.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/kindconfig"
)

// profileStateDir holds files a profile's clusters need at runtime, such as
// the registry host files mounted into Kind nodes.
func profileStateDir(profile string) string {
	return filepath.Join(dataDir(), "state", profile)
}

// prepareKindConfig writes the registry host files under stateDir and returns
// the kind config to use: the original path when no registry settings apply,
// otherwise a patched temp copy removed by the returned cleanup func.
func prepareKindConfig(kindCfgPath string, cfg *config.Config, stateDir string) (string, func(), error) {
	noop := func() {}
	certsDir := filepath.Join(stateDir, "certs.d")

	if regs := cfg.PrivateRegistries(); len(regs) > 0 {
		fmt.Printf("Trusting private registries: %s\n", strings.Join(regs, ", "))
	}

	files := cfg.HostsFiles()
	if len(files) == 0 {
		if err := os.RemoveAll(certsDir); err != nil {
			return "", noop, err
		}
		certsDir = ""
	} else {
		if err := kindconfig.WriteHostsDir(certsDir, files); err != nil {
			return "", noop, fmt.Errorf("writing registry host files: %w", err)
		}
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Printf("Configuring registry hosts: %s\n", strings.Join(names, ", "))
	}

	if regs := cfg.AuthRegistries(); len(regs) > 0 {
		fmt.Printf("Configuring registry auth: %s\n", strings.Join(regs, ", "))
	}

	path, cleanup, err := kindconfig.Patch(kindCfgPath, kindconfig.Options{
		CertsDir:  certsDir,
		AuthPatch: cfg.AuthPatch(),
	})
	if err != nil {
		return "", noop, fmt.Errorf("patching kind config for registries: %w", err)
	}
	return path, cleanup, nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/cmd/ -v`
Expected: PASS.

- [ ] **Step 5: Wire into `runProfileFunc`**

In `internal/cmd/create.go`, replace the block from `home, _ := os.UserHomeDir()` through the closing `}` of `if cfg != nil { ... }` (currently lines 70-94) with:

```go
	kindCfgPath := filepath.Join(p.Dir, "kind-config.yaml")
	if cfg != nil {
		patched, cleanup, err := prepareKindConfig(kindCfgPath, cfg, profileStateDir(p.Name))
		if err != nil {
			return err
		}
		defer cleanup()
		kindCfgPath = patched
	}
```

and in the `env := append(...)` call that follows, replace every use of the removed local `dataDir` variable with the `dataDir()` function:

```go
	env := append(os.Environ(),
		"KINDTKS_DATA_DIR="+dataDir(),
		"KINDTKS_CHARTS_DIR="+filepath.Join(dataDir(), "charts"),
		"KINDTKS_PROFILE_NAME="+p.Name,
		"KINDTKS_PROFILE_DIR="+p.Dir,
		"KINDTKS_KIND_CONFIG="+kindCfgPath,
	)
```

Remove the now-unused `kindconfig` import from `create.go`.

- [ ] **Step 6: Remove state on delete**

In `internal/cmd/delete.go`, replace `return runProfileFunc(p, "delete", nil)` with:

```go
		if err := runProfileFunc(p, "delete", nil); err != nil {
			return err
		}
		return os.RemoveAll(profileStateDir(name))
```

Add `"os"` to its imports.

- [ ] **Step 7: Remove the legacy registry code**

- Delete `internal/kindconfig/patch.go` and `internal/kindconfig/patch_test.go`.
- In `internal/config/registry.go`, delete `ContainerdPatch`.
- In `internal/config/registry_test.go`, delete `TestContainerdPatch`, `TestContainerdPatchEmpty`, `TestContainerdPatchAuth`, `TestContainerdPatchInsecureAndAuth`, `TestContainerdPatchNilAuthEntry`, `TestContainerdPatchEmptyCredentials`, `TestContainerdPatchMultipleAuth`, `TestContainerdPatchSpecialCharsInPassword`.
- In `internal/config/config.go`, remove the `RegistryAuth` field from `Config`, and in `Merge` remove `RegistryAuth: make(...)` and the two `RegistryAuth` loops.
- In `internal/config/config_test.go`, delete `TestLoadRegistryAuth`, `TestMergeRegistryAuth` and any other test referencing `cfg.RegistryAuth` / `merged.RegistryAuth` (check with `grep -n RegistryAuth internal/config/config_test.go`; only uses of the `RegistryAuth` *type* inside `Registry.Auth` may remain).

- [ ] **Step 8: Add a regression test for the removed key**

Append to `internal/config/config_test.go`:

```go
func TestLoadRejectsRegistryAuth(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("registryAuth:\n  r.io:\n    username: u\n    password: p\n"), 0644)

	_, err := Load(configFile)
	if err == nil || !strings.Contains(err.Error(), "registryAuth") {
		t.Fatalf("expected error mentioning registryAuth, got %v", err)
	}
}
```

- [ ] **Step 9: Run everything**

Run: `go vet ./... && go test ./... && bats test/unit/`
Expected: all PASS; `grep -rn 'mirrors\|ContainerdPatch\|PatchForRegistries' internal/` prints nothing.

- [ ] **Step 10: Commit**

```bash
git add -A internal/
git commit -m "feat: configure registries via containerd certs.d host files

Replaces registryAuth and the legacy mirrors/tls patch, which containerd
rejects once config_path is set."
```

---

### Task 4: Docs and air-gap test config

**Files:**
- Modify: `README.md:52-96`, `profiles/gen1/README.md:182-200`, `test/e2e/airgap-gen1.bats:144-147`

- [ ] **Step 1: Update the air-gap e2e config**

In `test/e2e/airgap-gen1.bats`, replace:

```yaml
registryAuth:
    registry.airgap:5000:
        username: airgap
        password: airgap
```

with:

```yaml
registries:
    registry.airgap:5000:
        auth:
            username: airgap
            password: airgap
```

- [ ] **Step 2: Update `README.md`**

Keep the "Custom Image Registry" intro and air-gap example. Replace the paragraph starting "Private registries are automatically trusted…" and the whole "### Registry Authentication" section (up to, not including, the "Note: `kind` pulls the node image…" paragraph) with:

````markdown
Private registries are automatically trusted as insecure (HTTP): kindtks writes a containerd `hosts.toml` for each one and mounts it into every Kind node. The following well-known registries are excluded from this auto-trust: docker.io, quay.io, ghcr.io, gcr.io, registry.k8s.io, k8s.gcr.io, mcr.microsoft.com, public.ecr.aws. Everything else is treated as private.

### Per-registry settings

The `registries` section configures containerd per registry:

- `hosts`: raw content of containerd's [`hosts.toml`](https://github.com/containerd/containerd/blob/main/docs/hosts.md) for that registry. Replaces the auto-trust config for a private registry.
- `auth`: username and password, for registries that need authentication (HTTP or HTTPS).

Authentication:

```yaml
images:
    cilium: registry.corp.com/cilium/cilium:v1.13.10
registries:
    registry.corp.com:
        auth:
            username: svc-account
            password: secret-token
```

Pull-through mirror: images keep their normal names and containerd pulls them through the mirror, falling back to the upstream registry if the mirror is down. An override config only needs the `registries` section:

```yaml
registries:
    quay.io:
        hosts: |
            server = "https://quay.io"
            [host."http://mirror:5000/v2/quay.io"]
              capabilities = ["pull", "resolve"]
              override_path = true
```

Host files are written to `~/.local/share/kindtks/state/<profile>/certs.d/` and removed by `kindtks delete`. The mirror must be reachable from the Kind nodes (e.g. a container on the `kind` Docker network).
````

- [ ] **Step 3: Update `profiles/gen1/README.md`**

Replace the "Private registries (anything not docker.io…)" paragraph and the "### Registry Authentication" section's YAML/intro (keep its trailing `kind-node` pre-pull note) with:

````markdown
Private registries (anything not docker.io, quay.io, ghcr.io, etc.) are automatically trusted as insecure (HTTP) in the Kind nodes' containerd config.

### Registry Authentication and Mirrors

Per-registry containerd settings go under `registries` (see the top-level README for details):

```yaml
images:
    kind-node: registry.internal:5000/kindest/node:v1.24.17
    cilium: registry.internal:5000/quay.io/cilium/cilium:v1.13.10
    # ... other images ...
registries:
    registry.internal:5000:
        auth:
            username: myuser
            password: mypass
```

Credentials are injected into the Kind nodes' containerd config. A `hosts` entry (raw containerd `hosts.toml`) points a registry at a mirror or pull-through cache.
````

Then ensure the next sentence still reads correctly (it starts "The `kind-node` image is pulled by Docker on the host…").

- [ ] **Step 4: Verify no stale references**

Run: `grep -rn registryAuth README.md profiles test --include=*.md --include=*.bats --include=*.yaml`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add README.md profiles/gen1/README.md test/e2e/airgap-gen1.bats
git commit -m "docs: document registries config for auth and mirrors"
```

---

### Task 5: End-to-end tests

**Files:**
- Create: `test/e2e/registry-hosts.bats`

**Interfaces:**
- Consumes: the `kindtks` binary built from the repo; `kindtks create <profile> --config`, `kindtks delete <profile>`.

- [ ] **Step 1: Write the e2e test**

`test/e2e/registry-hosts.bats`:

```bash
#!/usr/bin/env bats
# Verifies registries.<reg>.hosts end to end: host files are mounted on every
# node, containerd accepts config_path together with an auth patch, and pulls
# go through the configured mirror. Self-contained: uses its own two-node
# test profile and a registry:2 pull-through cache on the kind network.

load '../helpers/test_helper'

CLUSTER_NAME="kindtks-rh-e2e"
MIRROR_NAME="kindtks-rh-e2e-mirror"
PROFILE="rh-e2e"

setup_file() {
  export E2E_HOME="${BATS_FILE_TMPDIR}/home"
  local profile_dir="${E2E_HOME}/.local/share/kindtks/profiles/${PROFILE}"
  mkdir -p "${E2E_HOME}/.local/bin" "$profile_dir"

  go build -o "${E2E_HOME}/.local/bin/kindtks" "${BATS_TEST_DIRNAME}/../../"

  cat > "${profile_dir}/install.sh" <<EOF
REQUIRES="kind"

create() {
  kind create cluster --name ${CLUSTER_NAME} --image kindest/node:v1.24.17 --config "\$KINDTKS_KIND_CONFIG"
}

delete() {
  kind delete cluster --name ${CLUSTER_NAME}
}
EOF

  cat > "${profile_dir}/kind-config.yaml" <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
EOF

  : > "${profile_dir}/config.yaml"

  cat > "${BATS_FILE_TMPDIR}/config.yaml" <<EOF
images:
  probe: registry.e2e.invalid:5000/probe:v1
registries:
  docker.io:
    hosts: |
      server = "https://registry-1.docker.io"
      [host."http://${MIRROR_NAME}:5000"]
        capabilities = ["pull", "resolve"]
  auth.e2e.invalid:
    auth:
      username: u
      password: p
EOF

  HOME="$E2E_HOME" "${E2E_HOME}/.local/bin/kindtks" create "$PROFILE" \
    --config "${BATS_FILE_TMPDIR}/config.yaml"

  # Started after the cluster so the kind network exists.
  docker rm -f "$MIRROR_NAME" >/dev/null 2>&1 || true
  docker run -d --name "$MIRROR_NAME" --network kind \
    -e REGISTRY_PROXY_REMOTEURL=https://registry-1.docker.io registry:2
}

teardown_file() {
  kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
  docker rm -f "$MIRROR_NAME" >/dev/null 2>&1 || true
}

nodes() {
  kind get nodes --name "$CLUSTER_NAME"
}

kindtks() {
  HOME="$E2E_HOME" "${E2E_HOME}/.local/bin/kindtks" "$@"
}

@test "every node has the generated host files" {
  for node in $(nodes); do
    run docker exec "$node" cat /etc/containerd/certs.d/docker.io/hosts.toml
    assert_success
    assert_output --partial "http://${MIRROR_NAME}:5000"

    run docker exec "$node" cat /etc/containerd/certs.d/registry.e2e.invalid:5000/hosts.toml
    assert_success
    assert_output --partial "skip_verify = true"
  done
}

@test "auth-only registry gets no host file" {
  run docker exec "${CLUSTER_NAME}-control-plane" test -e /etc/containerd/certs.d/auth.e2e.invalid
  assert_failure
}

@test "containerd config has config_path and the auth patch on every node" {
  for node in $(nodes); do
    run docker exec "$node" cat /etc/containerd/config.toml
    assert_success
    assert_output --partial 'config_path = "/etc/containerd/certs.d"'
    assert_output --partial 'auth.e2e.invalid'
  done
}

@test "pulls from every node go through the mirror" {
  for node in $(nodes); do
    run docker exec "$node" crictl pull docker.io/library/busybox:1.36
    assert_success
  done
  run docker logs "$MIRROR_NAME"
  assert_output --partial "library/busybox"
}

@test "kindtks delete removes generated state" {
  run kindtks delete "$PROFILE"
  assert_success
  [ ! -e "${E2E_HOME}/.local/share/kindtks/state/${PROFILE}" ]
}
```

- [ ] **Step 2: Run the new e2e test**

Run: `bats test/e2e/registry-hosts.bats`
Expected: 5/5 PASS. If `config_path` is rendered differently by kind's TOML merge, adjust only that assertion after inspecting `docker exec kindtks-rh-e2e-control-plane cat /etc/containerd/config.toml`; do not weaken the pull-through assertion.

- [ ] **Step 3: Run the gen1 e2e test (no registries → unchanged kind config)**

Run: `bats test/e2e/gen1.bats`
Expected: all PASS.

- [ ] **Step 4: Run the air-gap e2e test (auto-trust + auth via hosts.toml)**

This runs inside the air-gap VM over SSH (see `test/e2e/airgap-gen1.bats`); the VM must be running.

Run: `bats test/e2e/airgap-gen1.bats`
Expected: all PASS. This is the only test covering an HTTP registry with credentials under `config_path`; a failure here means `configs.auth` is not applied for that host and must be investigated, not skipped.

- [ ] **Step 5: Commit**

```bash
git add test/e2e/registry-hosts.bats
git commit -m "test: add e2e test for registry hosts.toml mirrors"
```
