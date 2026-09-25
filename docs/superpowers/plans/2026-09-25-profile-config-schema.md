# Profile-specific Config with Two-layer JSON Schema — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure kindtks config into a common section plus per-profile `profiles.<name>` sections, validated by an embedded common JSON Schema and a per-profile `config.schema.json`, expose profile options to scripts through a hidden `kindtks value` command, and add gen1's `gateway.extraHosts`.

**Architecture:** `internal/config` loads YAML into generic JSON values, deep-merges profile defaults with the user file, validates with `santhosh-tekuri/jsonschema/v6`, and derives the typed `Config` (registries + active profile images) that the 0.4.0 registry code already consumes. `internal/cmd` writes the merged profile section to a temp JSON file for the script and serves `kindtks value <path>` from it. gen1 ships a schema and patches extra hosts onto its Gateway.

**Tech Stack:** Go 1.24, `gopkg.in/yaml.v3`, `github.com/santhosh-tekuri/jsonschema/v6`, `golang.org/x/text` (error messages), bash profiles, bats.

**Spec:** `docs/superpowers/specs/2026-09-25-profile-config-schema-design.md`

## Global Constraints

- Top level of a config document: only `registries` and `profiles`. Profile defaults file `profiles/<name>/config.yaml` holds the **body** of `profiles.<name>`.
- `images` is reserved inside every profile section: object of strings.
- Merge: objects merge key by key recursively; arrays and scalars from the higher layer replace.
- Error for a top-level `images`: must mention `profiles.<profile>.images`. Error for `registryAuth`: must mention `registries.<registry>.auth`.
- Validation error locations are rendered like `profiles.gen1.gateway.extraHosts[0]`.
- Sections for profiles not installed → warning `profiles.<name>: profile not installed, ignoring`, not an error.
- Script environment adds `KINDTKS_BIN` (absolute kindtks path) and `KINDTKS_PROFILE_CONFIG` (temp JSON of the merged profile section); `IMAGE_*` exports stay.
- `kindtks value <dot.path> [--required]`: hidden; scalar → one line; array of scalars → one line per element; missing/null → no output, exit 0 (non-zero with `--required`); object → error.
- gen1 `gateway.extraHosts` are added only to the HTTP (port 80) server of Gateway `istio-ingress/kindtks`.
- No new host tool dependencies (no `jq`).
- Public repo: docs/tests/examples use only `example.com` / `example.net`; never private hostnames.
- Commits: `<type>: <description>`, no AI attribution lines.
- Go via mise if `MISE_SHELL` is unset: `eval "$(~/.local/bin/mise activate bash)"`.

## File Structure

| File | Responsibility |
|---|---|
| `internal/config/document.go` (create) | `LoadDocument`, `normalize` |
| `internal/config/merge.go` (create) | `MergeValues` |
| `internal/config/schema.go` (create) | embedded common schema, `ProfileSchema`, `ValidateCommon`, `ValidateProfile`, `CombinedSchema`, error formatting |
| `internal/config/schema/common.json` (create) | common JSON Schema |
| `internal/config/resolve.go` (create) | `Resolved`, `Resolve`, `checkMovedKeys`, `typedConfig` |
| `internal/config/config.go` (modify) | types get JSON tags; `Load`/`Merge` removed |
| `internal/cmd/create.go`, `delete.go`, `config.go` (modify) | use `Resolve`; `config --schema` |
| `internal/cmd/value.go` (create) | hidden `value` command, `valueLines` |
| `internal/cmd/run.go` (create) | `runProfileFunc` moved here, writes profile JSON, sets env |
| `profiles/gen1/config.yaml`, `config.schema.json`, `install.sh` (modify/create) | gen1 options |
| `README.md`, `profiles/gen1/README.md`, `test/e2e/*.bats`, `test/unit/gen1_profile.bats` (modify) | docs & tests |

---

### Task 1: config — document loading, merge, and schema validation

**Files:**
- Create: `internal/config/document.go`, `internal/config/merge.go`, `internal/config/schema.go`, `internal/config/schema/common.json`
- Test: `internal/config/document_test.go`, `internal/config/merge_test.go`, `internal/config/schema_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Produces:
  - `func LoadDocument(path string) (map[string]any, error)`
  - `func MergeValues(base, override any) any`
  - `func ProfileSchema(profileDir string) ([]byte, error)`
  - `func ValidateCommon(doc any) error`
  - `func ValidateProfile(name string, schemaJSON []byte, section any) error`
  - `func CombinedSchema(name string, profileSchemaJSON []byte) (map[string]any, error)`

Nothing existing changes in this task; `Load`/`Merge` stay until Task 2.

- [ ] **Step 1: Add dependencies**

Run: `go get github.com/santhosh-tekuri/jsonschema/v6@latest golang.org/x/text@latest`
Expected: both appear in `go.mod`.

- [ ] **Step 2: Write failing tests**

`internal/config/document_test.go`:

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDocument_Missing(t *testing.T) {
	doc, err := LoadDocument(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || len(doc) != 0 {
		t.Fatalf("want empty doc, got %v, %v", doc, err)
	}
}

func TestLoadDocument_Empty(t *testing.T) {
	doc, err := LoadDocument(writeFile(t, "c.yaml", ""))
	if err != nil || len(doc) != 0 {
		t.Fatalf("want empty doc, got %v, %v", doc, err)
	}
}

func TestLoadDocument_NormalizesValues(t *testing.T) {
	doc, err := LoadDocument(writeFile(t, "c.yaml", "a:\n  n: 3\n  list: [x, true]\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := doc["a"].(map[string]any)
	if n, ok := a["n"].(json.Number); !ok || n.String() != "3" {
		t.Errorf("want json.Number 3, got %#v", a["n"])
	}
	list := a["list"].([]any)
	if list[0] != "x" || list[1] != true {
		t.Errorf("unexpected list %#v", list)
	}
}

func TestLoadDocument_RejectsNonMapping(t *testing.T) {
	_, err := LoadDocument(writeFile(t, "c.yaml", "- a\n- b\n"))
	if err == nil || !strings.Contains(err.Error(), "mapping") {
		t.Fatalf("want mapping error, got %v", err)
	}
}
```

`internal/config/merge_test.go`:

```go
package config

import (
	"reflect"
	"testing"
)

func TestMergeValues(t *testing.T) {
	base := map[string]any{
		"images":  map[string]any{"a": "base-a", "b": "base-b"},
		"gateway": map[string]any{"extraHosts": []any{"x"}},
		"keep":    "k",
	}
	override := map[string]any{
		"images":  map[string]any{"a": "over-a"},
		"gateway": map[string]any{"extraHosts": []any{"y", "z"}},
		"new":     true,
	}

	got := MergeValues(base, override)

	want := map[string]any{
		"images":  map[string]any{"a": "over-a", "b": "base-b"},
		"gateway": map[string]any{"extraHosts": []any{"y", "z"}},
		"keep":    "k",
		"new":     true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
	if base["images"].(map[string]any)["a"] != "base-a" {
		t.Error("MergeValues must not modify base")
	}
}

func TestMergeValues_ScalarReplacesObject(t *testing.T) {
	got := MergeValues(map[string]any{"a": map[string]any{"b": 1}}, map[string]any{"a": nil})
	if got.(map[string]any)["a"] != nil {
		t.Errorf("want nil, got %#v", got)
	}
}
```

`internal/config/schema_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testProfileSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "images": {"type": "object", "additionalProperties": false,
      "properties": {"cilium": {"type": "string"}}},
    "gateway": {"type": "object", "additionalProperties": false,
      "properties": {"extraHosts": {"type": "array", "items": {"type": "string", "pattern": "^[a-z*.]+$"}}}}
  }
}`

func obj(t *testing.T, yaml string) map[string]any {
	t.Helper()
	doc, err := LoadDocument(writeFile(t, "d.yaml", yaml))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestValidateCommon_Valid(t *testing.T) {
	doc := obj(t, `registries:
  quay.io:
    hosts: "server = 'x'"
    auth: {username: u, password: p}
profiles:
  gen1:
    images: {cilium: "quay.io/cilium/cilium:v1"}
    anything: [1, 2]
`)
	if err := ValidateCommon(doc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCommon_Errors(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"unknown top-level key": {"bogus: 1\n", "bogus"},
		"extra registry field":  {"registries:\n  quay.io:\n    mirror: x\n", "registries.quay.io"},
		"non-string image":      {"profiles:\n  gen1:\n    images: {cilium: 3}\n", "profiles.gen1.images.cilium"},
		"profile not an object": {"profiles:\n  gen1: [a]\n", "profiles.gen1"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateCommon(obj(t, c.yaml))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error mentioning %q, got %v", c.want, err)
			}
		})
	}
}

func TestValidateProfile_ReportsPath(t *testing.T) {
	section := obj(t, "gateway:\n  extraHosts: [\"BAD\"]\n")
	err := ValidateProfile("gen1", []byte(testProfileSchema), section)
	if err == nil || !strings.Contains(err.Error(), "profiles.gen1.gateway.extraHosts[0]") {
		t.Fatalf("want path profiles.gen1.gateway.extraHosts[0], got %v", err)
	}
}

func TestValidateProfile_UnknownOption(t *testing.T) {
	err := ValidateProfile("gen1", []byte(testProfileSchema), obj(t, "bogus: 1\n"))
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("want error mentioning bogus, got %v", err)
	}
}

func TestProfileSchema_DefaultAllowsOnlyImages(t *testing.T) {
	schema, err := ProfileSchema(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProfile("plain", schema, obj(t, "images: {x: \"img:1\"}\n")); err != nil {
		t.Errorf("images should be allowed: %v", err)
	}
	if err := ValidateProfile("plain", schema, obj(t, "gateway: {}\n")); err == nil {
		t.Error("non-images key should be rejected")
	}
}

func TestProfileSchema_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.schema.json"), []byte(testProfileSchema), 0644)
	schema, err := ProfileSchema(dir)
	if err != nil || string(schema) != testProfileSchema {
		t.Fatalf("want file contents, got %q, %v", schema, err)
	}
}

func TestCombinedSchema(t *testing.T) {
	combined, err := CombinedSchema("gen1", []byte(testProfileSchema))
	if err != nil {
		t.Fatal(err)
	}
	data, err := jsonMarshal(combined)
	if err != nil {
		t.Fatal(err)
	}
	ok := obj(t, "profiles:\n  gen1:\n    gateway: {extraHosts: [\"*.example.net\"]}\n")
	if err := validate("combined.json", data, ok, ""); err != nil {
		t.Errorf("valid doc rejected: %v", err)
	}
	bad := obj(t, "profiles:\n  gen1:\n    bogus: 1\n")
	if err := validate("combined.json", data, bad, ""); err == nil {
		t.Error("combined schema should apply the profile schema at profiles.gen1")
	}
}
```

- [ ] **Step 3: Run to verify failure**

Run: `go test ./internal/config/ -run 'LoadDocument|MergeValues|Validate|ProfileSchema|CombinedSchema' -v`
Expected: FAIL to compile (`undefined: LoadDocument`, `MergeValues`, ...).

- [ ] **Step 4: Implement**

`internal/config/document.go`:

```go
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// LoadDocument reads a YAML config file into JSON-compatible generic values
// (map[string]any, []any, string, bool, json.Number, nil). A missing or empty
// file yields an empty object.
func LoadDocument(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}

	var raw any
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if raw == nil {
		return map[string]any{}, nil
	}

	doc, err := normalize(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parsing %s: top level must be a mapping", path)
	}
	return obj, nil
}

// normalize converts decoded YAML into the JSON value model the schema
// validator expects by round-tripping it through JSON.
func normalize(v any) (any, error) {
	data, err := jsonMarshal(v)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(data))
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
```

`internal/config/merge.go`:

```go
package config

// MergeValues deep-merges override onto base: objects merge key by key,
// anything else in override replaces base. Inputs are not modified.
func MergeValues(base, override any) any {
	b, bok := base.(map[string]any)
	o, ook := override.(map[string]any)
	if !bok || !ook {
		return override
	}
	out := make(map[string]any, len(b)+len(o))
	for k, v := range b {
		out[k] = v
	}
	for k, v := range o {
		if bv, exists := out[k]; exists {
			out[k] = MergeValues(bv, v)
		} else {
			out[k] = v
		}
	}
	return out
}
```

`internal/config/schema/common.json`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "kindtks config",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "registries": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "hosts": {"type": "string"},
          "auth": {
            "type": "object",
            "additionalProperties": false,
            "required": ["username", "password"],
            "properties": {
              "username": {"type": "string"},
              "password": {"type": "string"}
            }
          }
        }
      }
    },
    "profiles": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "properties": {
          "images": {
            "type": "object",
            "additionalProperties": {"type": "string"}
          }
        }
      }
    }
  }
}
```

`internal/config/schema.go`:

```go
package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed schema/common.json
var commonSchemaJSON []byte

// defaultProfileSchemaJSON applies to profiles without a config.schema.json:
// only the reserved images map is allowed.
var defaultProfileSchemaJSON = []byte(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "images": {"type": "object", "additionalProperties": {"type": "string"}}
  }
}`)

// ProfileSchema returns the profile's config.schema.json, or the default
// images-only schema when the profile has none.
func ProfileSchema(profileDir string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(profileDir, "config.schema.json"))
	if errors.Is(err, os.ErrNotExist) {
		return defaultProfileSchemaJSON, nil
	}
	return data, err
}

// ValidateCommon validates a whole config document against the common schema.
func ValidateCommon(doc any) error {
	return validate("common.json", commonSchemaJSON, doc, "")
}

// ValidateProfile validates one profiles.<name> section against schemaJSON.
func ValidateProfile(name string, schemaJSON []byte, section any) error {
	return validate(name+".schema.json", schemaJSON, section, "profiles."+name)
}

// CombinedSchema returns the common schema with the profile's schema placed
// at profiles.<name>, for editor completion.
func CombinedSchema(name string, profileSchemaJSON []byte) (map[string]any, error) {
	var common, prof map[string]any
	if err := json.Unmarshal(commonSchemaJSON, &common); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(profileSchemaJSON, &prof); err != nil {
		return nil, fmt.Errorf("parsing %s schema: %w", name, err)
	}
	delete(prof, "$schema")

	profiles := common["properties"].(map[string]any)["profiles"].(map[string]any)
	props, _ := profiles["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		profiles["properties"] = props
	}
	props[name] = prof
	return common, nil
}

func validate(name string, schemaJSON []byte, inst any, prefix string) error {
	url := "mem://" + name
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return fmt.Errorf("parsing schema %s: %w", name, err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(url, doc); err != nil {
		return fmt.Errorf("loading schema %s: %w", name, err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return fmt.Errorf("compiling schema %s: %w", name, err)
	}

	err = sch.Validate(inst)
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	var msgs []string
	collectLeaves(ve, prefix, message.NewPrinter(language.English), &msgs)
	sort.Strings(msgs)
	return fmt.Errorf("invalid config:\n  %s", strings.Join(msgs, "\n  "))
}

// collectLeaves flattens a validation error tree into one message per leaf.
func collectLeaves(e *jsonschema.ValidationError, prefix string, p *message.Printer, out *[]string) {
	if len(e.Causes) == 0 {
		*out = append(*out, fmt.Sprintf("%s: %s", instancePath(prefix, e.InstanceLocation), e.ErrorKind.LocalizedString(p)))
		return
	}
	for _, c := range e.Causes {
		collectLeaves(c, prefix, p, out)
	}
}

// instancePath renders a location like profiles.gen1.gateway.extraHosts[0].
func instancePath(prefix string, loc []string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for _, seg := range loc {
		if isIndex(seg) {
			fmt.Fprintf(&b, "[%s]", seg)
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	if b.Len() == 0 {
		return "(root)"
	}
	return b.String()
}

func isIndex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
```

- [ ] **Step 5: Run to verify pass**

Run: `go mod tidy && go test ./internal/config/ -v`
Expected: PASS (new and existing tests).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config/
git commit -m "feat: add config document merge and JSON schema validation"
```

---

### Task 2: Resolve — layered config, typed view, and CLI wiring

**Files:**
- Create: `internal/config/resolve.go`, `internal/config/resolve_test.go`, `internal/cmd/run.go`
- Modify: `internal/config/config.go`, `internal/config/config_test.go`, `internal/cmd/create.go`, `internal/cmd/delete.go`, `internal/cmd/config.go`
- Delete: none

**Interfaces:**
- Consumes: Task 1 functions.
- Produces:
  - `type Resolved struct { Config *Config; Profile map[string]any; Warnings []string }`
  - `func Resolve(profilesDir, profile, userFile string) (*Resolved, error)`
  - `func runProfileFunc(p *profile.Profile, funcName string, res *config.Resolved, kindCfgPath string) error` (in `internal/cmd/run.go`)
  - `kindtks config <profile> [--schema]`

- [ ] **Step 1: Write failing tests**

`internal/config/resolve_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gen1Defaults = `images:
  cilium: quay.io/cilium/cilium:v1.13.10
gateway:
  extraHosts: []
`

func writeProfile(t *testing.T, profilesDir, name, defaults, schema string) {
	t.Helper()
	dir := filepath.Join(profilesDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "install.sh"), []byte("create() { :; }\ndelete() { :; }\n"), 0644)
	if defaults != "" {
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(defaults), 0644)
	}
	if schema != "" {
		os.WriteFile(filepath.Join(dir, "config.schema.json"), []byte(schema), 0644)
	}
}

func setupProfiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeProfile(t, dir, "gen1", gen1Defaults, testProfileSchema)
	return dir
}

func TestResolve_DefaultsOnly(t *testing.T) {
	res, err := Resolve(setupProfiles(t), "gen1", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.Images["cilium"] != "quay.io/cilium/cilium:v1.13.10" {
		t.Errorf("unexpected images %v", res.Config.Images)
	}
	if _, ok := res.Profile["gateway"]; !ok {
		t.Errorf("profile section should include gateway: %v", res.Profile)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("unexpected warnings %v", res.Warnings)
	}
}

func TestResolve_UserOverridesProfileSection(t *testing.T) {
	user := writeFile(t, "user.yaml", `profiles:
  gen1:
    images:
      cilium: registry.example.com/cilium:v1
    gateway:
      extraHosts: ["*.example.net"]
`)
	res, err := Resolve(setupProfiles(t), "gen1", user)
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.Images["cilium"] != "registry.example.com/cilium:v1" {
		t.Errorf("image not overridden: %v", res.Config.Images)
	}
	hosts := res.Profile["gateway"].(map[string]any)["extraHosts"].([]any)
	if len(hosts) != 1 || hosts[0] != "*.example.net" {
		t.Errorf("extraHosts not replaced: %v", hosts)
	}
}

func TestResolve_Registries(t *testing.T) {
	user := writeFile(t, "user.yaml", `registries:
  quay.io:
    hosts: |
      server = "https://quay.io"
  registry.example.com:
    auth: {username: u, password: p}
`)
	res, err := Resolve(setupProfiles(t), "gen1", user)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Config.Registries["quay.io"].Hosts, "https://quay.io") {
		t.Errorf("hosts not decoded: %+v", res.Config.Registries["quay.io"])
	}
	if a := res.Config.Registries["registry.example.com"].Auth; a == nil || a.Username != "u" || a.Password != "p" {
		t.Errorf("auth not decoded: %+v", a)
	}
}

func TestResolve_Errors(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"top-level images":        {"images: {cilium: x}\n", "profiles.<profile>.images"},
		"registryAuth":            {"registryAuth: {}\n", "registries.<registry>.auth"},
		"unknown top-level key":   {"bogus: 1\n", "bogus"},
		"unknown profile option":  {"profiles:\n  gen1:\n    bogus: 1\n", "profiles.gen1"},
		"bad extra host":          {"profiles:\n  gen1:\n    gateway: {extraHosts: [\"BAD\"]}\n", "profiles.gen1.gateway.extraHosts[0]"},
		"invalid hosts TOML":      {"registries:\n  quay.io:\n    hosts: \"server = \"\n", "quay.io"},
		"unsafe registry name":    {"registries:\n  a..b:\n    hosts: \"x = 1\"\n", "a..b"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve(setupProfiles(t), "gen1", writeFile(t, "user.yaml", c.yaml))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error mentioning %q, got %v", c.want, err)
			}
		})
	}
}

func TestResolve_InvalidDefaultsRejected(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "gen1", "bogus: 1\n", testProfileSchema)
	if _, err := Resolve(dir, "gen1", ""); err == nil {
		t.Fatal("defaults violating the profile schema must be rejected")
	}
}

func TestResolve_ProfileWithoutSchemaAcceptsOnlyImages(t *testing.T) {
	dir := t.TempDir()
	writeProfile(t, dir, "plain", "images:\n  x: img:1\n", "")
	if _, err := Resolve(dir, "plain", ""); err != nil {
		t.Fatalf("images-only defaults should be valid: %v", err)
	}
	user := writeFile(t, "user.yaml", "profiles:\n  plain:\n    gateway: {}\n")
	if _, err := Resolve(dir, "plain", user); err == nil {
		t.Fatal("non-images option should be rejected for a profile without schema")
	}
}

func TestResolve_OtherInstalledProfileValidated(t *testing.T) {
	dir := setupProfiles(t)
	writeProfile(t, dir, "other", "", "")
	user := writeFile(t, "user.yaml", "profiles:\n  other:\n    bogus: 1\n")
	_, err := Resolve(dir, "gen1", user)
	if err == nil || !strings.Contains(err.Error(), "profiles.other") {
		t.Fatalf("want error for profiles.other, got %v", err)
	}
}

func TestResolve_UninstalledProfileWarns(t *testing.T) {
	user := writeFile(t, "user.yaml", "profiles:\n  ghost:\n    anything: 1\n")
	res, err := Resolve(setupProfiles(t), "gen1", user)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "profiles.ghost: profile not installed") {
		t.Errorf("unexpected warnings %v", res.Warnings)
	}
}
```

In `internal/config/config_test.go`: delete every test that calls `Load` or `Merge` (`TestLoadExisting`, `TestLoadMissing`, `TestLoadEmpty`, `TestMerge`, `TestLoadRegistries`, `TestLoadRejectsUnknownKey`, `TestLoadRejectsInvalidHostsTOML`, `TestLoadRejectsBadRegistryName`, `TestMergeRegistriesPerField`, `TestMergeDoesNotModifyInputs`, `TestLoadRejectsRegistryAuth`); their behaviors are covered by `resolve_test.go`, `merge_test.go` and `schema_test.go`. If the file is left with no tests, delete it.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/ -run Resolve -v`
Expected: FAIL to compile (`undefined: Resolve`).

- [ ] **Step 3: Implement `Resolve` and update types**

`internal/config/config.go`: replace the type block and remove `Load` and `Merge` (keep `Validate` and `validateRegistryName`; drop now-unused imports `bytes`, `errors`, `io`, `os`, `yaml`):

```go
type RegistryAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Registry holds per-registry containerd settings.
type Registry struct {
	// Hosts is the raw content of certs.d/<registry>/hosts.toml.
	Hosts string        `json:"hosts,omitempty"`
	Auth  *RegistryAuth `json:"auth,omitempty"`
}

// Config is the typed view of the settings kindtks itself acts on: the common
// registries and the active profile's images.
type Config struct {
	Images     map[string]string
	Registries map[string]*Registry
}
```

`internal/config/resolve.go`:

```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Resolved is the effective configuration for one profile.
type Resolved struct {
	// Config is the typed view used by kindtks itself.
	Config *Config
	// Profile is the merged profiles.<name> section passed to the profile script.
	Profile map[string]any
	// Warnings are non-fatal issues, e.g. sections for profiles not installed.
	Warnings []string
}

// Resolve merges the profile's defaults with the optional user config file,
// validates the result against the common and profile schemas, and returns
// the effective configuration for profile. profilesDir holds the installed
// profiles, one directory each.
func Resolve(profilesDir, profile, userFile string) (*Resolved, error) {
	defaults, err := LoadDocument(filepath.Join(profilesDir, profile, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("loading profile defaults: %w", err)
	}
	merged := map[string]any{"profiles": map[string]any{profile: defaults}}

	if userFile != "" {
		user, err := LoadDocument(userFile)
		if err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
		if err := checkMovedKeys(user, userFile); err != nil {
			return nil, err
		}
		merged = MergeValues(merged, user).(map[string]any)
	}

	if err := ValidateCommon(merged); err != nil {
		return nil, err
	}

	res := &Resolved{}
	sections, _ := merged["profiles"].(map[string]any)
	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dir := filepath.Join(profilesDir, name)
		if _, err := os.Stat(filepath.Join(dir, "install.sh")); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("profiles.%s: profile not installed, ignoring", name))
			continue
		}
		schema, err := ProfileSchema(dir)
		if err != nil {
			return nil, fmt.Errorf("loading profiles.%s schema: %w", name, err)
		}
		if err := ValidateProfile(name, schema, sections[name]); err != nil {
			return nil, err
		}
	}

	res.Profile, _ = sections[profile].(map[string]any)
	if res.Profile == nil {
		res.Profile = map[string]any{}
	}
	cfg, err := typedConfig(merged, res.Profile)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	res.Config = cfg
	return res, nil
}

// checkMovedKeys gives migration hints for keys removed from the top level.
func checkMovedKeys(doc map[string]any, path string) error {
	if _, ok := doc["images"]; ok {
		return fmt.Errorf("%s: top-level `images` moved to `profiles.<profile>.images`; see README", path)
	}
	if _, ok := doc["registryAuth"]; ok {
		return fmt.Errorf("%s: `registryAuth` was replaced by `registries.<registry>.auth`; see README", path)
	}
	return nil
}

// typedConfig decodes the common registries and the active profile's images.
// The document has already passed schema validation.
func typedConfig(doc, section map[string]any) (*Config, error) {
	cfg := &Config{Images: map[string]string{}}
	if regs, ok := doc["registries"]; ok {
		data, err := json.Marshal(regs)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &cfg.Registries); err != nil {
			return nil, fmt.Errorf("decoding registries: %w", err)
		}
	}
	if imgs, ok := section["images"].(map[string]any); ok {
		for k, v := range imgs {
			s, _ := v.(string)
			cfg.Images[k] = s
		}
	}
	return cfg, nil
}
```

- [ ] **Step 4: Run config tests**

Run: `go test ./internal/config/ -v`
Expected: PASS. (`./internal/cmd` does not compile yet — next steps.)

- [ ] **Step 5: Move `runProfileFunc` and wire `create`/`delete`**

Create `internal/cmd/run.go` with the function moved out of `create.go`, taking the resolved config and kind config path:

```go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/profile"
)

// runProfileFunc sources the profile script and calls funcName with the
// kindtks environment.
func runProfileFunc(p *profile.Profile, funcName string, res *config.Resolved, kindCfgPath string) error {
	script := fmt.Sprintf("source %q && %s", p.Path, funcName)
	c := exec.Command("bash", "-e", "-c", script)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	env := append(os.Environ(),
		"KINDTKS_DATA_DIR="+dataDir(),
		"KINDTKS_CHARTS_DIR="+filepath.Join(dataDir(), "charts"),
		"KINDTKS_PROFILE_NAME="+p.Name,
		"KINDTKS_PROFILE_DIR="+p.Dir,
		"KINDTKS_KIND_CONFIG="+kindCfgPath,
	)
	for key, image := range res.Config.Images {
		envKey := "IMAGE_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
		env = append(env, envKey+"="+image)
	}

	c.Env = env
	return c.Run()
}
```

In `internal/cmd/create.go`: delete `loadProfileConfig` and the old `runProfileFunc`; replace the body after the prereq check with:

```go
		res, err := config.Resolve(dir, name, configFile)
		if err != nil {
			return err
		}
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}

		fmt.Printf("Creating cluster(s) from profile %q...\n", name)
		kindCfgPath, cleanup, err := prepareKindConfig(filepath.Join(p.Dir, "kind-config.yaml"), res.Config, profileStateDir(name))
		if err != nil {
			return err
		}
		defer cleanup()

		return runProfileFunc(p, "create", res, kindCfgPath)
```

Update imports (`os`, `path/filepath`, `config`; drop `os/exec` if unused).

In `internal/cmd/delete.go`, replace `if err := runProfileFunc(p, "delete", nil); err != nil {` with:

```go
		res, err := config.Resolve(dir, name, "")
		if err != nil {
			return err
		}
		if err := runProfileFunc(p, "delete", res, filepath.Join(p.Dir, "kind-config.yaml")); err != nil {
```

(add imports `path/filepath`, `config`).

- [ ] **Step 6: `kindtks config <profile> [--schema]`**

Replace `internal/cmd/config.go` with:

```go
package cmd

import (
	"encoding/json"
	"os"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/profile"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var showSchema bool

var configCmd = &cobra.Command{
	Use:   "config <profile>",
	Short: "Show default configuration (or its JSON schema) for a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		dir := profileDir()

		p, err := profile.Load(dir, name)
		if err != nil {
			return err
		}

		if showSchema {
			schema, err := config.ProfileSchema(p.Dir)
			if err != nil {
				return err
			}
			combined, err := config.CombinedSchema(name, schema)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(combined)
		}

		res, err := config.Resolve(dir, name, "")
		if err != nil {
			return err
		}
		out, err := yaml.Marshal(map[string]any{"profiles": map[string]any{name: res.Profile}})
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(out)
		return err
	},
}

func init() {
	configCmd.Flags().BoolVar(&showSchema, "schema", false, "print the JSON schema instead of the defaults")
	rootCmd.AddCommand(configCmd)
}
```

- [ ] **Step 7: Build and test everything**

Run: `go vet ./... && go test ./... && bats test/unit/`
Expected: all PASS. Note: `test/unit/gen1_profile.bats` still passes (gen1 files change in Task 4). `profiles/gen1/config.yaml` still has top-level `images:` which now resolves as the gen1 section body — this is already the new layout (body of `profiles.gen1`), so `go run . config gen1` must print `profiles: gen1: images: ...`; verify with `HOME=$(mktemp -d) sh -c 'mkdir -p $HOME/.local/share/kindtks && cp -r profiles $HOME/.local/share/kindtks/ && go run . config gen1'`.

- [ ] **Step 8: Commit**

```bash
git add -A internal/
git commit -m "feat: resolve config as common plus per-profile sections

Top-level images moved to profiles.<profile>.images; each section is
validated against the common and profile JSON schemas."
```

---

### Task 3: Profile config for scripts — `KINDTKS_PROFILE_CONFIG`, `KINDTKS_BIN`, `kindtks value`

**Files:**
- Create: `internal/cmd/value.go`, `internal/cmd/value_test.go`, `internal/cmd/run_test.go`
- Modify: `internal/cmd/run.go`

**Interfaces:**
- Consumes: `config.Resolved` (Task 2), `runProfileFunc` (Task 2).
- Produces: env `KINDTKS_BIN`, `KINDTKS_PROFILE_CONFIG`; hidden command `kindtks value <path> [--required]`; `func valueLines(doc any, path string) (lines []string, found bool, err error)`.

- [ ] **Step 1: Write failing tests**

`internal/cmd/value_test.go`:

```go
package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestValueLines(t *testing.T) {
	doc := decode(t, `{"gateway":{"extraHosts":["*.example.net","*.example.com"],"port":8080,"tls":false,"name":"gw","none":null},"empty":[]}`)
	cases := []struct {
		path  string
		lines []string
		found bool
	}{
		{"gateway.extraHosts", []string{"*.example.net", "*.example.com"}, true},
		{"gateway.port", []string{"8080"}, true},
		{"gateway.tls", []string{"false"}, true},
		{"gateway.name", []string{"gw"}, true},
		{"gateway.none", nil, false},
		{"gateway.missing", nil, false},
		{"gateway.name.deeper", nil, false},
		{"empty", []string{}, true},
	}
	for _, c := range cases {
		lines, found, err := valueLines(doc, c.path)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.path, err)
			continue
		}
		if found != c.found || (c.found && !reflect.DeepEqual(lines, c.lines)) {
			t.Errorf("%s: got %v %v, want %v %v", c.path, lines, found, c.lines, c.found)
		}
	}
}

func TestValueLines_Errors(t *testing.T) {
	doc := decode(t, `{"gateway":{"extraHosts":["a"]},"objs":[{"a":1}]}`)
	for _, path := range []string{"gateway", "objs", ""} {
		if _, _, err := valueLines(doc, path); err == nil {
			t.Errorf("%q: expected error", path)
		}
	}
}
```

`internal/cmd/run_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run 'ValueLines|RunProfileFunc' -v`
Expected: FAIL (`undefined: valueLines`; env missing `KINDTKS_BIN`).

- [ ] **Step 3: Implement**

Add to `runProfileFunc` in `internal/cmd/run.go`, before building `env`:

```go
	tmpDir, err := os.MkdirTemp("", "kindtks-profile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	profileCfg := filepath.Join(tmpDir, "profile-config.json")
	data, err := json.Marshal(res.Profile)
	if err != nil {
		return fmt.Errorf("encoding profile config: %w", err)
	}
	if err := os.WriteFile(profileCfg, data, 0600); err != nil {
		return err
	}
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating kindtks binary: %w", err)
	}
```

and append to the `env` list:

```go
		"KINDTKS_BIN="+bin,
		"KINDTKS_PROFILE_CONFIG="+profileCfg,
```

(import `encoding/json`).

`internal/cmd/value.go`:

```go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var valueRequired bool

var valueCmd = &cobra.Command{
	Use:    "value <path>",
	Short:  "Print a profile config value (for use in profile scripts)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := os.Getenv("KINDTKS_PROFILE_CONFIG")
		if path == "" {
			return fmt.Errorf("KINDTKS_PROFILE_CONFIG is not set; `kindtks value` is meant to be run from a profile script")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		dec := json.NewDecoder(f)
		dec.UseNumber()
		var doc any
		if err := dec.Decode(&doc); err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		lines, found, err := valueLines(doc, args[0])
		if err != nil {
			return err
		}
		if !found && valueRequired {
			return fmt.Errorf("%s is not set", args[0])
		}
		for _, l := range lines {
			fmt.Fprintln(cmd.OutOrStdout(), l)
		}
		return nil
	},
}

// valueLines resolves a dot path in doc and renders it for shell use: a
// scalar as one line, an array of scalars as one line per element. A missing
// or null value is not found.
func valueLines(doc any, path string) ([]string, bool, error) {
	if path == "" {
		return nil, false, fmt.Errorf("empty path")
	}
	cur := doc
	for _, key := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		if cur, ok = obj[key]; !ok {
			return nil, false, nil
		}
	}

	switch v := cur.(type) {
	case nil:
		return nil, false, nil
	case map[string]any:
		return nil, true, fmt.Errorf("%s is an object; query one of its fields", path)
	case []any:
		lines := make([]string, 0, len(v))
		for i, item := range v {
			s, err := scalarString(item)
			if err != nil {
				return nil, true, fmt.Errorf("%s[%d]: %w", path, i, err)
			}
			lines = append(lines, s)
		}
		return lines, true, nil
	default:
		s, err := scalarString(v)
		if err != nil {
			return nil, true, fmt.Errorf("%s: %w", path, err)
		}
		return []string{s}, true, nil
	}
}

func scalarString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	case bool:
		return strconv.FormatBool(x), nil
	default:
		return "", fmt.Errorf("not a scalar value")
	}
}

func init() {
	valueCmd.Flags().BoolVar(&valueRequired, "required", false, "fail if the value is not set")
	rootCmd.AddCommand(valueCmd)
}
```

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/
git commit -m "feat: expose profile config to scripts via kindtks value"
```

---

### Task 4: gen1 options, docs, and config migration

**Files:**
- Modify: `profiles/gen1/config.yaml`, `profiles/gen1/install.sh`, `profiles/gen1/README.md`, `README.md`, `test/unit/gen1_profile.bats`, `test/e2e/airgap-gen1.bats`, `test/e2e/registry-hosts.bats`
- Create: `profiles/gen1/config.schema.json`, `internal/config/profiles_test.go`

**Interfaces:**
- Consumes: `KINDTKS_BIN`, `kindtks value` (Task 3); `config.Resolve` (Task 2).

- [ ] **Step 1: Failing test that shipped profiles resolve**

`internal/config/profiles_test.go`:

```go
package config

import (
	"os"
	"testing"
)

// Every shipped profile's defaults must validate against its own schema.
func TestShippedProfilesResolve(t *testing.T) {
	entries, err := os.ReadDir("../../profiles")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			if _, err := Resolve("../../profiles", e.Name(), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGen1AcceptsExtraHosts(t *testing.T) {
	user := writeFile(t, "user.yaml", "profiles:\n  gen1:\n    gateway:\n      extraHosts: [\"*.example.net\", \"app.example.com\"]\n")
	if _, err := Resolve("../../profiles", "gen1", user); err != nil {
		t.Fatal(err)
	}
	bad := writeFile(t, "bad.yaml", "profiles:\n  gen1:\n    gateway:\n      extraHosts: [\"not a host\"]\n")
	if _, err := Resolve("../../profiles", "gen1", bad); err == nil {
		t.Fatal("invalid host must be rejected")
	}
}
```

Run: `go test ./internal/config/ -run 'Shipped|Gen1' -v`
Expected: `TestShippedProfilesResolve/gen1` passes already, `TestGen1AcceptsExtraHosts` FAILS (default schema rejects `gateway`).

- [ ] **Step 2: gen1 config and schema**

`profiles/gen1/config.yaml`:

```yaml
images:
  kind-node: kindest/node:v1.24.17
  cilium: quay.io/cilium/cilium:v1.13.10
  cilium-operator: quay.io/cilium/operator-generic:v1.13.10
  istio-pilot: docker.io/istio/pilot:1.16.7
  istio-proxy: docker.io/istio/proxyv2:1.16.7
  vault: docker.io/hashicorp/vault:2.0.3
  vault-secrets-operator: ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
gateway:
  extraHosts: []
```

`profiles/gen1/config.schema.json`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "kindtks gen1 profile",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "images": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "kind-node": {"type": "string"},
        "cilium": {"type": "string"},
        "cilium-operator": {"type": "string"},
        "istio-pilot": {"type": "string"},
        "istio-proxy": {"type": "string"},
        "vault": {"type": "string"},
        "vault-secrets-operator": {"type": "string"}
      }
    },
    "gateway": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "extraHosts": {
          "description": "Extra hosts accepted on the HTTP (port 80) server of Gateway istio-ingress/kindtks.",
          "type": "array",
          "uniqueItems": true,
          "items": {
            "type": "string",
            "pattern": "^(\\*\\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$"
          }
        }
      }
    }
  }
}
```

Run: `go test ./internal/config/ -v` → PASS.

- [ ] **Step 3: Failing bats tests for the gateway hosts**

Append to `test/unit/gen1_profile.bats`:

```bash
# --- gateway extra hosts ---

@test "add_gateway_extra_hosts patches each host onto the HTTP server" {
  export KUBECTL_ARGS_FILE="${BATS_TEST_TMPDIR}/kubectl_args"
  cat > "${MOCK_BIN}/kubectl" <<'MOCK'
#!/usr/bin/env bash
echo "$@" >> "$KUBECTL_ARGS_FILE"
MOCK
  chmod +x "${MOCK_BIN}/kubectl"
  cat > "${MOCK_BIN}/fake-kindtks" <<'MOCK'
#!/usr/bin/env bash
[ "$1 $2" = "value gateway.extraHosts" ] && printf '%s\n' '*.example.net' 'app.example.com'
MOCK
  chmod +x "${MOCK_BIN}/fake-kindtks"
  export KINDTKS_BIN="${MOCK_BIN}/fake-kindtks"

  run add_gateway_extra_hosts
  assert_success

  run cat "$KUBECTL_ARGS_FILE"
  assert_line --index 0 '-n istio-ingress patch gateway kindtks --type=json -p [{"op":"add","path":"/spec/servers/0/hosts/-","value":"*.example.net"}]'
  assert_line --index 1 '-n istio-ingress patch gateway kindtks --type=json -p [{"op":"add","path":"/spec/servers/0/hosts/-","value":"app.example.com"}]'
}

@test "add_gateway_extra_hosts does nothing without extra hosts" {
  export KUBECTL_ARGS_FILE="${BATS_TEST_TMPDIR}/kubectl_args"
  cat > "${MOCK_BIN}/kubectl" <<'MOCK'
#!/usr/bin/env bash
echo "$@" >> "$KUBECTL_ARGS_FILE"
MOCK
  chmod +x "${MOCK_BIN}/kubectl"
  printf '#!/usr/bin/env bash\n' > "${MOCK_BIN}/fake-kindtks"
  chmod +x "${MOCK_BIN}/fake-kindtks"
  export KINDTKS_BIN="${MOCK_BIN}/fake-kindtks"

  run add_gateway_extra_hosts
  assert_success
  [ ! -e "$KUBECTL_ARGS_FILE" ]
}

@test "gateway.yaml HTTP server is the first server" {
  run grep -m1 -A2 'port:' "${KINDTKS_PROFILE_DIR}/gateway.yaml"
  assert_output --partial "number: 80"
}
```

Run: `bats test/unit/gen1_profile.bats`
Expected: the two `add_gateway_extra_hosts` tests FAIL (`add_gateway_extra_hosts: command not found`).

- [ ] **Step 4: Implement in `install.sh`**

In `profiles/gen1/install.sh`, at the end of `install_istio` after `kubectl apply -f "$KINDTKS_PROFILE_DIR/gateway.yaml"`, add a call:

```bash
  add_gateway_extra_hosts
```

and add the function after `install_istio`:

```bash
# Adds profiles.gen1.gateway.extraHosts to the HTTP server (servers[0]) of
# the kindtks Gateway. HTTPS is untouched: kindtks-tls only covers the
# built-in domains.
add_gateway_extra_hosts() {
  local hosts host
  hosts=$("$KINDTKS_BIN" value gateway.extraHosts)
  while IFS= read -r host; do
    [ -n "$host" ] || continue
    echo "==> Adding gateway host ${host} (HTTP)..."
    kubectl -n istio-ingress patch gateway kindtks --type=json \
      -p "[{\"op\":\"add\",\"path\":\"/spec/servers/0/hosts/-\",\"value\":\"${host}\"}]"
  done <<< "$hosts"
}
```

Run: `bats test/unit/` → all PASS. Also `go test ./...` → PASS.

- [ ] **Step 5: Migrate e2e configs**

`test/e2e/airgap-gen1.bats`, in the heredoc for `~/airgap-config.yaml`, wrap images under the profile:

```yaml
profiles:
    gen1:
        images:
            cilium: registry.airgap:5000/quay.io/cilium/cilium:v1.13.10
            cilium-operator: registry.airgap:5000/quay.io/cilium/operator-generic:v1.13.10
            istio-pilot: registry.airgap:5000/docker.io/istio/pilot:1.16.7
            istio-proxy: registry.airgap:5000/docker.io/istio/proxyv2:1.16.7
            vault: registry.airgap:5000/hashicorp/vault:2.0.3
            vault-secrets-operator: registry.airgap:5000/ghcr.io/ricoberger/vault-secrets-operator:v1.26.0
registries:
    registry.airgap:5000:
        auth:
            username: airgap
            password: airgap
```

`test/e2e/registry-hosts.bats`, in the user config heredoc, replace

```yaml
images:
  probe: registry.e2e.invalid:5000/probe:v1
```

with

```yaml
profiles:
  ${PROFILE}:
    images:
      probe: registry.e2e.invalid:5000/probe:v1
```

(the heredoc is unquoted, so `${PROFILE}` expands to `rh-e2e`; the test profile has no schema, so only `images` is allowed — as intended).

- [ ] **Step 6: Docs**

`README.md` — in "Custom Image Registry":
- Change the `kindtks config gen1 > my-config.yaml` example comment to `# Edit profiles.gen1.images to point at your registry`.
- Replace the air-gap example with the same images nested under `profiles:\n    gen1:\n        images:` (4-space indentation as in the current file).
- In the Authentication example, nest `images` the same way (`profiles: gen1: images: cilium: registry.corp.com/...`).
- Add after the "Migrating from 0.3.0" line: `Migrating from 0.4.0: top-level \`images\` moved to \`profiles.<profile>.images\`.`

Add a new section before "Custom Image Registry":

````markdown
## Configuration

`kindtks create <profile> --config <file>` merges `<file>` over the profile's defaults (`kindtks config <profile>` prints them). Objects merge key by key; lists and scalars replace.

```yaml
registries:            # common to all profiles (see Custom Image Registry)
  ...
profiles:
  gen1:                # options of the gen1 profile
    images:
      cilium: quay.io/cilium/cilium:v1.13.10
    gateway:
      extraHosts: ["*.example.net"]
```

One file can hold sections for several profiles; sections for profiles not installed are ignored with a warning. Each profile validates its section with its own JSON Schema (`profiles/<name>/config.schema.json`). For editor completion:

```bash
kindtks config gen1 --schema > kindtks.schema.json
# then in your config file:
# yaml-language-server: $schema=./kindtks.schema.json
```

Profile scripts read their options with `"$KINDTKS_BIN" value <dot.path>` (lists print one item per line).
````

`profiles/gen1/README.md` — in "## Config":
- Replace the printed config example with the new layout (`profiles:\n    gen1:\n        images: ...\n        gateway:\n            extraHosts: []`).
- Change "Override any image" to "Override images or options".
- Nest the images in the Registry Authentication example under `profiles: gen1:`.
- Add a subsection:

````markdown
### Extra Gateway hosts

`gateway.extraHosts` adds hosts to the HTTP (port 80) server of Gateway `istio-ingress/kindtks`, e.g. for apps whose TLS terminates upstream:

```yaml
profiles:
    gen1:
        gateway:
            extraHosts: ["*.example.net"]
```

HTTPS still serves only `*.kindtks.localhost` and `*.kindtks.local`. Route a host with a VirtualService bound to `istio-ingress/kindtks`.
````

Verify: `grep -rn '^images:' README.md profiles/gen1/README.md test/e2e/*.bats` prints nothing outside `profiles/gen1/config.yaml`-style bodies; `grep -rniE 'jsgr|rophyinc' README.md profiles test` prints nothing.

- [ ] **Step 7: Commit**

```bash
git add profiles/gen1 README.md test/unit/gen1_profile.bats test/e2e/airgap-gen1.bats test/e2e/registry-hosts.bats internal/config/profiles_test.go
git commit -m "feat: add gateway.extraHosts option to gen1 profile"
```

---

### Task 5: End-to-end verification

**Files:**
- Modify: `test/e2e/gen1.bats`

- [ ] **Step 1: Create gen1 with an extra host**

In `test/e2e/gen1.bats` `setup_file`, replace `HOME="$E2E_HOME" "${E2E_HOME}/.local/bin/kindtks" create gen1` with:

```bash
  cat > "${BATS_FILE_TMPDIR}/config.yaml" <<'EOF'
profiles:
  gen1:
    gateway:
      extraHosts: ["*.example.net"]
EOF
  HOME="$E2E_HOME" "${E2E_HOME}/.local/bin/kindtks" create gen1 --config "${BATS_FILE_TMPDIR}/config.yaml"
```

- [ ] **Step 2: Assert the Gateway and routing**

Add after the existing "gateway has HTTPS server with TLS" test:

```bash
@test "gateway HTTP server includes extra hosts from config" {
  run kube -n istio-ingress get gateway kindtks -o jsonpath='{.spec.servers[0].hosts}'
  assert_output --partial '*.example.net'
  run kube -n istio-ingress get gateway kindtks -o jsonpath='{.spec.servers[1].hosts}'
  refute_output --partial 'example.net'
}
```

In "deploy echo service and reach it via gateway", add `- echo.example.net` to the VirtualService `hosts`, and before `# Cleanup` add:

```bash
  run curl -s --resolve echo.example.net:30080:127.0.0.1 http://echo.example.net:30080
  assert_success
  assert_output --partial "kindtks-ok"
```

- [ ] **Step 3: Run the e2e suites (foreground, Bash timeout 600000; nohup + polling if longer)**

- `bats test/e2e/gen1.bats` → all PASS (host ports 30080/30443 must be free; check `docker ps` first and report if another cluster holds them — do not delete other clusters).
- `bats test/e2e/registry-hosts.bats` → 5/5 PASS.
- `AIRGAP_LAB_DIR=/home/rophy/projects/airgap-lab AIRGAP_VM_IP=10.99.0.10 AIRGAP_REGISTRY_USER=airgap AIRGAP_REGISTRY_PASS=airgap bats test/e2e/airgap-gen1.bats` → 15/15 PASS.

- [ ] **Step 4: Commit**

```bash
git add test/e2e/gen1.bats
git commit -m "test: cover gen1 gateway extraHosts end to end"
```
