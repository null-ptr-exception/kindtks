# Run kindtks from its Docker image — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** kindtks runs only via `docker run` of its image; no host install, no host tools besides Docker.

**Architecture:** The image carries kindtks, profiles, charts, and pinned kind/kubectl/helm/docker-cli/bash/envsubst. kindtks reads its profiles dir from `KINDTKS_PROFILES_DIR` (set in the image), prints the README (with the image's own tag) when run without arguments, and checks the `docker run` mounts before `create`/`delete`. The `install` command and `~/.local/share/kindtks` go away.

**Tech Stack:** Go 1.24 (cobra), Dockerfile (multi-stage, alpine), bats.

**Spec:** `docs/superpowers/specs/2026-10-07-docker-run-design.md`

## Global Constraints

- Commit messages: `<type>: <description>`, types feat/fix/refactor/chore/docs/build/test; no mention of Claude, no Co-Authored-By, no "Generated with" footer. Commits are GPG-signed automatically; never disable signing.
- Never `git push`.
- Public content (code, docs, tests) uses only `example.com` / `example.net` style names; never private or internal domains or hostnames.
- Image reference constant: `ghcr.io/null-ptr-exception/kindtks:latest`; the image sets `KINDTKS_IMAGE=ghcr.io/null-ptr-exception/kindtks:<VERSION>`.
- Pins (exact, from the spec): `golang:1.24.13@sha256:d2d2bc1c84f7e60d7d2438a3836ae7d0c847f4888464e7ec9ba3a1339a1ee804`; `alpine:3.20.10@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc`; apk `bash=5.2.26-r0`, `gettext=0.22.5-r0`; docker CLI 29.2.1 `995b1d0b51e96d551a3b49c552c0170bc6ce9f8b9e0866b8c15bbc67d1cf93a3`; kind v0.33.0 `aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d`; kubectl v1.33.1 `5de4e9f2266738fd112b721265a0c1cd7f4e5208b670f811861f699474a100a3`; helm v3.21.0 `0093eb572e3d2380f094df162ddb525e219249de88957afe24cfbb19632acd36`.
- Run long commands (docker build, bats e2e) in the foreground with a generous timeout; do not leave background jobs or monitors.
- Do not touch kind clusters other than the ones a test creates (`gen1`, `pacds` and others may be running).

---

### Task 1: Profiles dir from env, README on no args, remove `install`

**Files:**
- Modify: `internal/cmd/list.go`, `internal/cmd/create.go`, `internal/cmd/delete.go`, `internal/cmd/config.go`, `internal/cmd/help.go`, `internal/cmd/root.go`, `internal/cmd/run.go`, `internal/cmd/registries.go` (comment only)
- Delete: `internal/cmd/install.go`
- Create: `internal/cmd/dirs_test.go`

**Interfaces:**
- Produces: `const imageRef = "ghcr.io/null-ptr-exception/kindtks:latest"`; `func profileDir() (string, error)`; `func readme() string`. Task 2 uses `imageRef` and `os.Getenv("KINDTKS_IMAGE")`.

- [ ] **Step 1: Write the failing tests** — `internal/cmd/dirs_test.go`:

```go
package cmd

import (
	"strings"
	"testing"
)

func TestProfileDir_FromEnv(t *testing.T) {
	t.Setenv("KINDTKS_PROFILES_DIR", "/profiles")
	dir, err := profileDir()
	if err != nil || dir != "/profiles" {
		t.Fatalf("got %q, %v", dir, err)
	}
}

func TestProfileDir_Unset(t *testing.T) {
	t.Setenv("KINDTKS_PROFILES_DIR", "")
	_, err := profileDir()
	if err == nil || !strings.Contains(err.Error(), "docker run --rm "+imageRef) {
		t.Fatalf("want hint to run the image, got %v", err)
	}
}

func TestReadme_SubstitutesImage(t *testing.T) {
	old := projectReadme
	t.Cleanup(func() { projectReadme = old })
	projectReadme = "run: docker run --rm " + imageRef + " list\n"

	t.Setenv("KINDTKS_IMAGE", "")
	if got := readme(); got != projectReadme {
		t.Errorf("outside image: got %q", got)
	}
	t.Setenv("KINDTKS_IMAGE", "ghcr.io/null-ptr-exception/kindtks:0.6.0")
	if got := readme(); got != "run: docker run --rm ghcr.io/null-ptr-exception/kindtks:0.6.0 list\n" {
		t.Errorf("inside image: got %q", got)
	}
}

func TestNoInstallCommand(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "install" {
			t.Fatal("install command still registered")
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run 'TestProfileDir|TestReadme|TestNoInstall' -v`
Expected: compile failure (`profileDir` returns one value, `readme`/`imageRef` undefined).

- [ ] **Step 3: Implement**

In `internal/cmd/list.go`, replace `dataDir()` and `profileDir()` with:

```go
// imageRef is the published image; the README's run command uses it and
// readme() swaps in the running image's own reference.
const imageRef = "ghcr.io/null-ptr-exception/kindtks:latest"

// profileDir is the profiles directory, baked into the image and exported
// as KINDTKS_PROFILES_DIR there.
func profileDir() (string, error) {
	dir := os.Getenv("KINDTKS_PROFILES_DIR")
	if dir == "" {
		return "", fmt.Errorf("KINDTKS_PROFILES_DIR is not set; kindtks runs from its image (see: docker run --rm %s)", imageRef)
	}
	return dir, nil
}
```

and change the start of `listCmd.RunE` to:

```go
		profileDir, err := profileDir()
		if err != nil {
			return err
		}
```

(drop `path/filepath` from imports if unused). In `create.go`, `delete.go`, `config.go`, replace `dir := profileDir()` with:

```go
		dir, err := profileDir()
		if err != nil {
			return err
		}
```

The following `p, err := profile.Load(dir, name)` line still compiles (`p` is new) and stays as is.

In `help.go`, the profile branch becomes:

```go
		dir, err := profileDir()
		if err != nil {
			return err
		}
		readmePath := filepath.Join(dir, name, "README.md")
```

and the no-arg branch prints `fmt.Print(readme())`. Add to `help.go`:

```go
// readme returns the project README with the run command pointing at the
// image kindtks is running from.
func readme() string {
	if img := os.Getenv("KINDTKS_IMAGE"); img != "" {
		return strings.ReplaceAll(projectReadme, imageRef, img)
	}
	return projectReadme
}
```

In `root.go`, the root `Run` becomes `fmt.Print(readme())` (remove `cmd.Help()` and the `/.dockerenv` install hint; drop `os` import if unused).

In `run.go`, delete the two env lines `KINDTKS_DATA_DIR=...` and `KINDTKS_CHARTS_DIR=...` (profile scripts inherit `KINDTKS_CHARTS_DIR` from the environment).

Delete `internal/cmd/install.go`.

In `registries.go`, replace the `stateDir` comment with:

```go
// stateDir is kindtks' XDG state dir. Its path must be the same on the host
// and in the container, because Kind mounts files from it into nodes.
```

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A internal/cmd
git commit -m "feat: read profiles from KINDTKS_PROFILES_DIR and drop install"
```

---

### Task 2: Mount checks before create/delete

**Files:**
- Create: `internal/cmd/mounts.go`, `internal/cmd/mounts_test.go`
- Modify: `internal/cmd/create.go`, `internal/cmd/delete.go`

**Interfaces:**
- Consumes: `imageRef`, `stateDir()`.
- Produces: `func checkMounts(dockerOK func() error) error`, `func dockerReachable() error`, `func checkConfigVisible(path string) error`.

- [ ] **Step 1: Write the failing tests** — `internal/cmd/mounts_test.go`:

```go
package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func okDocker() error { return nil }

func inImage(t *testing.T) string {
	t.Helper()
	t.Setenv("KINDTKS_IMAGE", "ghcr.io/null-ptr-exception/kindtks:0.6.0")
	t.Setenv("XDG_STATE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestCheckMounts_SkippedOutsideImage(t *testing.T) {
	t.Setenv("KINDTKS_IMAGE", "")
	t.Setenv("HOME", "/")
	if err := checkMounts(func() error { return errors.New("no docker") }); err != nil {
		t.Fatal(err)
	}
}

func TestCheckMounts_AllPresent(t *testing.T) {
	home := inImage(t)
	os.MkdirAll(filepath.Join(home, ".kube"), 0755)
	os.MkdirAll(filepath.Join(home, ".local", "state", "kindtks"), 0755)
	if err := checkMounts(okDocker); err != nil {
		t.Fatal(err)
	}
}

func TestCheckMounts_Failures(t *testing.T) {
	home := inImage(t)
	err := checkMounts(func() error { return errors.New("permission denied") })
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	for _, want := range []string{
		filepath.Join(home, ".kube"),
		filepath.Join(home, ".local", "state", "kindtks"),
		"docker socket",
		"permission denied",
		"docker run --rm ghcr.io/null-ptr-exception/kindtks:0.6.0",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestCheckMounts_RootHome(t *testing.T) {
	inImage(t)
	t.Setenv("HOME", "/")
	err := checkMounts(okDocker)
	if err == nil || !strings.Contains(err.Error(), `-e HOME="$HOME"`) {
		t.Fatalf("want HOME hint, got %v", err)
	}
}

func TestCheckConfigVisible(t *testing.T) {
	inImage(t)
	err := checkConfigVisible("/nonexistent/my.yaml")
	if err == nil || !strings.Contains(err.Error(), "mount") {
		t.Fatalf("want mount hint, got %v", err)
	}
	f := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(f, nil, 0644)
	if err := checkConfigVisible(f); err != nil {
		t.Fatal(err)
	}
	if err := checkConfigVisible(""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KINDTKS_IMAGE", "")
	if err := checkConfigVisible("/nonexistent/my.yaml"); err != nil {
		t.Fatalf("outside image the normal missing-file error applies later, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run 'TestCheck' -v`
Expected: compile failure (undefined `checkMounts`, `checkConfigVisible`).

- [ ] **Step 3: Implement** — `internal/cmd/mounts.go`:

```go
package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runHint points at the full docker run command, printed by running the
// image without arguments.
func runHint() string {
	return fmt.Sprintf("Run `docker run --rm %s` for the full command.", os.Getenv("KINDTKS_IMAGE"))
}

// checkMounts reports what the docker run command is missing. It only
// applies inside the image (KINDTKS_IMAGE set).
func checkMounts(dockerOK func() error) error {
	if os.Getenv("KINDTKS_IMAGE") == "" {
		return nil
	}
	var problems []string
	home := os.Getenv("HOME")
	if !filepath.IsAbs(home) || home == "/" {
		problems = append(problems, `HOME is not your host home directory; pass -e HOME="$HOME"`)
	} else {
		for _, dir := range []string{filepath.Join(home, ".kube"), stateDir()} {
			if err := writableDir(dir); err != nil {
				problems = append(problems, fmt.Sprintf("%s is not mounted writable at the same path (create it on the host first): %v", dir, err))
			}
		}
	}
	if err := dockerOK(); err != nil {
		problems = append(problems, fmt.Sprintf("docker socket is not usable (mount /var/run/docker.sock, add its group with --group-add): %v", err))
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s\n\n%s", strings.Join(problems, "\n"), runHint())
}

func writableDir(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errors.New("not a directory")
	}
	f, err := os.CreateTemp(dir, ".kindtks-check-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

func dockerReachable() error {
	var stderr bytes.Buffer
	c := exec.Command("docker", "version")
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// checkConfigVisible explains a --config file missing inside the image,
// which almost always means its directory is not mounted.
func checkConfigVisible(path string) error {
	if path == "" || os.Getenv("KINDTKS_IMAGE") == "" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config file %s not found inside the container; mount its directory (the documented command mounts $PWD)\n\n%s", path, runHint())
	}
	return nil
}
```

In `create.go`, add `SilenceUsage: true` to `createCmd`, and at the start of `RunE` (before `profileDir()`):

```go
		if err := checkMounts(dockerReachable); err != nil {
			return err
		}
		if err := checkConfigVisible(configFile); err != nil {
			return err
		}
```

In `delete.go`, add `SilenceUsage: true` to `deleteCmd` and at the start of `RunE`:

```go
		if err := checkMounts(dockerReachable); err != nil {
			return err
		}
```

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd
git commit -m "feat: check docker run mounts before create and delete"
```

---

### Task 3: Image with pinned tools

**Files:**
- Modify: `Dockerfile`

**Interfaces:**
- Consumes: `KINDTKS_PROFILES_DIR`, `KINDTKS_IMAGE` semantics from Tasks 1-2.
- Produces: image with `/usr/local/bin/{kindtks,kind,kubectl,helm,docker}`, `bash`, `envsubst`, `/profiles`, `/charts`, env `KINDTKS_PROFILES_DIR=/profiles KINDTKS_CHARTS_DIR=/charts KINDTKS_IMAGE=ghcr.io/null-ptr-exception/kindtks:${VERSION}`.

- [ ] **Step 1: Replace `Dockerfile`** with:

```dockerfile
# Every tool is pinned; bump a version together with its SHA-256.
ARG GO_IMAGE=golang:1.24.13@sha256:d2d2bc1c84f7e60d7d2438a3836ae7d0c847f4888464e7ec9ba3a1339a1ee804
ARG ALPINE_IMAGE=alpine:3.20.10@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
# apk keeps only the latest revision per branch: a vanished pin fails the
# build on purpose, as the signal to bump it.
ARG BASH_APK=bash=5.2.26-r0
ARG GETTEXT_APK=gettext=0.22.5-r0

FROM ${GO_IMAGE} AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /kindtks .

FROM ${ALPINE_IMAGE} AS tools
ARG KIND_VERSION=0.33.0
ARG KIND_SHA256=aee6151561422756b764a4ae28e7f44cda5af5a9eead3cc9985112b1de8d8e0d
ARG KUBECTL_VERSION=1.33.1
ARG KUBECTL_SHA256=5de4e9f2266738fd112b721265a0c1cd7f4e5208b670f811861f699474a100a3
ARG HELM_VERSION=3.21.0
ARG HELM_SHA256=0093eb572e3d2380f094df162ddb525e219249de88957afe24cfbb19632acd36
ARG DOCKER_VERSION=29.2.1
ARG DOCKER_SHA256=995b1d0b51e96d551a3b49c552c0170bc6ce9f8b9e0866b8c15bbc67d1cf93a3
WORKDIR /dl
RUN mkdir /out \
 && wget -qO kind "https://kind.sigs.k8s.io/dl/v${KIND_VERSION}/kind-linux-amd64" \
 && echo "${KIND_SHA256}  kind" | sha256sum -c - \
 && install -m 0755 kind /out/kind \
 && wget -qO kubectl "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
 && echo "${KUBECTL_SHA256}  kubectl" | sha256sum -c - \
 && install -m 0755 kubectl /out/kubectl \
 && wget -qO helm.tgz "https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz" \
 && echo "${HELM_SHA256}  helm.tgz" | sha256sum -c - \
 && tar xzf helm.tgz linux-amd64/helm \
 && install -m 0755 linux-amd64/helm /out/helm \
 && wget -qO docker.tgz "https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_VERSION}.tgz" \
 && echo "${DOCKER_SHA256}  docker.tgz" | sha256sum -c - \
 && tar xzf docker.tgz docker/docker \
 && install -m 0755 docker/docker /out/docker

FROM ${ALPINE_IMAGE} AS charts
ARG BASH_APK
RUN apk add --no-cache "${BASH_APK}"
COPY --from=tools /out/helm /usr/local/bin/helm
COPY scripts/pull-charts.sh /pull-charts.sh
RUN /pull-charts.sh /charts

FROM ${ALPINE_IMAGE}
ARG BASH_APK
ARG GETTEXT_APK
RUN apk add --no-cache "${BASH_APK}" "${GETTEXT_APK}"
COPY --from=tools /out/ /usr/local/bin/
COPY --from=builder /kindtks /usr/local/bin/kindtks
COPY profiles/ /profiles/
COPY --from=charts /charts/ /charts/
ARG VERSION=dev
ENV KINDTKS_PROFILES_DIR=/profiles \
    KINDTKS_CHARTS_DIR=/charts \
    KINDTKS_IMAGE=ghcr.io/null-ptr-exception/kindtks:${VERSION}
ENTRYPOINT ["kindtks"]
```

- [ ] **Step 2: Build** (slow mirrors can take 10+ minutes; use a 20-minute timeout, foreground)

Run: `docker build -t kindtks:dev .`
Expected: success, all four `sha256sum -c` lines print `OK`.

- [ ] **Step 3: Verify tools and README output**

Run:
```bash
docker run --rm --entrypoint sh kindtks:dev -c 'kind version; kubectl version --client | head -1; helm version --short; docker --version; bash --version | head -1; command -v envsubst; ls /charts'
docker run --rm kindtks:dev | grep -c 'kindtks:dev'
```
Expected: kind v0.33.0, kubectl v1.33.1, helm v3.21.0, Docker 29.2.1, bash 5.2.26, `/usr/bin/envsubst`, six chart `.tgz` files; the grep count is ≥ 1 only after Task 5 rewrites the README (before that it prints 0 — acceptable at this step; note it in the report).

- [ ] **Step 4: Commit**

```bash
git add Dockerfile
git commit -m "build: ship pinned kind, kubectl, helm, docker CLI in the image"
```

---

### Task 4: Tests use the env dir; docker-run e2e

**Files:**
- Modify: `test/e2e/gen1.bats`, `test/e2e/registry-hosts.bats`
- Create: `test/e2e/docker-run.bats`

**Interfaces:**
- Consumes: `KINDTKS_PROFILES_DIR`, `KINDTKS_CHARTS_DIR`, image `kindtks:dev` behaviour, mount checks.

- [ ] **Step 1: `gen1.bats` setup** — replace the block from `mkdir -p "${E2E_HOME}/.local/bin" ...` through the `sed -i ...install.sh"` line with:

```bash
  mkdir -p "${E2E_HOME}/.local/bin"

  # Build kindtks binary
  go build -o "${E2E_HOME}/.local/bin/kindtks" "${BATS_TEST_DIRNAME}/../../"

  # Profiles and charts, as the image provides them
  export KINDTKS_PROFILES_DIR="${BATS_FILE_TMPDIR}/profiles"
  cp -r "${BATS_TEST_DIRNAME}/../../profiles" "$KINDTKS_PROFILES_DIR"

  # Pull charts if not cached
  local charts_cache="${BATS_TEST_DIRNAME}/../../.cache/charts"
  if [ ! -d "$charts_cache" ] || [ -z "$(ls "$charts_cache"/*.tgz 2>/dev/null)" ]; then
    mkdir -p "$charts_cache"
    "${BATS_TEST_DIRNAME}/../../scripts/pull-charts.sh" "$charts_cache"
  fi
  export KINDTKS_CHARTS_DIR="$charts_cache"

  # Use a unique cluster name to avoid collisions
  sed -i "s/CLUSTER_NAME=\"gen1\"/CLUSTER_NAME=\"${CLUSTER_NAME}\"/" \
    "${KINDTKS_PROFILES_DIR}/gen1/install.sh"
```

- [ ] **Step 2: `registry-hosts.bats` setup** — change

```bash
  local profile_dir="${E2E_HOME}/.local/share/kindtks/profiles/${PROFILE}"
```
to
```bash
  export KINDTKS_PROFILES_DIR="${BATS_FILE_TMPDIR}/profiles"
  local profile_dir="${KINDTKS_PROFILES_DIR}/${PROFILE}"
```

- [ ] **Step 3: Create `test/e2e/docker-run.bats`:**

```bash
#!/usr/bin/env bats
# Runs kindtks the documented way: only `docker run` of the image. Checks the
# cluster comes up, files written on the host belong to the user, the state
# dir reaches the nodes by its host path, and missing mounts get a hint.

load '../helpers/test_helper'

IMAGE="kindtks:docker-run-e2e"
CLUSTER_NAME="kindtks-dr-e2e"
PROFILE="dr-e2e"

setup_file() {
  docker build -t "$IMAGE" "${BATS_TEST_DIRNAME}/../../"

  export H="${BATS_FILE_TMPDIR}/home"
  export TP="${BATS_FILE_TMPDIR}/test-profiles"
  mkdir -p "$H/.kube" "$H/.local/state/kindtks" "$H/work" "$TP/$PROFILE"

  cat > "$TP/$PROFILE/install.sh" <<EOF
REQUIRES="kind kubectl helm envsubst"

create() {
  kind create cluster --name ${CLUSTER_NAME} --image kindest/node:v1.24.17 --config "\$KINDTKS_KIND_CONFIG"
  kubectl --context kind-${CLUSTER_NAME} get nodes
}

delete() {
  kind delete cluster --name ${CLUSTER_NAME}
}
EOF
  cat > "$TP/$PROFILE/kind-config.yaml" <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
EOF
  : > "$TP/$PROFILE/config.yaml"

  cat > "$H/work/my.yaml" <<'EOF'
registries:
  docker.io:
    hosts: |
      server = "https://registry-1.docker.io"
EOF
}

teardown_file() {
  kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
  docker rmi "$IMAGE" >/dev/null 2>&1 || true
}

# The documented command, with HOME pointed at the test home and the test
# profile mounted in place of the image's profiles.
kt() {
  (cd "$H/work" && docker run --rm --network host \
    --user "$(id -u):$(id -g)" --group-add "$(stat -c %g /var/run/docker.sock)" \
    --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
    -e HOME="$H" \
    --mount type=bind,source="$H/.kube",target="$H/.kube" \
    --mount type=bind,source="$H/.local/state/kindtks",target="$H/.local/state/kindtks" \
    --mount type=bind,source="$PWD",target="$PWD",readonly -w "$PWD" \
    --mount type=bind,source="$TP",target=/test-profiles,readonly -e KINDTKS_PROFILES_DIR=/test-profiles \
    "$IMAGE" "$@")
}

@test "no arguments prints the README with this image" {
  run docker run --rm "$IMAGE"
  assert_success
  assert_output --partial "docker run --rm --network host"
  # KINDTKS_IMAGE is the published reference with this build's VERSION (dev).
  assert_output --partial "ghcr.io/null-ptr-exception/kindtks:dev create gen1"
}

@test "missing kube mount is reported with a hint" {
  run docker run --rm --user "$(id -u):$(id -g)" \
    --group-add "$(stat -c %g /var/run/docker.sock)" \
    --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
    -e HOME="$H" \
    --mount type=bind,source="$H/.local/state/kindtks",target="$H/.local/state/kindtks" \
    --mount type=bind,source="$TP",target=/test-profiles,readonly -e KINDTKS_PROFILES_DIR=/test-profiles \
    "$IMAGE" create "$PROFILE"
  assert_failure
  assert_output --partial "$H/.kube"
  assert_output --partial "docker run --rm ghcr.io/null-ptr-exception/kindtks:dev"
}

@test "create through docker run" {
  run kt create "$PROFILE" --config my.yaml
  assert_success
  assert_output --partial "${CLUSTER_NAME}-control-plane"
}

@test "host files belong to the user" {
  [ "$(stat -c %u "$H/.kube/config")" = "$(id -u)" ]
  [ "$(stat -c %u "$H/.local/state/kindtks/$PROFILE/certs.d/docker.io/hosts.toml")" = "$(id -u)" ]
}

@test "state dir reaches the node by its host path" {
  run docker exec "${CLUSTER_NAME}-control-plane" cat /etc/containerd/certs.d/docker.io/hosts.toml
  assert_success
  assert_output --partial "registry-1.docker.io"
}

@test "host kubectl uses the written kubeconfig" {
  run kubectl --kubeconfig "$H/.kube/config" --context "kind-${CLUSTER_NAME}" get nodes
  assert_success
}

@test "delete through docker run" {
  run kt delete "$PROFILE"
  assert_success
  [ ! -e "$H/.local/state/kindtks/$PROFILE" ]
  run kind get clusters
  refute_output --partial "$CLUSTER_NAME"
}
```

Note: the first test needs the README rewrite from Task 5; it is expected to fail until then. Run only the other tests in this task.

- [ ] **Step 4: Run**

Run (foreground, 20-minute timeout): `bats test/e2e/docker-run.bats -f 'missing|create|host|state|delete'`
Expected: 6 tests pass. Then `bats test/unit/` passes. (`gen1.bats` and `registry-hosts.bats` are run by the controller after Task 5.)

- [ ] **Step 5: Commit**

```bash
git add test/e2e
git commit -m "test: run e2e from KINDTKS_PROFILES_DIR and add docker run e2e"
```

---

### Task 5: Docs, release notes, air-gap tests

**Files:**
- Modify: `README.md`, `profiles/gen1/README.md`, `.github/workflows/release.yml`, `test/e2e/airgap-gen1.bats`, `test/llm/airgap-deploy.bats`

- [ ] **Step 1: README** — replace everything from `## Install` up to (not including) `## Usage` with:

````markdown
## Run

kindtks runs from its image; the host needs only Docker. Create the two directories once, then run any command:

```bash
mkdir -p ~/.kube ~/.local/state/kindtks
docker run --rm --network host \
  --user "$(id -u):$(id -g)" --group-add "$(stat -c %g /var/run/docker.sock)" \
  --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
  -e HOME="$HOME" \
  --mount type=bind,source="$HOME/.kube",target="$HOME/.kube" \
  --mount type=bind,source="$HOME/.local/state/kindtks",target="$HOME/.local/state/kindtks" \
  --mount type=bind,source="$PWD",target="$PWD",readonly -w "$PWD" \
  ghcr.io/null-ptr-exception/kindtks:latest create gen1
```

In the rest of this document, `kindtks <args>` is short for this command with `<args>` in place of `create gen1`.

| Part | Why |
|---|---|
| `--network host` | profile scripts reach the cluster API on `127.0.0.1` |
| docker socket, `--group-add` | Kind creates node containers on the host's Docker |
| `--user` | files written on the host belong to you |
| `-e HOME`, same-path mounts | the kubeconfig lands in your `~/.kube`; Kind mounts the state dir into nodes by its host path |
| `$PWD` mount | `--config` files in the current directory are visible; mount other directories the same way |
| `--mount` instead of `-v` | a missing directory fails instead of being created as root |

The image includes kind, kubectl, helm and every other tool the profiles use. To use the cluster afterwards from the host you still need your own kubectl (or run it from the image with `--entrypoint kubectl`).

Air-gapped hosts: push this image to your registry and use that reference instead.

Upgrading from 0.5.x: `kindtks install` is gone; `~/.local/bin/kindtks` and `~/.local/share/kindtks` are no longer used and can be deleted.
````

- [ ] **Step 2: gen1 README** — under `## Quick Start`, before the code block, add the line:

```markdown
`kindtks` below is short for the `docker run` command in the [top-level README](../../README.md#run).
```

- [ ] **Step 3: release.yml** — replace the `NOTES=$(printf ...)` line with:

```bash
          NOTES=$(printf '## Container Image\n\n```\ndocker pull %s\n```\n\nDigest: `%s`\n\n## Run\n\n```bash\ndocker run --rm %s\n```\n\nprints the README, including the full command to run kindtks.' "$IMAGE_REF" "$DIGEST" "$IMAGE_REF")
```

- [ ] **Step 4: airgap-gen1.bats**
  - In `clean VM state`, replace `sudo rm -rf ~/.local/bin/kindtks ~/.local/share/kindtks` with `rm -rf ~/.local/state/kindtks/gen1`.
  - Replace the whole `@test "install kindtks in VM"` with:

```bash
@test "pull kindtks image in VM" {
  run ssh_vm "docker pull ${VM_REGISTRY}/kindtks:airgap-test"
  assert_success
}
```

  - In `create gen1 cluster with registry auth`, change the heredoc opener from `<<'SSH'` to `<<SSH`, escape the existing `$` uses inside it (`\$PATH`, `\$HOME` — the inner `<<'EOF'` config block contains no `$`), and replace the line `kindtks create gen1 --config ~/airgap-config.yaml` with:

```bash
mkdir -p ~/.kube ~/.local/state/kindtks
cd ~
docker run --rm --network host \
  --user "\$(id -u):\$(id -g)" --group-add "\$(stat -c %g /var/run/docker.sock)" \
  --mount type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock \
  -e HOME="\$HOME" \
  --mount type=bind,source="\$HOME/.kube",target="\$HOME/.kube" \
  --mount type=bind,source="\$HOME/.local/state/kindtks",target="\$HOME/.local/state/kindtks" \
  --mount type=bind,source="\$PWD",target="\$PWD",readonly -w "\$PWD" \
  ${VM_REGISTRY}/kindtks:airgap-test create gen1 --config airgap-config.yaml
```

- [ ] **Step 5: test/llm/airgap-deploy.bats**
  - In `setup_file`, replace `sudo rm -rf ~/.local/bin/kindtks ~/.local/share/kindtks` with `rm -rf ~/.local/state/kindtks/gen1`.
  - In the prompt, replace step `2. Install the tool from the container image` with `2. Run the tool from the container image as its help output describes`.

- [ ] **Step 6: Verify**

Run:
```bash
go test ./...
docker build -t kindtks:dev . && docker run --rm kindtks:dev | grep -c 'ghcr.io/null-ptr-exception/kindtks:dev'
bash -n test/e2e/airgap-gen1.bats; bash -n test/llm/airgap-deploy.bats
git grep -nE "$PRIVATE_DOMAINS_RE" -- README.md profiles .github test docs || true  # PRIVATE_DOMAINS_RE: regex of private domains, kept outside the repo
```
Expected: tests pass; grep count ≥ 1; syntax checks silent; the last grep prints nothing.

- [ ] **Step 7: Commit**

```bash
git add README.md profiles/gen1/README.md .github/workflows/release.yml test/e2e/airgap-gen1.bats test/llm/airgap-deploy.bats
git commit -m "docs: document running kindtks with docker run"
```

---

### Controller verification after Task 5

- `bats test/e2e/docker-run.bats` (all 7), `bats test/e2e/registry-hosts.bats`, `bats test/e2e/gen1.bats`, `bats test/unit/`.
- `airgap-gen1.bats` when the air-gap lab is up.
