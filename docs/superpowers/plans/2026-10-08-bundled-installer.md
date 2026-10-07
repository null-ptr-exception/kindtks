# Bundled Tools and One-Line Installer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `docker run --rm <image> install-script | bash` installs kindtks plus pinned kind, kubectl and helm from the kindtks image; kindtks runs profile scripts on the host with those bundled tools.

**Architecture:** The image (`FROM scratch`) carries static `kindtks`, `kind`, `kubectl`, `helm`, profiles and charts, stamped with a unique build-ID label. `kindtks install-script` prints an embedded bash installer with the build ID filled in; the installer finds the image by label, `docker cp`s the files into `~/.local/bin` and `~/.local/share/kindtks/{bin,profiles,charts}`, and offers bundled tools where the host lacks them or has older ones. kindtks prepends `~/.local/share/kindtks/bin` to `PATH`. gen1 generates its TLS cert with a new hidden `kindtks tls-cert` instead of `openssl`. The docker-run design's mount checks and run command are removed.

**Tech Stack:** Go 1.24 (cobra, crypto/x509), bash, Dockerfile, bats, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-08-bundled-installer-design.md`

## Global Constraints

- Work on branch `bundled-installer` (created from `main` by the controller). Never `git push`.
- Commit messages: `<type>: <description>`, types feat/fix/refactor/chore/docs/build/test; no mention of Claude, no Co-Authored-By, no "Generated with" footer. Commits are GPG-signed automatically; never disable signing.
- Public content (code, docs, tests) uses only `example.com` / `example.net` style names; never private or internal domains or hostnames.
- Data dir: `$HOME/.local/share/kindtks` with `bin/`, `profiles/`, `charts/`. Default install destination: `$HOME/.local/bin`. State dir unchanged (`~/.local/state/kindtks`).
- Build-ID label key: `io.kindtks.build`; env `KINDTKS_BUILD_ID`; build arg `BUILD_ID` (default `dev`); installer placeholder `__KINDTKS_BUILD_ID__`.
- Bundled versions (pinned, unchanged): kind v0.33.0, kubectl v1.33.1, helm v3.21.0; Go builder `golang:1.24.13@sha256:d2d2bc1c84f7e60d7d2438a3836ae7d0c847f4888464e7ec9ba3a1339a1ee804`; alpine `alpine:3.20.10@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc`; apk `bash=5.2.26-r0` (charts stage only).
- Run long commands (docker build, bats e2e) in the foreground with your tool's maximum timeout; re-run on timeout (layers are cached). Never use run_in_background, Monitor, or `until … pgrep` wait loops.
- Kind clusters you did not create (`gen1`, `pacds`, others) belong to other people: never touch them.
- Never write to the real `~/.local/bin` or `~/.local/share/kindtks`: tests set `HOME` to a temp dir.

---

### Task 1: Data dir, bundled tools on PATH, remove docker-run pieces

**Files:**
- Create: `internal/cmd/dirs.go`
- Modify: `internal/cmd/list.go`, `internal/cmd/create.go`, `internal/cmd/delete.go`, `internal/cmd/config.go`, `internal/cmd/help.go`, `internal/cmd/root.go`, `internal/cmd/run.go`, `internal/cmd/run_test.go`, `internal/cmd/dirs_test.go` (rewrite)
- Delete: `internal/cmd/mounts.go`, `internal/cmd/mounts_test.go`, `test/e2e/docker-run.bats`

**Interfaces:**
- Produces: `func dataDir() string`, `func profileDir() string`, `func chartsDir() string`, `func prependToolsDir()`. `projectReadme` printed as is.

- [ ] **Step 1: Write the failing tests** — replace `internal/cmd/dirs_test.go` with:

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KINDTKS_PROFILES_DIR", "")
	if got, want := profileDir(), filepath.Join(home, ".local", "share", "kindtks", "profiles"); got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
	t.Setenv("KINDTKS_PROFILES_DIR", "/p")
	if got := profileDir(); got != "/p" {
		t.Errorf("override: got %q", got)
	}
}

func TestChartsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KINDTKS_CHARTS_DIR", "")
	if got, want := chartsDir(), filepath.Join(home, ".local", "share", "kindtks", "charts"); got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
	t.Setenv("KINDTKS_CHARTS_DIR", "/c")
	if got := chartsDir(); got != "/c" {
		t.Errorf("override: got %q", got)
	}
}

func TestPrependToolsDir_Once(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")
	prependToolsDir()
	prependToolsDir()
	want := filepath.Join(home, ".local", "share", "kindtks", "bin") + ":/usr/bin:/bin"
	if got := os.Getenv("PATH"); got != want {
		t.Errorf("got %q, want %q", got, want)
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

In `internal/cmd/run_test.go`, `TestRunProfileFunc_ExportsProfileConfig`: add `"KINDTKS_CHARTS_DIR=" + chartsDir()` to the slice of wanted env lines (the `for _, want := range []string{...}` loop).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run 'TestProfileDir|TestChartsDir|TestPrependToolsDir|TestRunProfileFunc' -v`
Expected: compile failure (`chartsDir`, `prependToolsDir` undefined; `profileDir` returns two values).

- [ ] **Step 3: Implement**

Create `internal/cmd/dirs.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

// dataDir holds the bundled tools, profiles and charts that the installer
// copies out of the kindtks image.
func dataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "kindtks")
}

// profileDir is $KINDTKS_PROFILES_DIR when set (tests, development), else
// the installed profiles.
func profileDir() string {
	if dir := os.Getenv("KINDTKS_PROFILES_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(dataDir(), "profiles")
}

// chartsDir is exported to profile scripts as KINDTKS_CHARTS_DIR.
func chartsDir() string {
	if dir := os.Getenv("KINDTKS_CHARTS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(dataDir(), "charts")
}

// prependToolsDir puts the bundled kind, kubectl and helm first on PATH, so
// REQUIRES checks and profile scripts use the versions kindtks was tested with.
func prependToolsDir() {
	bin := filepath.Join(dataDir(), "bin")
	path := os.Getenv("PATH")
	if first, _, _ := strings.Cut(path, string(os.PathListSeparator)); first == bin {
		return
	}
	os.Setenv("PATH", bin+string(os.PathListSeparator)+path)
}
```

In `internal/cmd/list.go`: delete `imageRef` and the old `profileDir()`; the start of `RunE` becomes `profileDir := profileDir()` (no error). Drop imports that become unused.

In `create.go`, `delete.go`, `config.go`, `help.go`: replace

```go
		dir, err := profileDir()
		if err != nil {
			return err
		}
```

with `dir := profileDir()`, keeping the following `p, err := profile.Load(dir, name)` (in `help.go`, `readmePath := filepath.Join(dir, name, "README.md")`). In `create.go` and `delete.go` also delete the `checkMounts(dockerReachable)` and `checkConfigVisible(configFile)` blocks.

In `help.go`: delete `readme()`; the no-argument branch prints `fmt.Print(projectReadme)`. Drop unused imports.

In `root.go`: the root `Run` prints `fmt.Print(projectReadme)`; `Execute()` calls `prependToolsDir()` before `rootCmd.Execute()`.

In `run.go`, add `"KINDTKS_CHARTS_DIR="+chartsDir(),` to the env list after `"KINDTKS_PROFILE_DIR="+p.Dir,` (a later duplicate key wins in `exec.Cmd.Env`).

Delete `internal/cmd/mounts.go`, `internal/cmd/mounts_test.go`, `test/e2e/docker-run.bats`.

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./... && grep -rn 'KINDTKS_IMAGE\|checkMounts\|imageRef' --include=*.go . || echo clean`
Expected: PASS and `clean`.

- [ ] **Step 5: Commit**

```bash
git add -A internal/cmd test/e2e/docker-run.bats
git commit -m "feat: run profiles with bundled tools from the data dir"
```

---

### Task 2: `kindtks tls-cert`, gen1 drops openssl

**Files:**
- Create: `internal/cmd/tlscert.go`, `internal/cmd/tlscert_test.go`
- Modify: `profiles/gen1/install.sh` (the `openssl req` block in `install_istio`), `test/unit/gen1_profile.bats`

**Interfaces:**
- Produces: hidden command `kindtks tls-cert <dir> <dns-name>...` writing `<dir>/tls.crt` (0644) and `<dir>/tls.key` (0600); `func writeSelfSignedCert(dir string, names []string, now time.Time) error`.

- [ ] **Step 1: Write the failing Go test** — `internal/cmd/tlscert_test.go`:

```go
package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestWriteSelfSignedCert(t *testing.T) {
	dir := t.TempDir()
	names := []string{"kindtks.localhost", "*.kindtks.localhost", "kindtks.local", "*.kindtks.local"}
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	if err := writeSelfSignedCert(dir, names, now); err != nil {
		t.Fatal(err)
	}

	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"))
	if err != nil {
		t.Fatalf("cert and key do not match: %v", err)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if cert.Subject.CommonName != "kindtks.localhost" {
		t.Errorf("CN = %q", cert.Subject.CommonName)
	}
	if !slices.Equal(cert.DNSNames, names) {
		t.Errorf("SANs = %v", cert.DNSNames)
	}
	if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
		t.Errorf("not self-signed: %v", err)
	}
	if !cert.NotAfter.Equal(now.AddDate(10, 0, 0)) {
		t.Errorf("NotAfter = %v", cert.NotAfter)
	}
	fi, err := os.Stat(filepath.Join(dir, "tls.key"))
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Errorf("key mode = %v, %v", fi.Mode().Perm(), err)
	}
}

func TestTLSCertCommand_Hidden(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"tls-cert"})
	if err != nil || c.Name() != "tls-cert" || !c.Hidden {
		t.Fatalf("tls-cert not registered as hidden: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run 'TestWriteSelfSignedCert|TestTLSCertCommand' -v`
Expected: compile failure (`writeSelfSignedCert` undefined).

- [ ] **Step 3: Implement** — `internal/cmd/tlscert.go`:

```go
package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// writeSelfSignedCert writes tls.crt and tls.key to dir: a self-signed
// ECDSA P-256 certificate for names (CN = names[0]), valid for ten years.
func writeSelfSignedCert(dir string, names []string, now time.Time) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: names[0]},
		DNSNames:              names,
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "tls.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600)
}

var tlsCertCmd = &cobra.Command{
	Use:          "tls-cert <dir> <dns-name>...",
	Short:        "Write a self-signed TLS certificate and key (for profile scripts)",
	Hidden:       true,
	Args:         cobra.MinimumNArgs(2),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return writeSelfSignedCert(args[0], args[1:], time.Now())
	},
}

func init() {
	rootCmd.AddCommand(tlsCertCmd)
}
```

- [ ] **Step 4: Switch gen1** — in `profiles/gen1/install.sh`, `install_istio`, replace the four-line `openssl req … 2>/dev/null` command with:

```bash
  "$KINDTKS_BIN" tls-cert "$certdir" kindtks.localhost '*.kindtks.localhost' kindtks.local '*.kindtks.local'
```

(the `certdir=$(mktemp -d)`, `kubectl … create secret tls …` and `rm -rf "$certdir"` lines stay).

- [ ] **Step 5: gen1 bats tests** — append to `test/unit/gen1_profile.bats`:

```bash
@test "install_istio creates the TLS cert with kindtks tls-cert" {
  export KINDTKS_ARGS_FILE="${BATS_TEST_TMPDIR}/kindtks_args"
  cat > "${MOCK_BIN}/fake-kindtks" <<'MOCK'
#!/usr/bin/env bash
echo "$@" >> "$KINDTKS_ARGS_FILE"
MOCK
  chmod +x "${MOCK_BIN}/fake-kindtks"
  export KINDTKS_BIN="${MOCK_BIN}/fake-kindtks"

  run install_istio
  assert_success
  run grep '^tls-cert ' "$KINDTKS_ARGS_FILE"
  assert_output --regexp '^tls-cert /.+ kindtks\.localhost \*\.kindtks\.localhost kindtks\.local \*\.kindtks\.local$'
}

@test "no profile calls openssl" {
  run grep -rn 'openssl' "${BATS_TEST_DIRNAME}/../../profiles"
  assert_failure
}
```

- [ ] **Step 6: Run tests**

Run: `go vet ./... && go test ./... && bats test/unit/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/cmd/tlscert.go internal/cmd/tlscert_test.go profiles/gen1/install.sh test/unit/gen1_profile.bats
git commit -m "feat: generate gen1's TLS cert with kindtks tls-cert instead of openssl"
```

---

### Task 3: Installer script and `kindtks install-script`

**Files:**
- Create: `internal/cmd/installer.sh`, `internal/cmd/installscript.go`, `internal/cmd/installscript_test.go`, `test/unit/installer.bats`

**Interfaces:**
- Consumes: data dir layout from Task 1.
- Produces: `kindtks install-script` (prints the installer with `__KINDTKS_BUILD_ID__` replaced by `$KINDTKS_BUILD_ID`); installer flags `--yes`, `--no-tools`, `--dest DIR`; test hook env `KINDTKS_INSTALLER_TTY` (default `/dev/tty`).

- [ ] **Step 1: Write the Go test** — `internal/cmd/installscript_test.go`:

```go
package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func runInstallScript(t *testing.T, id string) (string, error) {
	t.Helper()
	t.Setenv("KINDTKS_BUILD_ID", id)
	var out bytes.Buffer
	installScriptCmd.SetOut(&out)
	err := installScriptCmd.RunE(installScriptCmd, nil)
	return out.String(), err
}

func TestInstallScript_FillsBuildID(t *testing.T) {
	out, err := runInstallScript(t, "0.6.0-abc1234-20261008T000000Z")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `BUILD_ID="0.6.0-abc1234-20261008T000000Z"`) || strings.Contains(out, "__KINDTKS_BUILD_ID__") {
		t.Errorf("build ID not filled in:\n%s", out)
	}
	if !strings.HasPrefix(out, "#!/usr/bin/env bash\n") {
		t.Errorf("not a bash script")
	}
}

func TestInstallScript_RejectsMissingOrUnsafeID(t *testing.T) {
	for _, id := range []string{"", "dev", `x"; rm -rf ~; "`} {
		if _, err := runInstallScript(t, id); err == nil {
			t.Errorf("build ID %q: want error", id)
		}
	}
}
```

- [ ] **Step 2: Write the bats test** — `test/unit/installer.bats`:

```bash
#!/usr/bin/env bats
# The installer against a fake docker whose "image" is a directory, and fake
# kind/kubectl/helm that print set versions.

load '../helpers/test_helper'

fake_tool() { # fake_tool DIR NAME VERSION
  local out
  case "$2" in
    kind) out="kind $3 go1.26.7 linux/amd64" ;;
    kubectl) out="Client Version: $3" ;;
    helm) out="$3+gabcdef0" ;;
  esac
  mkdir -p "$1"
  printf '#!/usr/bin/env bash\necho "%s"\n' "$out" > "$1/$2"
  chmod +x "$1/$2"
}

setup() {
  for t in kind kubectl helm; do
    if PATH=/usr/bin:/bin command -v "$t" >/dev/null; then
      skip "a real $t in /usr/bin or /bin would interfere"
    fi
  done

  export HOME="${BATS_TEST_TMPDIR}/home"
  mkdir -p "$HOME"
  DEST="$HOME/.local/bin"
  DATA="$HOME/.local/share/kindtks"

  export FAKE_IMAGE_ROOT="${BATS_TEST_TMPDIR}/image"
  fake_tool "$FAKE_IMAGE_ROOT/usr/local/bin" kind v0.33.0
  fake_tool "$FAKE_IMAGE_ROOT/usr/local/bin" kubectl v1.33.1
  fake_tool "$FAKE_IMAGE_ROOT/usr/local/bin" helm v3.21.0
  printf '#!/usr/bin/env bash\necho kindtks\n' > "$FAKE_IMAGE_ROOT/usr/local/bin/kindtks"
  chmod +x "$FAKE_IMAGE_ROOT/usr/local/bin/kindtks"
  mkdir -p "$FAKE_IMAGE_ROOT/profiles/gen1" "$FAKE_IMAGE_ROOT/charts"
  echo 'REQUIRES=""' > "$FAKE_IMAGE_ROOT/profiles/gen1/install.sh"
  touch "$FAKE_IMAGE_ROOT/charts/c-1.0.0.tgz"

  MOCK_BIN="${BATS_TEST_TMPDIR}/mock"
  mkdir -p "$MOCK_BIN"
  export DOCKER_LOG="${BATS_TEST_TMPDIR}/docker.log"
  cat > "$MOCK_BIN/docker" <<'MOCK'
#!/usr/bin/env bash
echo "$*" >> "$DOCKER_LOG"
case "$1" in
  info)
    if [ -n "${FAKE_DOCKER_DOWN:-}" ]; then echo "Cannot connect to the Docker daemon" >&2; exit 1; fi ;;
  images) [ -n "${FAKE_IMAGE_MISSING:-}" ] || echo "fakeimage123" ;;
  create) echo "fakecid" ;;
  cp) rm -rf "$3"; cp -r "${FAKE_IMAGE_ROOT}${2#fakecid:}" "$3" ;;
  rm) ;;
esac
MOCK
  chmod +x "$MOCK_BIN/docker"

  HOSTBIN="${BATS_TEST_TMPDIR}/hostbin"
  mkdir -p "$HOSTBIN"
  export PATH="$MOCK_BIN:/usr/bin:/bin"
  export KINDTKS_INSTALLER_TTY="${BATS_TEST_TMPDIR}/no-such-tty"

  INSTALLER="${BATS_TEST_TMPDIR}/install.sh"
  sed 's/__KINDTKS_BUILD_ID__/test-build-1/' "${BATS_TEST_DIRNAME}/../../internal/cmd/installer.sh" > "$INSTALLER"
}

@test "installs kindtks and the private copies" {
  run bash "$INSTALLER" --no-tools
  assert_success
  [ -x "$DEST/kindtks" ]
  for t in kind kubectl helm; do [ -x "$DATA/bin/$t" ]; done
  [ -f "$DATA/profiles/gen1/install.sh" ]
  [ -f "$DATA/charts/c-1.0.0.tgz" ]
  run cat "$DOCKER_LOG"
  assert_output --partial "images -q --filter label=io.kindtks.build=test-build-1"
  assert_output --partial "rm fakecid"
}

@test "upgrade replaces the private directories whole" {
  mkdir -p "$DATA/profiles/removed-profile"
  run bash "$INSTALLER" --no-tools
  assert_success
  [ ! -e "$DATA/profiles/removed-profile" ]
}

@test "missing tools are installed with --yes" {
  run bash "$INSTALLER" --yes
  assert_success
  assert_output --partial "Install kubectl v1.33.1 to $DEST? [Y/n] yes (--yes)"
  for t in kind kubectl helm; do [ -x "$DEST/$t" ]; done
}

@test "missing tools are skipped without a terminal" {
  run bash "$INSTALLER"
  assert_success
  assert_output --partial "Install kubectl v1.33.1 to $DEST? [Y/n] skipped: no terminal"
  assert_output --partial "--yes"
  [ ! -e "$DEST/kubectl" ]
}

@test "answers are read from the terminal" {
  printf 'y\nn\n\n' > "${BATS_TEST_TMPDIR}/tty"
  KINDTKS_INSTALLER_TTY="${BATS_TEST_TMPDIR}/tty" run bash "$INSTALLER"
  assert_success
  [ -x "$DEST/kind" ]
  [ ! -e "$DEST/kubectl" ]
  [ -x "$DEST/helm" ]
}

@test "older tool is offered when the destination comes first on PATH" {
  fake_tool "$HOSTBIN" kubectl v1.29.3
  PATH="$MOCK_BIN:$DEST:$HOSTBIN:/usr/bin:/bin" run bash "$INSTALLER" --yes
  assert_success
  assert_output --partial "kubectl v1.29.3 at $HOSTBIN/kubectl is older than bundled v1.33.1. Install v1.33.1 to $DEST ($DEST comes first on PATH)?"
  [ -x "$DEST/kubectl" ]
  run "$HOSTBIN/kubectl"
  assert_output "Client Version: v1.29.3"
}

@test "older tool warns when the destination comes later on PATH" {
  fake_tool "$HOSTBIN" kubectl v1.29.3
  PATH="$MOCK_BIN:$HOSTBIN:$DEST:/usr/bin:/bin" run bash "$INSTALLER" --yes
  assert_success
  assert_output --partial "($DEST comes after $HOSTBIN on PATH; the new copy will not take effect until PATH is changed)?"
}

@test "newer tool is only reported" {
  fake_tool "$HOSTBIN" kubectl v1.34.0
  PATH="$MOCK_BIN:$HOSTBIN:/usr/bin:/bin" run bash "$INSTALLER" --yes
  assert_success
  assert_output --partial "kubectl v1.34.0 found at $HOSTBIN/kubectl (bundled v1.33.1)"
  [ ! -e "$DEST/kubectl" ]
}

@test "--dest installs elsewhere and prints the PATH line" {
  run bash "$INSTALLER" --no-tools --dest "${BATS_TEST_TMPDIR}/custom"
  assert_success
  [ -x "${BATS_TEST_TMPDIR}/custom/kindtks" ]
  assert_output --partial "${BATS_TEST_TMPDIR}/custom is not on PATH"
  assert_output --partial "export PATH=\"${BATS_TEST_TMPDIR}/custom:\$PATH\""
}

@test "image not found" {
  FAKE_IMAGE_MISSING=1 run bash "$INSTALLER" --yes
  assert_failure 1
  assert_output --partial "kindtks image with build test-build-1 not found locally"
}

@test "docker not usable" {
  FAKE_DOCKER_DOWN=1 run bash "$INSTALLER" --yes
  assert_failure 1
  assert_output --partial "docker is not usable"
  assert_output --partial "Cannot connect to the Docker daemon"
}

@test "unknown option" {
  run bash "$INSTALLER" --bogus
  assert_failure 2
  assert_output --partial "unknown option: --bogus"
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/cmd/ -run TestInstallScript -v; bats test/unit/installer.bats`
Expected: Go compile failure (`installScriptCmd` undefined); bats fails (`installer.sh` missing).

- [ ] **Step 4: Write the installer** — `internal/cmd/installer.sh`:

```bash
#!/usr/bin/env bash
# kindtks installer for one image build. Printed by
#   docker run --rm <kindtks image> install-script | bash -s -- [--yes] [--no-tools] [--dest DIR]
# It finds the image on this host by its build label, copies kindtks and the
# bundled kind, kubectl and helm out of it, and offers the bundled tools
# where the host lacks them or has older ones.
set -euo pipefail

BUILD_ID="__KINDTKS_BUILD_ID__"
DATA_DIR="$HOME/.local/share/kindtks"
DEST="$HOME/.local/bin"
TTY="${KINDTKS_INSTALLER_TTY:-/dev/tty}"
YES=0
TOOLS=1

usage() {
  echo "usage: docker run --rm <kindtks image> install-script | bash -s -- [--yes] [--no-tools] [--dest DIR]"
}

die() {
  echo "error: $*" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --yes) YES=1 ;;
    --no-tools) TOOLS=0 ;;
    --dest)
      [ $# -ge 2 ] || { usage >&2; exit 2; }
      DEST="$2"
      shift
      ;;
    -h | --help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done
DEST="${DEST%/}"

# 1. Docker must work for this user.
if ! err=$(docker info 2>&1 >/dev/null); then
  die "docker is not usable as $(id -un): $err"
fi

image=$(docker images -q --filter "label=io.kindtks.build=$BUILD_ID" | head -n 1)
[ -n "$image" ] || die "kindtks image with build $BUILD_ID not found locally"
cid=$(docker create "$image")
trap 'docker rm "$cid" >/dev/null 2>&1 || true' EXIT

copy_out() { # copy_out PATH_IN_IMAGE HOST_PATH
  local out
  out=$(docker cp "$cid:$1" "$2" 2>&1) || die "copying $1 from the image: $out"
}

# 2. kindtks, and the private tools, profiles and charts it runs with.
mkdir -p "$DATA_DIR" "$DEST"
rm -rf "$DATA_DIR/bin.new" "$DATA_DIR/profiles.new" "$DATA_DIR/charts.new"
mkdir "$DATA_DIR/bin.new"
for tool in kind kubectl helm; do
  copy_out "/usr/local/bin/$tool" "$DATA_DIR/bin.new/$tool"
done
copy_out /profiles "$DATA_DIR/profiles.new"
copy_out /charts "$DATA_DIR/charts.new"
for dir in bin profiles charts; do
  rm -rf "${DATA_DIR:?}/$dir"
  mv "$DATA_DIR/$dir.new" "$DATA_DIR/$dir"
done
copy_out /usr/local/bin/kindtks "$DEST/kindtks"

installed=("kindtks -> $DEST/kindtks")
skipped=()
need_yes=0

version_of() { # version_of TOOL PATH: prints vX.Y.Z, or nothing
  local args
  case "$1" in
    kind) args=(version) ;;
    kubectl) args=(version --client) ;;
    helm) args=(version --short) ;;
  esac
  "$2" "${args[@]}" 2>/dev/null | grep -o 'v[0-9]\+\.[0-9]\+\.[0-9]\+' | head -n 1 || true
}

older() { # older A B: version A is older than version B
  [ "${1#v}" != "${2#v}" ] &&
    [ "$(printf '%s\n%s\n' "${1#v}" "${2#v}" | sort -V | head -n 1)" = "${1#v}" ]
}

path_index() { # path_index DIR: position of DIR on PATH, or -1
  local i=0 entry IFS=:
  for entry in $PATH; do
    if [ "${entry%/}" = "$1" ]; then
      echo "$i"
      return
    fi
    i=$((i + 1))
  done
  echo -1
}

tty_state=unknown
confirm() { # confirm QUESTION: succeeds when the answer is yes
  local answer
  if [ "$YES" = 1 ]; then
    echo "$1 [Y/n] yes (--yes)"
    return 0
  fi
  if [ "$tty_state" = unknown ]; then
    if { exec 3<"$TTY"; } 2>/dev/null; then tty_state=open; else tty_state=none; fi
  fi
  if [ "$tty_state" = none ]; then
    echo "$1 [Y/n] skipped: no terminal"
    need_yes=1
    return 1
  fi
  printf '%s [Y/n] ' "$1"
  read -r answer <&3 || answer=n
  [ -t 3 ] || echo "$answer"
  case "$answer" in
    "" | y | Y | yes | Yes | YES) return 0 ;;
    *) return 1 ;;
  esac
}

# 3. kind, kubectl and helm for using clusters yourself.
if [ "$TOOLS" = 1 ]; then
  dest_index=$(path_index "$DEST")
  for tool in kind kubectl helm; do
    bundled=$(version_of "$tool" "$DATA_DIR/bin/$tool")
    current=$(command -v "$tool" || true)
    if [ -z "$current" ]; then
      question="Install $tool $bundled to $DEST?"
    else
      have=$(version_of "$tool" "$current")
      if [ -z "$have" ]; then
        echo "$tool found at $current, version unknown (bundled $bundled)"
        continue
      fi
      if ! older "$have" "$bundled"; then
        echo "$tool $have found at $current (bundled $bundled)"
        continue
      fi
      question="$tool $have at $current is older than bundled $bundled. Install $bundled to $DEST"
      current_dir=$(dirname "$current")
      if [ "$current_dir" = "$DEST" ]; then
        question="$question?"
      elif [ "$dest_index" = -1 ]; then
        question="$question ($DEST is not on PATH yet)?"
      elif [ "$dest_index" -lt "$(path_index "$current_dir")" ]; then
        question="$question ($DEST comes first on PATH)?"
      else
        question="$question ($DEST comes after $current_dir on PATH; the new copy will not take effect until PATH is changed)?"
      fi
    fi
    if confirm "$question"; then
      cp "$DATA_DIR/bin/$tool" "$DEST/$tool"
      installed+=("$tool $bundled -> $DEST/$tool")
    else
      skipped+=("$tool")
    fi
  done
fi

# 4 and 5. PATH and summary.
echo
echo "Installed:"
printf '  %s\n' "${installed[@]}"
echo "  kind, kubectl, helm, profiles, charts -> $DATA_DIR (used by kindtks)"
if [ ${#skipped[@]} -gt 0 ]; then
  echo "Skipped: ${skipped[*]}"
  if [ "$need_yes" = 1 ]; then
    echo "  No terminal for prompts; to accept them, rerun with: ... install-script | bash -s -- --yes"
  fi
fi
if [ "$(path_index "$DEST")" = -1 ]; then
  echo
  echo "$DEST is not on PATH. Add this line to your shell profile:"
  printf '  export PATH="%s:$PATH"\n' "$DEST"
fi
echo
echo "Get started: kindtks list"
```

- [ ] **Step 5: Write the command** — `internal/cmd/installscript.go`:

```go
package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed installer.sh
var installerScript string

var buildIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var installScriptCmd = &cobra.Command{
	Use:          "install-script",
	Short:        "Print the installer for this kindtks image (pipe it to bash)",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := os.Getenv("KINDTKS_BUILD_ID")
		switch {
		case id == "":
			return errors.New("KINDTKS_BUILD_ID is not set; run this from the kindtks image: docker run --rm <image> install-script | bash")
		case id == "dev":
			return errors.New("this image was built without a build ID; rebuild it with --build-arg BUILD_ID=<unique id>")
		case !buildIDPattern.MatchString(id):
			return fmt.Errorf("invalid build ID %q", id)
		}
		fmt.Fprint(cmd.OutOrStdout(), strings.ReplaceAll(installerScript, "__KINDTKS_BUILD_ID__", id))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(installScriptCmd)
}
```

- [ ] **Step 6: Run tests**

Run: `go vet ./... && go test ./... && bats test/unit/`
Expected: PASS (all installer tests, plus existing unit tests).

- [ ] **Step 7: Commit**

```bash
git add internal/cmd/installer.sh internal/cmd/installscript.go internal/cmd/installscript_test.go test/unit/installer.bats
git commit -m "feat: add install-script, a one-line installer found by build label"
```

---

### Task 4: Scratch image with build ID, release workflow, installer e2e

**Files:**
- Modify: `Dockerfile`, `.github/workflows/release.yml`
- Create: `test/e2e/installer.bats`

**Interfaces:**
- Consumes: `kindtks install-script` (Task 3), data dir layout (Task 1).
- Produces: image with `/usr/local/bin/{kindtks,kind,kubectl,helm}`, `/profiles`, `/charts`, `LABEL io.kindtks.build`, `ENV KINDTKS_BUILD_ID`.

- [ ] **Step 1: Dockerfile**
  - Delete the `GETTEXT_APK` arg and its comment line if only it uses it (keep the comment about apk pins: it still applies to `BASH_APK`).
  - In the `tools` stage, delete the `DOCKER_VERSION`/`DOCKER_SHA256` args and the four docker lines (`wget … docker.tgz`, its `sha256sum -c`, `tar xzf docker.tgz docker/docker`, `install … /out/docker`), ending the `RUN` after `install -m 0755 linux-amd64/helm /out/helm`.
  - Replace the whole final stage (from the last `FROM --platform=linux/amd64 ${ALPINE_IMAGE}` to the end) with:

```dockerfile
FROM --platform=linux/amd64 scratch
COPY --from=tools /out/ /usr/local/bin/
COPY --from=builder /kindtks /usr/local/bin/kindtks
COPY profiles/ /profiles/
COPY --from=charts /charts/ /charts/
# Unique per build: the installer finds this image on the host by the label.
ARG BUILD_ID=dev
LABEL io.kindtks.build=${BUILD_ID}
ENV KINDTKS_BUILD_ID=${BUILD_ID}
ENTRYPOINT ["/usr/local/bin/kindtks"]
```

- [ ] **Step 2: Build and check**

Run:
```bash
docker build --build-arg BUILD_ID=check-$(date +%s) -t kindtks:dev .
docker run --rm kindtks:dev | head -3
docker run --rm kindtks:dev install-script | head -12 | grep BUILD_ID=
docker inspect kindtks:dev --format '{{index .Config.Labels "io.kindtks.build"}} {{.Os}}/{{.Architecture}}'
docker build -t kindtks:nobuildid . >/dev/null && docker run --rm kindtks:nobuildid install-script; echo "exit $?"; docker rmi kindtks:nobuildid >/dev/null
```
Expected: README head printed; `BUILD_ID="check-<n>"`; label `check-<n> linux/amd64`; the no-build-ID image prints the "built without a build ID" error with `exit 1`.

- [ ] **Step 3: Release workflow** — in `.github/workflows/release.yml`, `publish` job:
  - Before the `docker/build-push-action` step, add:

```yaml
      - id: buildid
        run: echo "id=${GITHUB_REF_NAME}-${GITHUB_SHA::7}-$(date -u +%Y%m%dT%H%M%SZ)" >> "$GITHUB_OUTPUT"
```

  - In that build step, replace `build-args: VERSION=${{ github.ref_name }}` with:

```yaml
          build-args: |
            VERSION=${{ github.ref_name }}
            BUILD_ID=${{ steps.buildid.outputs.id }}
```

  - Replace the `NOTES=$(printf …)` line with:

```bash
          NOTES=$(printf '## Install\n\n```bash\ndocker run --rm %s install-script | bash\n```\n\nRequires Docker and bash. Add `| bash -s -- --yes` to accept all prompts.\n\n## Container Image\n\n```\ndocker pull %s\n```\n\nDigest: `%s`' "$IMAGE_REF" "$IMAGE_REF" "$DIGEST")
```

- [ ] **Step 4: E2e test** — `test/e2e/installer.bats`:

```bash
#!/usr/bin/env bats
# The documented one-liner against a real image pulled under another name
# from a temporary registry, installing into a temporary HOME.

load '../helpers/test_helper'

REGISTRY_NAME="kindtks-installer-e2e-reg"

setup_file() {
  export BUILD_ID="e2e-$(date +%s)-$$"
  export SRC_IMAGE="kindtks:installer-e2e"
  docker build --build-arg BUILD_ID="$BUILD_ID" -t "$SRC_IMAGE" "${BATS_TEST_DIRNAME}/../../"

  docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
  docker run -d --name "$REGISTRY_NAME" -p 127.0.0.1::5000 registry:2 >/dev/null
  local port
  port=$(docker port "$REGISTRY_NAME" 5000/tcp | head -n 1 | cut -d: -f2)
  for _ in $(seq 1 30); do
    curl -fs "http://127.0.0.1:${port}/v2/" >/dev/null && break
    sleep 1
  done
  export CORP_IMAGE="localhost:${port}/corp/kindtks:e2e"
  docker tag "$SRC_IMAGE" "$CORP_IMAGE"
  docker push -q "$CORP_IMAGE"
  docker rmi "$SRC_IMAGE" "$CORP_IMAGE" >/dev/null

  export H="${BATS_FILE_TMPDIR}/home"
  mkdir -p "$H"
}

teardown_file() {
  docker rm -f "$REGISTRY_NAME" >/dev/null 2>&1 || true
  docker rmi -f "$CORP_IMAGE" "$SRC_IMAGE" >/dev/null 2>&1 || true
}

@test "no local copy of the image before installing" {
  run docker images -q --filter "label=io.kindtks.build=$BUILD_ID"
  assert_output ""
}

@test "one-liner installs from the registry name" {
  run bash -c "docker run --rm '$CORP_IMAGE' install-script | HOME='$H' bash -s -- --yes"
  assert_success
  assert_output --partial "Get started: kindtks list"
}

@test "installed files belong to the user" {
  for f in "$H/.local/bin/kindtks" "$H/.local/share/kindtks/bin/kind" \
    "$H/.local/share/kindtks/bin/kubectl" "$H/.local/share/kindtks/bin/helm"; do
    [ -x "$f" ]
    [ "$(stat -c %u "$f")" = "$(id -u)" ]
  done
  [ "$(stat -c %u "$H/.local/share/kindtks/profiles/gen1/install.sh")" = "$(id -u)" ]
}

@test "installed kindtks lists gen1 from the installed profiles" {
  run env -u KINDTKS_PROFILES_DIR HOME="$H" "$H/.local/bin/kindtks" list
  assert_success
  assert_output --partial "gen1"
}

@test "bundled tools are the pinned versions" {
  run "$H/.local/share/kindtks/bin/kind" version
  assert_output --partial "v0.33.0"
  run "$H/.local/share/kindtks/bin/kubectl" version --client
  assert_output --partial "v1.33.1"
  run "$H/.local/share/kindtks/bin/helm" version --short
  assert_output --partial "v3.21.0"
}

@test "installed charts are present" {
  run ls "$H/.local/share/kindtks/charts"
  assert_output --partial "cilium-1.13.10.tgz"
}
```

- [ ] **Step 5: Run**

Run: `bats test/e2e/installer.bats` (foreground, maximum timeout).
Expected: 6/6 pass; afterwards `docker ps -a | grep installer-e2e` prints nothing.

- [ ] **Step 6: Commit**

```bash
git add Dockerfile .github/workflows/release.yml test/e2e/installer.bats
git commit -m "build: scratch image with build-ID label for the installer"
```

---

### Task 5: Docs and air-gap tests

**Files:**
- Modify: `README.md`, `profiles/gen1/README.md`, `test/e2e/airgap-gen1.bats`, `test/llm/airgap-deploy.bats`

- [ ] **Step 1: README** — replace everything from `## Run` up to (not including) `## Usage` with:

````markdown
## Install

Requires Docker and bash (Linux or WSL2).

```bash
docker run --rm ghcr.io/null-ptr-exception/kindtks:latest install-script | bash
```

The installer copies, out of the image:

- `kindtks` to `~/.local/bin`;
- the pinned `kind`, `kubectl` and `helm`, the profiles and the Helm charts to `~/.local/share/kindtks`. kindtks always runs profiles with these.

It then checks `kind`, `kubectl` and `helm` on your `PATH` and offers to install the bundled version to `~/.local/bin` where one is missing or older. Nothing outside that directory is changed.

Options, passed as `| bash -s -- <options>`:

| Option | Effect |
|---|---|
| `--yes` | answer yes to every prompt (prompts need a terminal; without one they are skipped) |
| `--no-tools` | skip the `PATH` check; install only kindtks and its private copies |
| `--dest DIR` | install to `DIR` instead of `~/.local/bin` |

Upgrade: run the same command with a newer image.

Air-gapped hosts: push the image to your registry and run the same command with that reference. The installer finds the pulled image by its build label, whatever its name.

Bundled versions: kind v0.33.0, kubectl v1.33.1, helm v3.21.0. kubectl 1.33 is outside Kubernetes' version-skew policy for gen1's Kubernetes 1.24; kindtks' end-to-end tests cover this combination.

Upgrading from 0.5.x: `kindtks install` is gone; run the installer above.
````

- [ ] **Step 2: gen1 README** — delete the line ``\`kindtks\` below is short for the `docker run` command in the [top-level README](../../README.md#run).`` (and the blank line after it).

- [ ] **Step 3: airgap-gen1.bats**
  - In `@test "build kindtks image"`, change the build to `docker build --build-arg BUILD_ID="airgap-$(date +%s)" -t kindtks:airgap-test "${BATS_TEST_DIRNAME}/../../"`.
  - In `@test "clean VM state"`, replace `ssh_vm 'rm -rf ~/.local/state/kindtks/gen1'` with `ssh_vm 'rm -rf ~/.local/bin/kindtks ~/.local/share/kindtks ~/.local/state/kindtks/gen1'`.
  - Replace `@test "pull kindtks image in VM"` with:

```bash
@test "install kindtks in VM" {
  run ssh_vm "docker run --rm ${VM_REGISTRY}/kindtks:airgap-test install-script | bash -s -- --yes"
  assert_success
  assert_output --partial "Get started: kindtks list"
}
```

  - In `@test "create gen1 cluster with registry auth"`, replace everything from the line `mkdir -p ~/.kube ~/.local/state/kindtks` through the `${VM_REGISTRY}/kindtks:airgap-test create gen1 --config airgap-config.yaml` line with:

```bash
kindtks create gen1 --config ~/airgap-config.yaml
```

  (the heredoc stays `<<SSH` with its escaped `\$` uses; this line has no `$`).

- [ ] **Step 4: test/llm/airgap-deploy.bats**
  - In `setup_file`, replace `ssh_vm 'rm -rf ~/.local/state/kindtks/gen1'` with `ssh_vm 'rm -rf ~/.local/bin/kindtks ~/.local/share/kindtks ~/.local/state/kindtks/gen1'`.
  - In the prompt, replace `2. Run the tool from the container image as its help output describes` with `2. Install the tool from the container image as its help output describes`.

- [ ] **Step 5: Verify**

Run:
```bash
go test ./... && bats test/unit/
AIRGAP_LAB_DIR=x AIRGAP_VM_IP=x AIRGAP_REGISTRY_USER=x AIRGAP_REGISTRY_PASS=x bats --count test/e2e/airgap-gen1.bats test/llm/airgap-deploy.bats
grep -rn 'docker-run\|KINDTKS_IMAGE\|--network host' README.md profiles .github test || echo clean
```
Expected: tests pass; a test count is printed (no parse error); `clean`.

- [ ] **Step 6: Commit**

```bash
git add README.md profiles/gen1/README.md test/e2e/airgap-gen1.bats test/llm/airgap-deploy.bats
git commit -m "docs: document the one-line installer"
```

---

### Controller verification after Task 5

- `bats test/e2e/installer.bats`, `bats test/e2e/registry-hosts.bats`, `bats test/unit/`, `go test ./...`.
- `gen1.bats` only if port 30080/30443 are free (infra's gen1 uses them); otherwise ask the user.
- `airgap-gen1.bats` when the lab VM is up.
- Deviation from the spec's testing section, on purpose: `gen1.bats` and `registry-hosts.bats` keep pointing at the repo's profiles and charts through `KINDTKS_PROFILES_DIR`/`KINDTKS_CHARTS_DIR` (still supported) instead of an installed data dir; the installed layout is covered by `installer.bats` (`kindtks list` from the installed profiles) and the Go unit tests of the default dirs.
