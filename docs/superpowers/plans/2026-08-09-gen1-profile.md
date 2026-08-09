# gen1 Profile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor profile system to directory-based layout, add config/charts support for air-gapped environments, and implement the gen1 profile (k8s 1.24, Cilium 1.12, Istio 1.16, Vault, Vault Secrets Operator).

**Architecture:** Profiles become directories (`profiles/<name>/install.sh`). Helm charts are bundled as `.tgz` files in the Docker image and copied to host during install. A config file (`~/.config/kindtks/config.yaml`) provides a registry prefix for air-gapped environments. Profile scripts receive env vars for charts dir, registry, and profile dir.

**Tech Stack:** Go 1.24, cobra, Docker, Helm, Kind

**Prerequisites:** Go via `eval "$(~/.local/bin/mise activate bash)" && <go command>`.

---

## File Structure (changes from current)

```
kindtks/
├── internal/
│   ├── cmd/
│   │   ├── create.go              # MODIFY: pass new env vars to profile scripts
│   │   ├── install.go             # MODIFY: copy charts dir, handle profile subdirs
│   │   └── list.go                # MODIFY: (minor) adjust for dir-based profiles
│   ├── config/
│   │   ├── config.go              # NEW: Load config from ~/.config/kindtks/config.yaml
│   │   └── config_test.go         # NEW: tests
│   └── profile/
│       ├── profile.go             # MODIFY: look for <name>/install.sh instead of <name>.sh
│       └── profile_test.go        # MODIFY: update tests for new layout
├── profiles/
│   ├── example/
│   │   └── install.sh             # MOVE from profiles/example.sh
│   └── gen1/
│       ├── install.sh             # NEW: gen1 profile script
│       └── kind-config.yaml       # NEW: Kind cluster config for gen1
├── charts/                        # NEW: Helm chart .tgz files (downloaded at Docker build time)
│   └── .gitkeep
├── Dockerfile                     # MODIFY: add helm stage to pull charts
└── scripts/
    └── pull-charts.sh             # NEW: script to download all chart .tgz files
```

---

### Task 1: Refactor profile loading to directory-based layout

**Files:**
- Modify: `internal/profile/profile.go`
- Modify: `internal/profile/profile_test.go`
- Move: `profiles/example.sh` → `profiles/example/install.sh`

- [ ] **Step 1: Update tests for directory-based profiles**

Replace `internal/profile/profile_test.go`:
```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)" && go test ./internal/profile/ -v
```
Expected: failures due to `Profile.Dir` not existing and path mismatches.

- [ ] **Step 3: Update profile.go for directory-based layout**

Replace `internal/profile/profile.go`:
```go
package profile

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Profile struct {
	Name     string
	Dir      string
	Path     string
	Requires []string
}

func Load(profileDir string, name string) (*Profile, error) {
	dir := filepath.Join(profileDir, name)
	path := filepath.Join(dir, "install.sh")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("profile %q not found (expected %s): %w", name, path, err)
	}

	requires, err := parseRequires(path)
	if err != nil {
		return nil, fmt.Errorf("parsing profile %q: %w", name, err)
	}

	return &Profile{
		Name:     name,
		Dir:      dir,
		Path:     path,
		Requires: requires,
	}, nil
}

func ListAll(profileDir string) ([]string, error) {
	entries, err := os.ReadDir(profileDir)
	if err != nil {
		return nil, fmt.Errorf("reading profile directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		installSh := filepath.Join(profileDir, e.Name(), "install.sh")
		if _, err := os.Stat(installSh); err == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func parseRequires(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "REQUIRES=") {
			value := strings.TrimPrefix(line, "REQUIRES=")
			value = strings.Trim(value, `"'`)
			if value == "" {
				return nil, nil
			}
			return strings.Fields(value), nil
		}
	}
	return nil, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)" && go test ./internal/profile/ -v
```
Expected: all 4 tests pass.

- [ ] **Step 5: Move example profile to directory layout**

```bash
mkdir -p profiles/example
mv profiles/example.sh profiles/example/install.sh
```

- [ ] **Step 6: Verify build and list still work**

```bash
eval "$(~/.local/bin/mise activate bash)" && go build -o kindtks .
```

- [ ] **Step 7: Commit**

```bash
git add internal/profile/ profiles/
git commit -m "refactor: change profiles from flat files to directory layout"
```

---

### Task 2: Add config file support

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExisting(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte("registry: corp-registry.internal/\n"), 0644)

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Registry != "corp-registry.internal/" {
		t.Errorf("expected 'corp-registry.internal/', got %q", cfg.Registry)
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("Load should not error for missing file: %v", err)
	}
	if cfg.Registry != "" {
		t.Errorf("expected empty registry, got %q", cfg.Registry)
	}
}

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	os.WriteFile(configFile, []byte(""), 0644)

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Registry != "" {
		t.Errorf("expected empty registry, got %q", cfg.Registry)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
eval "$(~/.local/bin/mise activate bash)" && go test ./internal/config/ -v
```

- [ ] **Step 3: Implement config package**

Create `internal/config/config.go`:
```go
package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Registry string `yaml:"registry"`
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

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.config/kindtks/config.yaml"
}
```

Then add the yaml dependency:
```bash
eval "$(~/.local/bin/mise activate bash)" && go get gopkg.in/yaml.v3
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
eval "$(~/.local/bin/mise activate bash)" && go test ./internal/config/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: add config file support for registry prefix"
```

---

### Task 3: Update create/delete commands with new env vars

**Files:**
- Modify: `internal/cmd/create.go`

- [ ] **Step 1: Update runProfileFunc to pass new env vars**

Replace `runProfileFunc` and `dataDir` in `internal/cmd/create.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/prereq"
	"github.com/rophy/kindtks/internal/profile"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <profile>",
	Short: "Create cluster(s) from a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		dir := profileDir()

		p, err := profile.Load(dir, name)
		if err != nil {
			return err
		}

		if missing := prereq.Check(p.Requires); len(missing) > 0 {
			return fmt.Errorf("missing required tools: %s", strings.Join(missing, ", "))
		}

		fmt.Printf("Creating cluster(s) from profile %q...\n", name)
		return runProfileFunc(p, "create")
	},
}

func runProfileFunc(p *profile.Profile, funcName string) error {
	script := fmt.Sprintf("source %q && %s", p.Path, funcName)
	c := exec.Command("bash", "-e", "-c", script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	cfg, _ := config.Load(config.DefaultPath())
	registry := ""
	if cfg != nil {
		registry = cfg.Registry
	}

	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".local", "share", "kindtks")

	c.Env = append(os.Environ(),
		"KINDTKS_DATA_DIR="+dataDir,
		"KINDTKS_CHARTS_DIR="+filepath.Join(dataDir, "charts"),
		"KINDTKS_PROFILE_NAME="+p.Name,
		"KINDTKS_PROFILE_DIR="+p.Dir,
		"KINDTKS_REGISTRY="+registry,
	)
	return c.Run()
}

func init() {
	rootCmd.AddCommand(createCmd)
}
```

Note: remove the old `dataDir()` function since it's inlined now.

- [ ] **Step 2: Verify build**

```bash
eval "$(~/.local/bin/mise activate bash)" && go build -o kindtks .
```

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/create.go
git commit -m "feat: pass charts dir, profile dir, and registry to profile scripts"
```

---

### Task 4: Update install command for directory-based profiles and charts

**Files:**
- Modify: `internal/cmd/install.go`

- [ ] **Step 1: Update install.go to handle subdirectories and charts**

Replace `internal/cmd/install.go`:
```go
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	containerProfileDir = "/profiles"
	containerChartsDir  = "/charts"
	hostBinDir          = "/.local/bin"
	hostDataDir         = "/.local/share/kindtks"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install kindtks binary and profiles to the host (run inside Docker)",
	RunE: func(cmd *cobra.Command, args []string) error {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding own executable: %w", err)
		}

		destBin := filepath.Join(hostBinDir, "kindtks")
		fmt.Printf("Copying kindtks to %s...\n", destBin)
		if err := copyFile(self, destBin, 0755); err != nil {
			return fmt.Errorf("copying binary: %w", err)
		}

		profileDst := filepath.Join(hostDataDir, "profiles")
		fmt.Println("Copying profiles...")
		if err := copyDirRecursive(containerProfileDir, profileDst); err != nil {
			return fmt.Errorf("copying profiles: %w", err)
		}

		chartsDst := filepath.Join(hostDataDir, "charts")
		if _, err := os.Stat(containerChartsDir); err == nil {
			fmt.Println("Copying charts...")
			if err := copyDirRecursive(containerChartsDir, chartsDst); err != nil {
				return fmt.Errorf("copying charts: %w", err)
			}
		}

		fmt.Println()
		fmt.Println("Installation complete!")
		fmt.Println("Run 'kindtks list' to see available profiles.")
		return nil
	},
}

func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		fmt.Printf("  %s\n", rel)
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func init() {
	rootCmd.AddCommand(installCmd)
}
```

- [ ] **Step 2: Verify build**

```bash
eval "$(~/.local/bin/mise activate bash)" && go build -o kindtks .
```

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/install.go
git commit -m "feat: install command copies profile dirs and charts recursively"
```

---

### Task 5: Chart pull script and Dockerfile update

**Files:**
- Create: `scripts/pull-charts.sh`
- Create: `charts/.gitkeep`
- Modify: `Dockerfile`

- [ ] **Step 1: Create chart pull script**

Create `scripts/pull-charts.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail

CHARTS_DIR="${1:-charts}"
mkdir -p "$CHARTS_DIR"

helm repo add cilium https://helm.cilium.io/ --force-update
helm repo add istio https://istio-release.storage.googleapis.com/charts --force-update
helm repo add hashicorp https://helm.releases.hashicorp.com --force-update
helm repo update

helm pull cilium/cilium --version 1.12.19 --destination "$CHARTS_DIR"
helm pull istio/base --version 1.16.7 --destination "$CHARTS_DIR"
helm pull istio/istiod --version 1.16.7 --destination "$CHARTS_DIR"
helm pull istio/gateway --version 1.16.7 --destination "$CHARTS_DIR"
helm pull hashicorp/vault --version 0.34.0 --destination "$CHARTS_DIR"
helm pull hashicorp/vault-secrets-operator --version 1.5.0 --destination "$CHARTS_DIR"

echo "Charts downloaded to $CHARTS_DIR:"
ls -la "$CHARTS_DIR"/*.tgz
```

- [ ] **Step 2: Create charts/.gitkeep**

```bash
mkdir -p charts
touch charts/.gitkeep
echo '*.tgz' > charts/.gitignore
```

- [ ] **Step 3: Update Dockerfile**

Replace `Dockerfile`:
```dockerfile
FROM golang:1.24 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /kindtks .

FROM alpine:3.20 AS charts
RUN apk add --no-cache helm
COPY scripts/pull-charts.sh /pull-charts.sh
RUN /pull-charts.sh /charts

FROM alpine:3.20

COPY --from=builder /kindtks /usr/local/bin/kindtks
COPY profiles/ /profiles/
COPY --from=charts /charts/ /charts/

ENTRYPOINT ["kindtks"]
```

- [ ] **Step 4: Build the image**

```bash
cd /home/rophy/projects/kindtks
docker build -t kindtks .
```

- [ ] **Step 5: Verify charts are in the image**

```bash
docker run --rm kindtks ls /charts/
```

- [ ] **Step 6: Commit**

```bash
git add scripts/pull-charts.sh charts/.gitkeep charts/.gitignore Dockerfile
git commit -m "build: bundle Helm charts in Docker image"
```

---

### Task 6: Write gen1 profile

**Files:**
- Create: `profiles/gen1/install.sh`
- Create: `profiles/gen1/kind-config.yaml`

- [ ] **Step 1: Create Kind config for gen1**

Create `profiles/gen1/kind-config.yaml`:
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
  podSubnet: "10.244.0.0/16"
  serviceSubnet: "10.96.0.0/12"
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30080
        hostPort: 80
        protocol: TCP
      - containerPort: 30443
        hostPort: 443
        protocol: TCP
```

- [ ] **Step 2: Create gen1 install.sh**

Create `profiles/gen1/install.sh`:
```bash
REQUIRES="kind helm kubectl"

CLUSTER_NAME="gen1"
KIND_IMAGE="kindest/node:v1.24.15"

create() {
  echo "==> Creating Kind cluster '$CLUSTER_NAME' (k8s 1.24)..."
  kind create cluster \
    --name "$CLUSTER_NAME" \
    --image "$KIND_IMAGE" \
    --config "$KINDTKS_PROFILE_DIR/kind-config.yaml"

  echo "==> Waiting for cluster to be ready..."
  kubectl wait --for=condition=Ready nodes --all --timeout=120s

  install_cilium
  install_istio
  install_vault
  install_vault_secrets_operator

  echo ""
  echo "==> gen1 cluster is ready!"
  echo "    kubectl config use-context kind-$CLUSTER_NAME"
}

delete() {
  echo "==> Deleting Kind cluster '$CLUSTER_NAME'..."
  kind delete cluster --name "$CLUSTER_NAME"
  echo "==> Cluster '$CLUSTER_NAME' deleted."
}

install_cilium() {
  echo "==> Installing Cilium 1.12..."
  local values=""
  values="$values --set ipam.mode=kubernetes"
  values="$values --set image.pullPolicy=IfNotPresent"
  if [ -n "${KINDTKS_REGISTRY:-}" ]; then
    values="$values --set image.repository=${KINDTKS_REGISTRY}cilium/cilium"
    values="$values --set image.useDigest=false"
    values="$values --set operator.image.repository=${KINDTKS_REGISTRY}cilium/operator"
    values="$values --set operator.image.useDigest=false"
    values="$values --set preflight.image.repository=${KINDTKS_REGISTRY}cilium/cilium"
    values="$values --set preflight.image.useDigest=false"
  fi

  helm install cilium "$KINDTKS_CHARTS_DIR/cilium-1.12.19.tgz" \
    --namespace kube-system \
    $values

  echo "==> Waiting for Cilium to be ready..."
  kubectl -n kube-system rollout status deployment/cilium-operator --timeout=120s
  kubectl -n kube-system rollout status daemonset/cilium --timeout=120s
}

install_istio() {
  echo "==> Installing Istio 1.16..."
  local hub_values=""
  if [ -n "${KINDTKS_REGISTRY:-}" ]; then
    hub_values="--set global.hub=${KINDTKS_REGISTRY}istio --set global.tag=1.16.7"
  fi

  helm install istio-base "$KINDTKS_CHARTS_DIR/base-1.16.7.tgz" \
    --namespace istio-system --create-namespace \
    $hub_values

  helm install istiod "$KINDTKS_CHARTS_DIR/istiod-1.16.7.tgz" \
    --namespace istio-system \
    --wait --timeout 120s \
    $hub_values

  echo "==> Installing Istio Ingress Gateway..."
  helm install istio-ingress "$KINDTKS_CHARTS_DIR/gateway-1.16.7.tgz" \
    --namespace istio-ingress --create-namespace \
    --wait --timeout 120s \
    $hub_values
}

install_vault() {
  echo "==> Installing Vault (dev mode)..."
  local values=""
  values="$values --set server.dev.enabled=true"
  values="$values --set injector.enabled=false"
  if [ -n "${KINDTKS_REGISTRY:-}" ]; then
    values="$values --set server.image.repository=${KINDTKS_REGISTRY}hashicorp/vault"
  fi

  helm install vault "$KINDTKS_CHARTS_DIR/vault-0.34.0.tgz" \
    --namespace vault --create-namespace \
    --wait --timeout 120s \
    $values
}

install_vault_secrets_operator() {
  echo "==> Installing Vault Secrets Operator..."
  local values=""
  if [ -n "${KINDTKS_REGISTRY:-}" ]; then
    values="$values --set controller.manager.image.repository=${KINDTKS_REGISTRY}hashicorp/vault-secrets-operator"
  fi

  helm install vault-secrets-operator "$KINDTKS_CHARTS_DIR/vault-secrets-operator-1.5.0.tgz" \
    --namespace vault-secrets-operator --create-namespace \
    --wait --timeout 120s \
    $values
}
```

- [ ] **Step 3: Verify the profile is discovered**

```bash
eval "$(~/.local/bin/mise activate bash)" && go build -o kindtks .
mkdir -p ~/.local/share/kindtks/profiles
cp -r profiles/gen1 ~/.local/share/kindtks/profiles/
./kindtks list
```
Expected: shows `gen1` with requires: kind, helm, kubectl.

- [ ] **Step 4: Commit**

```bash
git add profiles/gen1/
git commit -m "feat: add gen1 profile (k8s 1.24, cilium, istio, vault)"
```

---

### Task 7: Rebuild Docker image and end-to-end test

**Files:**
- No new files.

- [ ] **Step 1: Rebuild Docker image**

```bash
cd /home/rophy/projects/kindtks
docker build -t kindtks .
```

- [ ] **Step 2: Test install from Docker**

```bash
mkdir -p /tmp/kindtks-e2e/.local/bin /tmp/kindtks-e2e/.local/share/kindtks
docker run --rm --user "$(id -u):$(id -g)" \
  -v /tmp/kindtks-e2e/.local/bin:/.local/bin \
  -v /tmp/kindtks-e2e/.local/share/kindtks:/.local/share/kindtks \
  kindtks install
```

- [ ] **Step 3: Verify profiles and charts installed**

```bash
HOME=/tmp/kindtks-e2e /tmp/kindtks-e2e/.local/bin/kindtks list
ls /tmp/kindtks-e2e/.local/share/kindtks/charts/*.tgz
```
Expected: shows gen1 and example profiles; charts directory has 6 `.tgz` files.

- [ ] **Step 4: Test gen1 create (if Kind is available)**

```bash
HOME=/tmp/kindtks-e2e /tmp/kindtks-e2e/.local/bin/kindtks create gen1
```
Expected: creates cluster, installs all components.

- [ ] **Step 5: Test gen1 delete**

```bash
HOME=/tmp/kindtks-e2e /tmp/kindtks-e2e/.local/bin/kindtks delete gen1
```

- [ ] **Step 6: Clean up**

```bash
rm -rf /tmp/kindtks-e2e
```

- [ ] **Step 7: Commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix: adjustments from gen1 end-to-end testing"
```
