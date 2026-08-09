# kindtks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI that bootstraps customized Kind clusters via shell-script profiles, distributed as a Docker image.

**Architecture:** Thin Go CLI using cobra for commands. Profile scripts are standalone bash files discovered from `~/.local/share/kindtks/profiles/`. The CLI parses REQUIRES from scripts, checks prerequisites, then exec's bash to run create/delete functions. Docker image bundles binary + profiles and handles install.

**Tech Stack:** Go 1.24, cobra (CLI framework), Docker

**Prerequisites:** Go is available via `mise exec go@1.24 -- <command>`. A `mise.toml` with `go = "1.24"` exists in the project root. Run all Go commands through mise: `eval "$(~/.local/bin/mise activate bash)" && go <command>`.

---

## File Structure

```
kindtks/
├── mise.toml                      # pins Go 1.24
├── go.mod                         # module: github.com/rophy/kindtks
├── go.sum
├── main.go                        # entrypoint, wires cobra commands
├── internal/
│   ├── cmd/
│   │   ├── root.go                # root cobra command, no-args prints help
│   │   ├── create.go              # `kindtks create <profile>`
│   │   ├── delete.go              # `kindtks delete <profile>`
│   │   ├── list.go                # `kindtks list`
│   │   └── install.go             # `kindtks install` (Docker context only)
│   ├── profile/
│   │   ├── profile.go             # Profile struct, Load(), ListAll()
│   │   └── profile_test.go        # tests for profile loading/parsing
│   └── prereq/
│       ├── prereq.go              # Check() - verifies tools on PATH
│       └── prereq_test.go         # tests for prereq checking
├── profiles/
│   └── example.sh                 # example profile script
├── Dockerfile
└── docs/
```

---

### Task 1: Go module and CLI skeleton

**Files:**
- Create: `go.mod`, `go.sum`, `main.go`
- Create: `internal/cmd/root.go`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go mod init github.com/rophy/kindtks
```

- [ ] **Step 2: Add cobra dependency**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go get github.com/spf13/cobra@latest
```

- [ ] **Step 3: Create root command**

Create `internal/cmd/root.go`:
```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "kindtks",
	Short: "Bootstrap customized Kind clusters with pre-defined profiles",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("kindtks - Bootstrap customized Kind clusters")
		fmt.Println()
		fmt.Println("Quick start:")
		fmt.Println("  kindtks list              List available profiles")
		fmt.Println("  kindtks create <profile>  Create cluster(s) from a profile")
		fmt.Println("  kindtks delete <profile>  Delete cluster(s) from a profile")
		fmt.Println()
		fmt.Println("Install instructions:")
		fmt.Println("  docker run --rm \\")
		fmt.Println("    -v ~/.local/bin:/.local/bin \\")
		fmt.Println("    -v ~/.local/share/kindtks:/.local/share/kindtks \\")
		fmt.Println("    kindtks install")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Create main.go**

Create `main.go`:
```go
package main

import "github.com/rophy/kindtks/internal/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 5: Verify it builds and runs**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go build -o kindtks .
./kindtks
```

Expected: prints the help/install instructions text.

- [ ] **Step 6: Commit**

```bash
git add main.go go.mod go.sum internal/cmd/root.go mise.toml
git commit -m "feat: add Go CLI skeleton with cobra root command"
```

---

### Task 2: Profile loading and listing

**Files:**
- Create: `internal/profile/profile.go`
- Create: `internal/profile/profile_test.go`
- Create: `internal/cmd/list.go`
- Create: `profiles/example.sh`

- [ ] **Step 1: Create the example profile script**

Create `profiles/example.sh`:
```bash
REQUIRES="kind kubectl"

create() {
  kind create cluster --name example --image kindest/node:v1.31.0
  echo "Example cluster created."
}

delete() {
  kind delete cluster --name example
  echo "Example cluster deleted."
}
```

- [ ] **Step 2: Write failing tests for profile loading**

Create `internal/profile/profile_test.go`:
```go
package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	script := `REQUIRES="kind helm kubectl"

create() {
  echo "creating"
}

delete() {
  echo "deleting"
}
`
	err := os.WriteFile(filepath.Join(dir, "testprofile.sh"), []byte(script), 0644)
	if err != nil {
		t.Fatal(err)
	}

	p, err := Load(dir, "testprofile")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if p.Name != "testprofile" {
		t.Errorf("expected name 'testprofile', got %q", p.Name)
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

func TestListAll(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "alpha.sh"), []byte(`REQUIRES="kind"`), 0644)
	os.WriteFile(filepath.Join(dir, "beta.sh"), []byte(`REQUIRES="kind"`), 0644)
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

- [ ] **Step 3: Run tests to verify they fail**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go test ./internal/profile/ -v
```

Expected: compilation errors (Load, ListAll, Profile not defined).

- [ ] **Step 4: Implement profile package**

Create `internal/profile/profile.go`:
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
	Path     string
	Requires []string
}

func Load(profileDir string, name string) (*Profile, error) {
	path := filepath.Join(profileDir, name+".sh")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("profile %q not found: %w", name, err)
	}

	requires, err := parseRequires(path)
	if err != nil {
		return nil, fmt.Errorf("parsing profile %q: %w", name, err)
	}

	return &Profile{
		Name:     name,
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".sh"))
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

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go test ./internal/profile/ -v
```

Expected: all 3 tests pass.

- [ ] **Step 6: Create the list command**

Create `internal/cmd/list.go`:
```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rophy/kindtks/internal/profile"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		profileDir := profileDir()
		names, err := profile.ListAll(profileDir)
		if err != nil {
			return fmt.Errorf("listing profiles: %w", err)
		}
		if len(names) == 0 {
			fmt.Println("No profiles found in", profileDir)
			return nil
		}
		for _, name := range names {
			p, err := profile.Load(profileDir, name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				continue
			}
			fmt.Printf("%-20s requires: %s\n", p.Name, joinOrNone(p.Requires))
		}
		return nil
	},
}

func profileDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "kindtks", "profiles")
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	result := ""
	for i, item := range items {
		if i > 0 {
			result += ", "
		}
		result += item
	}
	return result
}

func init() {
	rootCmd.AddCommand(listCmd)
}
```

- [ ] **Step 7: Verify it builds**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go build -o kindtks .
./kindtks list
```

Expected: prints "No profiles found" (since `~/.local/share/kindtks/profiles` doesn't exist yet).

- [ ] **Step 8: Commit**

```bash
git add internal/profile/ internal/cmd/list.go profiles/example.sh
git commit -m "feat: add profile loading, listing, and example profile"
```

---

### Task 3: Prerequisite checking

**Files:**
- Create: `internal/prereq/prereq.go`
- Create: `internal/prereq/prereq_test.go`

- [ ] **Step 1: Write failing tests for prerequisite checking**

Create `internal/prereq/prereq_test.go`:
```go
package prereq

import (
	"testing"
)

func TestCheckAllPresent(t *testing.T) {
	// "sh" and "echo" should be on PATH in any test environment
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go test ./internal/prereq/ -v
```

Expected: compilation error (Check not defined).

- [ ] **Step 3: Implement prereq package**

Create `internal/prereq/prereq.go`:
```go
package prereq

import "os/exec"

func Check(tools []string) []string {
	var missing []string
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	return missing
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go test ./internal/prereq/ -v
```

Expected: all 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/prereq/
git commit -m "feat: add prerequisite checking"
```

---

### Task 4: Create command

**Files:**
- Create: `internal/cmd/create.go`

- [ ] **Step 1: Implement the create command**

Create `internal/cmd/create.go`:
```go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	c.Env = append(os.Environ(),
		"KINDTKS_DATA_DIR="+dataDir(),
		"KINDTKS_PROFILE_NAME="+p.Name,
	)
	return c.Run()
}

func dataDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.local/share/kindtks"
}

func init() {
	rootCmd.AddCommand(createCmd)
}
```

- [ ] **Step 2: Verify it builds**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go build -o kindtks .
./kindtks create nonexistent
```

Expected: error message about profile not found.

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/create.go
git commit -m "feat: add create command with prereq checking and profile execution"
```

---

### Task 5: Delete command

**Files:**
- Create: `internal/cmd/delete.go`

- [ ] **Step 1: Implement the delete command**

Create `internal/cmd/delete.go`:
```go
package cmd

import (
	"fmt"
	"strings"

	"github.com/rophy/kindtks/internal/prereq"
	"github.com/rophy/kindtks/internal/profile"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <profile>",
	Short: "Delete cluster(s) created by a profile",
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

		fmt.Printf("Deleting cluster(s) from profile %q...\n", name)
		return runProfileFunc(p, "delete")
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
```

- [ ] **Step 2: Verify it builds**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go build -o kindtks .
./kindtks delete --help
```

Expected: shows help for the delete command.

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/delete.go
git commit -m "feat: add delete command"
```

---

### Task 6: Install command

**Files:**
- Create: `internal/cmd/install.go`

- [ ] **Step 1: Implement the install command**

Create `internal/cmd/install.go`:
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
	hostBinDir          = "/.local/bin"
	hostProfileDir      = "/.local/share/kindtks/profiles"
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

		if err := os.MkdirAll(hostProfileDir, 0755); err != nil {
			return fmt.Errorf("creating profile directory: %w", err)
		}

		entries, err := os.ReadDir(containerProfileDir)
		if err != nil {
			return fmt.Errorf("reading profiles from %s: %w", containerProfileDir, err)
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			src := filepath.Join(containerProfileDir, e.Name())
			dst := filepath.Join(hostProfileDir, e.Name())
			fmt.Printf("Copying profile %s...\n", e.Name())
			if err := copyFile(src, dst, 0644); err != nil {
				return fmt.Errorf("copying profile %s: %w", e.Name(), err)
			}
		}

		fmt.Println()
		fmt.Println("Installation complete!")
		fmt.Println("Run 'kindtks list' to see available profiles.")
		return nil
	},
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

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

- [ ] **Step 2: Verify it builds**

Run:
```bash
eval "$(~/.local/bin/mise activate bash)"
cd /home/rophy/projects/kindtks
go build -o kindtks .
./kindtks install --help
```

Expected: shows help for the install command.

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/install.go
git commit -m "feat: add install command for Docker-based distribution"
```

---

### Task 7: Dockerfile

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Create the Dockerfile**

Create `Dockerfile`:
```dockerfile
FROM golang:1.24 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /kindtks .

FROM alpine:3.20

COPY --from=builder /kindtks /usr/local/bin/kindtks
COPY profiles/ /profiles/

ENTRYPOINT ["kindtks"]
```

- [ ] **Step 2: Build the image**

Run:
```bash
cd /home/rophy/projects/kindtks
docker build -t kindtks .
```

Expected: image builds successfully.

- [ ] **Step 3: Test no-args output**

Run:
```bash
docker run --rm kindtks
```

Expected: prints the help/install instructions.

- [ ] **Step 4: Test list command**

Run:
```bash
docker run --rm kindtks list
```

Expected: error about profile directory not found (expected — profiles are at `/profiles/` inside the container but the `list` command looks at `~/.local/share/kindtks/profiles/`). This is fine — `list` is meant to run on the host after install.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile
git commit -m "build: add multi-stage Dockerfile"
```

---

### Task 8: End-to-end test with example profile

**Files:**
- No new files — validates the full flow works.

- [ ] **Step 1: Install from Docker image to a test directory**

Run:
```bash
mkdir -p /tmp/kindtks-test/bin /tmp/kindtks-test/share/kindtks
docker run --rm \
  -v /tmp/kindtks-test/bin:/.local/bin \
  -v /tmp/kindtks-test/share/kindtks:/.local/share/kindtks \
  kindtks install
```

Expected: copies binary and profiles.

- [ ] **Step 2: Verify installed binary works**

Run:
```bash
/tmp/kindtks-test/bin/kindtks list
```

This will look at `~/.local/share/kindtks/profiles/` (the user's home), not the test dir. To test properly:

Run:
```bash
HOME=/tmp/kindtks-test/share/kindtks/.. /tmp/kindtks-test/bin/kindtks list
```

Expected: shows `example` profile with its requirements.

- [ ] **Step 3: Test create with missing prerequisites**

Run:
```bash
HOME=/tmp/kindtks-test/share/kindtks/.. /tmp/kindtks-test/bin/kindtks create example
```

Expected: if `kind` is not on PATH, prints error listing missing tools. If `kind` is available, creates the cluster.

- [ ] **Step 4: Clean up**

Run:
```bash
rm -rf /tmp/kindtks-test
```

- [ ] **Step 5: Commit (if any fixes were needed)**

```bash
git add -A
git commit -m "fix: adjustments from end-to-end testing"
```
