# Prompted and Checked Registry Credentials — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `registries.<reg>.auth: prompt` asks for credentials on the terminal at `kindtks create`, and every registry credential is checked against the registry (like `docker login`) before anything is created.

**Architecture:** `internal/config` learns the `prompt` form, the endpoint containerd will use for a registry name, and the pull scopes of the profile's images. A new `internal/regauth` package performs the `/v2/` handshake (basic or bearer token). A credential flow in `internal/cmd` checks file credentials, prompts (with reuse and up to 3 attempts), and is wired into `create` with `/dev/tty`, hidden password input (`golang.org/x/term`) and `--skip-auth-check`.

**Tech Stack:** Go 1.24 (cobra, net/http, BurntSushi/toml, golang.org/x/term v0.40.0), bats, Docker.

**Spec:** `docs/superpowers/specs/2026-10-08-registry-auth-prompt-design.md`

## Global Constraints

- Branch `registry-auth-prompt`. Never `git push`.
- Commit messages: `<type>: <description>`, types feat/fix/refactor/chore/docs/build/test; no mention of Claude, no Co-Authored-By, no "Generated with" footer. Commits are GPG-signed automatically; never disable signing.
- Public content uses only `example.com` / `example.net` style names; never private or internal domains or hostnames.
- `auth` accepts `{username, password}` or the exact string `prompt`; nothing else.
- At most 3 attempts per prompted registry; check timeout 10 s per request.
- New dependency: `golang.org/x/term` v0.40.0 exactly (with `golang.org/x/sys` v0.41.0); `go.mod` stays `go 1.24.13`.
- Credentials are never written anywhere except the kind config patch kindtks already generates.
- Run long commands in the foreground; never use run_in_background, Monitor, or wait loops. Never touch kind clusters you did not create, and never the real `~/.local`.

---

### Task 1: Config — `auth: prompt`, check endpoint, pull scopes

**Files:**
- Modify: `internal/config/config.go`, `internal/config/registry.go`, `internal/config/schema/common.json`
- Create: `internal/config/auth_test.go`

**Interfaces:**
- Produces: `RegistryAuth.Prompt bool`; `func (c *Config) PromptRegistries() []string`; `type AuthEndpoint struct { URL string; SkipVerify bool }`; `func (c *Config) AuthEndpoint(name string) AuthEndpoint`; `func (c *Config) PullScopes(name string) []string`.

- [ ] **Step 1: Write the failing tests** — `internal/config/auth_test.go`:

```go
package config

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestRegistryAuth_PromptForm(t *testing.T) {
	var regs map[string]*Registry
	data := `{"a.example.com":{"auth":"prompt"},"b.example.com":{"auth":{"username":"u","password":"p"}}}`
	if err := json.Unmarshal([]byte(data), &regs); err != nil {
		t.Fatal(err)
	}
	if a := regs["a.example.com"].Auth; a == nil || !a.Prompt {
		t.Errorf("prompt form not decoded: %+v", a)
	}
	if a := regs["b.example.com"].Auth; a.Prompt || a.Username != "u" || a.Password != "p" {
		t.Errorf("object form changed: %+v", a)
	}
	if err := json.Unmarshal([]byte(`{"auth":"ask"}`), new(Registry)); err == nil {
		t.Error(`auth "ask": want error`)
	}
}

func TestPromptRegistries(t *testing.T) {
	cfg := &Config{Registries: map[string]*Registry{
		"z.example.com": {Auth: &RegistryAuth{Prompt: true}},
		"a.example.com": {Auth: &RegistryAuth{Prompt: true}},
		"m.example.com": {Auth: &RegistryAuth{Username: "u", Password: "p"}},
		"h.example.com": {Hosts: "server = \"https://h.example.com\"\n"},
	}}
	if got := cfg.PromptRegistries(); !slices.Equal(got, []string{"a.example.com", "z.example.com"}) {
		t.Errorf("got %v", got)
	}
}

func TestValidateCommon_AuthPrompt(t *testing.T) {
	doc := func(auth any) map[string]any {
		return map[string]any{"registries": map[string]any{"r.example.com": map[string]any{"auth": auth}}}
	}
	if err := ValidateCommon(doc("prompt")); err != nil {
		t.Errorf("prompt: %v", err)
	}
	if err := ValidateCommon(doc(map[string]any{"username": "u", "password": "p"})); err != nil {
		t.Errorf("object: %v", err)
	}
	if err := ValidateCommon(doc("ask")); err == nil {
		t.Error(`"ask": want error`)
	}
}

func TestAuthEndpoint(t *testing.T) {
	cfg := &Config{
		Images: map[string]string{"app": "registry.example.com:5000/team/app:1"},
		Registries: map[string]*Registry{
			"docker.io": {Hosts: "server = \"https://registry-1.docker.io\"\n\n" +
				"[host.\"http://mirror.example.com:5000/v2/docker.io\"]\n" +
				"  capabilities = [\"pull\", \"resolve\"]\n  override_path = true\n"},
			"secure.example.com": {Auth: &RegistryAuth{Username: "u", Password: "p"}},
		},
	}
	cases := map[string]AuthEndpoint{
		"registry.example.com:5000": {URL: "http://registry.example.com:5000", SkipVerify: true},
		"mirror.example.com:5000":   {URL: "http://mirror.example.com:5000"},
		"docker.io":                 {URL: "https://registry-1.docker.io"},
		"secure.example.com":        {URL: "https://secure.example.com"},
	}
	for name, want := range cases {
		if got := cfg.AuthEndpoint(name); got != want {
			t.Errorf("%s: got %+v, want %+v", name, got, want)
		}
	}
}

func TestPullScopes(t *testing.T) {
	cfg := &Config{Images: map[string]string{
		"a": "nginx:1.25",
		"b": "docker.io/istio/pilot:1.16.7",
		"c": "docker.io/busybox@sha256:0123456789abcdef",
		"d": "registry.example.com:5000/team/app:1",
		"e": "registry.example.com:5000/team/app:2",
	}}
	if got, want := cfg.PullScopes("docker.io"), []string{
		"repository:istio/pilot:pull", "repository:library/busybox:pull", "repository:library/nginx:pull",
	}; !slices.Equal(got, want) {
		t.Errorf("docker.io: got %v", got)
	}
	if got := cfg.PullScopes("registry.example.com:5000"); !slices.Equal(got, []string{"repository:team/app:pull"}) {
		t.Errorf("private: got %v", got)
	}
	if got := cfg.PullScopes("other.example.com"); len(got) != 0 {
		t.Errorf("other: got %v", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/ -run 'TestRegistryAuth_PromptForm|TestPromptRegistries|TestValidateCommon_AuthPrompt|TestAuthEndpoint|TestPullScopes' -v`
Expected: compile failure (`Prompt`, `PromptRegistries`, `AuthEndpoint`, `PullScopes` undefined).

- [ ] **Step 3: Implement**

In `internal/config/config.go`, replace `RegistryAuth` with (add `encoding/json` import):

```go
// RegistryAuth is a registry's credentials, or Prompt when the config says
// `auth: prompt` and create asks for them on the terminal.
type RegistryAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Prompt   bool   `json:"-"`
}

// UnmarshalJSON accepts {username, password} or the string "prompt".
func (a *RegistryAuth) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s != "prompt" {
			return fmt.Errorf(`auth must be {username, password} or "prompt", got %q`, s)
		}
		*a = RegistryAuth{Prompt: true}
		return nil
	}
	type plain RegistryAuth
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*a = RegistryAuth(p)
	return nil
}
```

In `internal/config/registry.go` (add imports `net/url`, `github.com/BurntSushi/toml`), add:

```go
// PromptRegistries returns the sorted names of registries whose credentials
// are still to be asked for.
func (c *Config) PromptRegistries() []string {
	var names []string
	for name, r := range c.Registries {
		if r != nil && r.Auth != nil && r.Auth.Prompt {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// AuthEndpoint is where containerd connects for a registry host name, and so
// where its credentials are checked.
type AuthEndpoint struct {
	URL        string // scheme://host[:port]
	SkipVerify bool
}

// AuthEndpoint finds name in the host files kindtks writes (explicit or
// auto-trust), as a server or a [host."…"] URL; otherwise docker.io maps to
// Docker Hub's API host and anything else to https://name.
func (c *Config) AuthEndpoint(name string) AuthEndpoint {
	files := c.HostsFiles()
	regs := make([]string, 0, len(files))
	for reg := range files {
		regs = append(regs, reg)
	}
	sort.Strings(regs)
	for _, reg := range regs {
		var doc struct {
			Server     string `toml:"server"`
			SkipVerify bool   `toml:"skip_verify"`
			Host       map[string]struct {
				SkipVerify bool `toml:"skip_verify"`
			} `toml:"host"`
		}
		if _, err := toml.Decode(files[reg], &doc); err != nil {
			continue
		}
		hosts := make([]string, 0, len(doc.Host))
		for u := range doc.Host {
			hosts = append(hosts, u)
		}
		sort.Strings(hosts)
		for _, u := range hosts {
			if ep, ok := endpointFor(u, name); ok {
				ep.SkipVerify = doc.Host[u].SkipVerify
				return ep
			}
		}
		if ep, ok := endpointFor(doc.Server, name); ok {
			ep.SkipVerify = doc.SkipVerify
			return ep
		}
	}
	if name == "docker.io" {
		return AuthEndpoint{URL: "https://registry-1.docker.io"}
	}
	return AuthEndpoint{URL: "https://" + name}
}

func endpointFor(raw, name string) (AuthEndpoint, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host != name || (u.Scheme != "http" && u.Scheme != "https") {
		return AuthEndpoint{}, false
	}
	return AuthEndpoint{URL: u.Scheme + "://" + u.Host}, true
}

// PullScopes returns the token scopes for pulling the profile's images from
// registry name.
func (c *Config) PullScopes(name string) []string {
	seen := map[string]bool{}
	var scopes []string
	for _, image := range c.Images {
		reg, repo := imageRepository(image)
		scope := "repository:" + repo + ":pull"
		if reg == name && !seen[scope] {
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	return scopes
}

// imageRepository splits an image reference into the registry it is pulled
// from (docker.io for Docker Hub, implicit or explicit) and its repository.
func imageRepository(image string) (registry, repo string) {
	ref := image
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon >= 0 && !strings.Contains(ref[colon+1:], "/") {
		ref = ref[:colon]
	}
	registry, repo = RegistryFromImage(image), ref
	if registry == "" {
		registry = "docker.io"
	} else {
		repo = strings.TrimPrefix(ref, registry+"/")
	}
	if registry == "docker.io" && !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	return registry, repo
}
```

In `internal/config/schema/common.json`, replace the `"auth": {…}` object with:

```json
          "auth": {
            "oneOf": [
              {
                "type": "object",
                "additionalProperties": false,
                "required": ["username", "password"],
                "properties": {
                  "username": {"type": "string"},
                  "password": {"type": "string"}
                }
              },
              {"const": "prompt"}
            ]
          }
```

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./...`
Expected: PASS (new and existing tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat: accept auth: prompt and derive registry check endpoints and scopes"
```

---

### Task 2: `internal/regauth` — the `docker login` handshake

**Files:**
- Create: `internal/regauth/regauth.go`, `internal/regauth/regauth_test.go`

**Interfaces:**
- Produces: `type Endpoint struct { URL string; SkipVerify bool }`; `type Status int` with `OK`, `Rejected`, `NoAuth`; `type Result struct { Status Status; Detail string }`; `func Check(ctx context.Context, ep Endpoint, username, password string, scopes []string) (Result, error)` — an error means the credentials could not be checked.

- [ ] **Step 1: Write the failing tests** — `internal/regauth/regauth_test.go`:

```go
package regauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func basicHandler(user, pass string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); ok && u == user && p == pass {
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
		w.WriteHeader(http.StatusUnauthorized)
	}
}

// bearerServer challenges /v2/ with a token realm on itself and records the
// scopes and service the token request carried.
func bearerServer(t *testing.T, user, pass string, scopes *[]string, service *string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="test-registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		*scopes = r.URL.Query()["scope"]
		*service = r.URL.Query().Get("service")
		if u, p, ok := r.BasicAuth(); !ok || u != user || p != pass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"token":"t"}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func check(t *testing.T, ep Endpoint, user, pass string, scopes []string) (Result, error) {
	t.Helper()
	return Check(context.Background(), ep, user, pass, scopes)
}

func TestCheck_Basic(t *testing.T) {
	srv := httptest.NewServer(basicHandler("u", "p"))
	defer srv.Close()
	if r, err := check(t, Endpoint{URL: srv.URL}, "u", "p", nil); err != nil || r.Status != OK {
		t.Errorf("right: %+v, %v", r, err)
	}
	r, err := check(t, Endpoint{URL: srv.URL}, "u", "wrong", nil)
	if err != nil || r.Status != Rejected || r.Detail != "401 Unauthorized" {
		t.Errorf("wrong: %+v, %v", r, err)
	}
}

func TestCheck_Bearer(t *testing.T) {
	var scopes []string
	var service string
	srv := bearerServer(t, "u", "p", &scopes, &service)
	want := []string{"repository:a:pull", "repository:b:pull"}
	if r, err := check(t, Endpoint{URL: srv.URL}, "u", "p", want); err != nil || r.Status != OK {
		t.Errorf("right: %+v, %v", r, err)
	}
	if !slices.Equal(scopes, want) || service != "test-registry" {
		t.Errorf("token request: scopes %v, service %q", scopes, service)
	}
	if r, err := check(t, Endpoint{URL: srv.URL}, "u", "wrong", want); err != nil || r.Status != Rejected {
		t.Errorf("wrong: %+v, %v", r, err)
	}
}

func TestCheck_NoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	if r, err := check(t, Endpoint{URL: srv.URL}, "u", "p", nil); err != nil || r.Status != NoAuth {
		t.Errorf("got %+v, %v", r, err)
	}
}

func TestCheck_CouldNotCheck(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	if _, err := check(t, Endpoint{URL: failing.URL}, "u", "p", nil); err == nil {
		t.Error("500: want error")
	}

	closed := httptest.NewServer(basicHandler("u", "p"))
	closed.Close()
	if _, err := check(t, Endpoint{URL: closed.URL}, "u", "p", nil); err == nil {
		t.Error("connection refused: want error")
	}

	plain := httptest.NewServer(basicHandler("u", "p"))
	defer plain.Close()
	if _, err := check(t, Endpoint{URL: "https://" + strings.TrimPrefix(plain.URL, "http://")}, "u", "p", nil); err == nil {
		t.Error("TLS to plain HTTP: want error")
	}
}

func TestCheck_SkipVerify(t *testing.T) {
	srv := httptest.NewTLSServer(basicHandler("u", "p"))
	defer srv.Close()
	if _, err := check(t, Endpoint{URL: srv.URL}, "u", "p", nil); err == nil {
		t.Error("self-signed without skip: want error")
	}
	if r, err := check(t, Endpoint{URL: srv.URL, SkipVerify: true}, "u", "p", nil); err != nil || r.Status != OK {
		t.Errorf("skip verify: %+v, %v", r, err)
	}
}

func TestParseChallenge_QuotedCommas(t *testing.T) {
	scheme, params := parseChallenge(`Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:a:pull,push"`)
	if scheme != "Bearer" || params["realm"] != "https://auth.example.com/token" ||
		params["service"] != "registry.example.com" || params["scope"] != "repository:a:pull,push" {
		t.Errorf("got %q %v", scheme, params)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/regauth/ -v`
Expected: compile failure (package has no non-test files / `Check` undefined).

- [ ] **Step 3: Implement** — `internal/regauth/regauth.go`:

```go
// Package regauth checks registry credentials the way `docker login` does:
// GET /v2/, then answer its Basic or Bearer challenge with the credentials.
package regauth

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoint is the registry API base URL (scheme://host[:port]) and whether to
// skip TLS verification, as containerd will connect to it.
type Endpoint struct {
	URL        string
	SkipVerify bool
}

type Status int

const (
	OK       Status = iota // credentials accepted
	Rejected               // registry answered 401 or 403
	NoAuth                 // registry needs no login
)

type Result struct {
	Status Status
	Detail string // HTTP status of a rejection
}

const timeout = 10 * time.Second

// Check returns OK, Rejected or NoAuth; an error means the credentials could
// not be checked (network, TLS, or an unexpected answer).
func Check(ctx context.Context, ep Endpoint, username, password string, scopes []string) (Result, error) {
	client := newClient(ep.SkipVerify)
	resp, err := get(ctx, client, ep.URL+"/v2/", "", "")
	if err != nil {
		return Result{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return Result{Status: NoAuth}, nil
	case http.StatusUnauthorized:
	default:
		return Result{}, fmt.Errorf("unexpected %s from %s/v2/", resp.Status, ep.URL)
	}

	challenge := resp.Header.Get("WWW-Authenticate")
	scheme, params := parseChallenge(challenge)
	var target string
	switch strings.ToLower(scheme) {
	case "basic":
		target = ep.URL + "/v2/"
	case "bearer":
		u, err := url.Parse(params["realm"])
		if err != nil || params["realm"] == "" {
			return Result{}, fmt.Errorf("unusable login challenge %q", challenge)
		}
		q := u.Query()
		if s := params["service"]; s != "" {
			q.Set("service", s)
		}
		for _, s := range scopes {
			q.Add("scope", s)
		}
		u.RawQuery = q.Encode()
		target = u.String()
	default:
		return Result{}, fmt.Errorf("unsupported login challenge %q", challenge)
	}

	resp, err = get(ctx, client, target, username, password)
	if err != nil {
		return Result{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return Result{Status: OK}, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return Result{Status: Rejected, Detail: resp.Status}, nil
	}
	return Result{}, fmt.Errorf("unexpected %s from %s", resp.Status, target)
}

func newClient(skipVerify bool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		// Matches containerd's skip_verify for this host.
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

func get(ctx context.Context, client *http.Client, target, username, password string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	return resp, nil
}

// parseChallenge splits `Scheme k1="v1",k2=v2` into the scheme and its
// parameters; quoted values may contain commas.
func parseChallenge(h string) (string, map[string]string) {
	scheme, rest, _ := strings.Cut(strings.TrimSpace(h), " ")
	params := map[string]string{}
	for rest = strings.TrimSpace(rest); rest != ""; {
		key, after, ok := strings.Cut(rest, "=")
		if !ok {
			break
		}
		key = strings.ToLower(strings.TrimSpace(key))
		var val string
		if strings.HasPrefix(after, `"`) {
			end := strings.Index(after[1:], `"`)
			if end < 0 {
				val, rest = after[1:], ""
			} else {
				val, rest = after[1:1+end], after[2+end:]
			}
		} else {
			val, rest, _ = strings.Cut(after, ",")
		}
		params[key] = val
		rest = strings.TrimLeft(strings.TrimSpace(rest), ", ")
	}
	return scheme, params
}
```

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./internal/regauth/ -v && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/regauth
git commit -m "feat: check registry credentials with the docker login handshake"
```

---

### Task 3: Credential flow (check file credentials, prompt, reuse, retry)

**Files:**
- Create: `internal/cmd/credentials.go`, `internal/cmd/credentials_test.go`

**Interfaces:**
- Consumes: `config.Config.AuthRegistries()`, `PromptRegistries()`, `RegistryAuth{Username, Password, Prompt}` (Task 1); `regauth.Result`, `regauth.OK|Rejected|NoAuth` (Task 2).
- Produces: `type terminal interface { ReadLine(prompt string) (string, error); ReadPassword(prompt string) (string, error) }`; `type checkFunc func(registry, username, password string) (regauth.Result, error)`; `type credentialFlow struct { cfg *config.Config; term terminal; out io.Writer; check checkFunc }`; `func (f *credentialFlow) run() error`. `term == nil` means no terminal; `check == nil` means `--skip-auth-check`. On success every prompted registry's `Auth` has `Username`/`Password` set and `Prompt` false.

- [ ] **Step 1: Write the failing tests** — `internal/cmd/credentials_test.go`:

```go
package cmd

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/regauth"
)

type fakeTerm struct {
	answers []string
	prompts []string
}

func (f *fakeTerm) next(p string) (string, error) {
	f.prompts = append(f.prompts, p)
	if len(f.answers) == 0 {
		return "", io.EOF
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return a, nil
}

func (f *fakeTerm) ReadLine(p string) (string, error)     { return f.next(p) }
func (f *fakeTerm) ReadPassword(p string) (string, error) { return f.next(p) }

// fakeCheck accepts the password "right" (strict.example.com accepts only
// "right-strict"), says open.example.com needs no login, and cannot reach
// down.example.com.
func fakeCheck(calls *[]string) checkFunc {
	return func(reg, user, pass string) (regauth.Result, error) {
		*calls = append(*calls, reg+":"+user+":"+pass)
		switch {
		case reg == "down.example.com":
			return regauth.Result{}, errors.New("dial tcp: connection refused")
		case reg == "open.example.com":
			return regauth.Result{Status: regauth.NoAuth}, nil
		case reg == "strict.example.com" && pass == "right-strict", reg != "strict.example.com" && pass == "right":
			return regauth.Result{Status: regauth.OK}, nil
		}
		return regauth.Result{Status: regauth.Rejected, Detail: "401 Unauthorized"}, nil
	}
}

func cfgWith(regs map[string]*config.RegistryAuth) *config.Config {
	cfg := &config.Config{Registries: map[string]*config.Registry{}}
	for name, auth := range regs {
		cfg.Registries[name] = &config.Registry{Auth: auth}
	}
	return cfg
}

func prompt() *config.RegistryAuth { return &config.RegistryAuth{Prompt: true} }

func runFlow(cfg *config.Config, term terminal, check checkFunc) (string, error) {
	var out bytes.Buffer
	f := &credentialFlow{cfg: cfg, out: &out, check: check}
	if term != nil {
		f.term = term
	}
	err := f.run()
	return out.String(), err
}

func wantAuth(t *testing.T, cfg *config.Config, reg, user, pass string) {
	t.Helper()
	a := cfg.Registries[reg].Auth
	if a.Prompt || a.Username != user || a.Password != pass {
		t.Errorf("%s auth = %+v, want %s/%s", reg, a, user, pass)
	}
}

func TestCredentials_PromptAccepted(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "right"}}
	if _, err := runFlow(cfg, term, fakeCheck(&calls)); err != nil {
		t.Fatal(err)
	}
	wantAuth(t, cfg, "a.example.com", "svc", "right")
	if len(calls) != 1 {
		t.Errorf("checks: %v", calls)
	}
}

func TestCredentials_RejectedThenAccepted(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "wrong", "svc", "right"}}
	out, err := runFlow(cfg, term, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Rejected by a.example.com (401 Unauthorized)") {
		t.Errorf("output:\n%s", out)
	}
	wantAuth(t, cfg, "a.example.com", "svc", "right")
}

func TestCredentials_ThreeRejections(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "w1", "svc", "w2", "svc", "w3"}}
	_, err := runFlow(cfg, term, fakeCheck(&calls))
	if err == nil || !strings.Contains(err.Error(), "rejected 3 times") {
		t.Fatalf("got %v", err)
	}
}

func TestCredentials_ReuseAccepted(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt(), "b.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "right", ""}}
	if _, err := runFlow(cfg, term, fakeCheck(&calls)); err != nil {
		t.Fatal(err)
	}
	wantAuth(t, cfg, "b.example.com", "svc", "right")
	if !strings.Contains(strings.Join(term.prompts, "\n"), "Use the same as a.example.com (svc)? [Y/n]") {
		t.Errorf("prompts: %q", term.prompts)
	}
}

func TestCredentials_ReuseDeclined(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt(), "b.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "right", "n", "other", "right"}}
	if _, err := runFlow(cfg, term, fakeCheck(&calls)); err != nil {
		t.Fatal(err)
	}
	wantAuth(t, cfg, "b.example.com", "other", "right")
}

func TestCredentials_ReusedRejectedAsksAgain(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt(), "strict.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "right", "y", "svc", "right-strict"}}
	out, err := runFlow(cfg, term, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Rejected by strict.example.com") {
		t.Errorf("output:\n%s", out)
	}
	wantAuth(t, cfg, "strict.example.com", "svc", "right-strict")
}

func TestCredentials_EmptyInputCountsAsAttempt(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	term := &fakeTerm{answers: []string{"", "", "svc", "right"}}
	out, err := runFlow(cfg, term, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "required") {
		t.Errorf("output:\n%s", out)
	}
	wantAuth(t, cfg, "a.example.com", "svc", "right")
}

func TestCredentials_CouldNotCheck(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"down.example.com": prompt()})
	out, err := runFlow(cfg, &fakeTerm{answers: []string{"svc", "pw", "y"}}, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "connection refused") {
		t.Errorf("output:\n%s", out)
	}
	wantAuth(t, cfg, "down.example.com", "svc", "pw")

	cfg = cfgWith(map[string]*config.RegistryAuth{"down.example.com": prompt()})
	if _, err := runFlow(cfg, &fakeTerm{answers: []string{"svc", "pw", ""}}, fakeCheck(&calls)); err == nil ||
		!strings.Contains(err.Error(), "could not check") {
		t.Errorf("default no: got %v", err)
	}
}

func TestCredentials_NoTerminal(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	_, err := runFlow(cfg, nil, fakeCheck(&calls))
	if err == nil || err.Error() != `registries.a.example.com.auth is "prompt", but there is no terminal to ask on` {
		t.Fatalf("got %v", err)
	}
}

func TestCredentials_NoAuthNote(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"open.example.com": prompt()})
	out, err := runFlow(cfg, &fakeTerm{answers: []string{"svc", "pw"}}, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "open.example.com does not require a login; credentials not checked") {
		t.Errorf("output:\n%s", out)
	}
}

func TestCredentials_SkipCheck(t *testing.T) {
	cfg := cfgWith(map[string]*config.RegistryAuth{
		"a.example.com": prompt(),
		"b.example.com": {Username: "u", Password: "anything"},
	})
	if _, err := runFlow(cfg, &fakeTerm{answers: []string{"svc", "anything"}}, nil); err != nil {
		t.Fatal(err)
	}
	wantAuth(t, cfg, "a.example.com", "svc", "anything")
}

func TestCredentials_FileCredentials(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{
		"a.example.com": {Username: "u", Password: "wrong"},
		"b.example.com": prompt(),
	})
	term := &fakeTerm{answers: []string{"svc", "right"}}
	_, err := runFlow(cfg, term, fakeCheck(&calls))
	if err == nil || !strings.Contains(err.Error(), "rejected") || !strings.Contains(err.Error(), "a.example.com") {
		t.Fatalf("got %v", err)
	}
	if len(term.prompts) != 0 {
		t.Errorf("prompted before failing on file credentials: %q", term.prompts)
	}

	cfg = cfgWith(map[string]*config.RegistryAuth{"down.example.com": {Username: "u", Password: "p"}})
	if _, err := runFlow(cfg, nil, fakeCheck(&calls)); err == nil || !strings.Contains(err.Error(), "--skip-auth-check") {
		t.Errorf("could not check: got %v", err)
	}

	cfg = cfgWith(map[string]*config.RegistryAuth{"a.example.com": {Username: "u", Password: "right"}})
	if _, err := runFlow(cfg, nil, fakeCheck(&calls)); err != nil {
		t.Errorf("accepted: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run TestCredentials -v`
Expected: compile failure (`credentialFlow`, `terminal`, `checkFunc` undefined).

- [ ] **Step 3: Implement** — `internal/cmd/credentials.go`:

```go
package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/regauth"
)

const maxAttempts = 3

// terminal is where credentials are asked for; tests fake it.
type terminal interface {
	ReadLine(prompt string) (string, error)
	ReadPassword(prompt string) (string, error)
}

// checkFunc checks credentials against a registry; an error means they could
// not be checked.
type checkFunc func(registry, username, password string) (regauth.Result, error)

// credentialFlow checks the credentials written in the config, then asks for
// the ones marked `auth: prompt`. term is nil without a terminal; check is
// nil with --skip-auth-check.
type credentialFlow struct {
	cfg   *config.Config
	term  terminal
	out   io.Writer
	check checkFunc
}

type credentials struct {
	registry, username, password string
}

func (f *credentialFlow) run() error {
	if f.check != nil {
		for _, name := range f.cfg.AuthRegistries() {
			auth := f.cfg.Registries[name].Auth
			if auth.Prompt {
				continue
			}
			res, err := f.check(name, auth.Username, auth.Password)
			if err != nil {
				return fmt.Errorf("could not check the credentials for %s: %v (use --skip-auth-check to create anyway)", name, err)
			}
			switch res.Status {
			case regauth.Rejected:
				return fmt.Errorf("the credentials for %s in the config file were rejected by the registry (%s)", name, res.Detail)
			case regauth.NoAuth:
				fmt.Fprintf(f.out, "%s does not require a login; credentials not checked\n", name)
			}
		}
	}

	var last *credentials
	for _, name := range f.cfg.PromptRegistries() {
		if f.term == nil {
			return fmt.Errorf(`registries.%s.auth is "prompt", but there is no terminal to ask on`, name)
		}
		fmt.Fprintf(f.out, "Credentials for %s\n", name)
		c, err := f.ask(name, last)
		if err != nil {
			return err
		}
		auth := f.cfg.Registries[name].Auth
		auth.Username, auth.Password, auth.Prompt = c.username, c.password, false
		last = c
	}
	return nil
}

// ask offers the previous registry's credentials, then asks for new ones,
// until the registry accepts them or maxAttempts entries were made.
func (f *credentialFlow) ask(name string, last *credentials) (*credentials, error) {
	if last != nil {
		yes, err := f.confirm(fmt.Sprintf("  Use the same as %s (%s)? [Y/n] ", last.registry, last.username), true)
		if err != nil {
			return nil, err
		}
		if yes {
			ok, err := f.verify(name, last.username, last.password)
			if err != nil {
				return nil, err
			}
			if ok {
				return &credentials{name, last.username, last.password}, nil
			}
		}
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		user, err := f.term.ReadLine("  Username: ")
		if err != nil {
			return nil, err
		}
		pass, err := f.term.ReadPassword("  Password: ")
		if err != nil {
			return nil, err
		}
		if user == "" || pass == "" {
			fmt.Fprintln(f.out, "  Username and password are required.")
			continue
		}
		ok, err := f.verify(name, user, pass)
		if err != nil {
			return nil, err
		}
		if ok {
			return &credentials{name, user, pass}, nil
		}
	}
	return nil, fmt.Errorf("the credentials for %s were rejected %d times", name, maxAttempts)
}

// verify reports whether to keep the credentials: true when accepted, when
// the registry needs no login, or when the user continues without a check.
func (f *credentialFlow) verify(name, user, pass string) (bool, error) {
	if f.check == nil {
		return true, nil
	}
	res, err := f.check(name, user, pass)
	if err != nil {
		fmt.Fprintf(f.out, "  Could not check the credentials: %v\n", err)
		yes, cerr := f.confirm("  Continue without checking? [y/N] ", false)
		if cerr != nil {
			return false, cerr
		}
		if !yes {
			return false, fmt.Errorf("could not check the credentials for %s", name)
		}
		return true, nil
	}
	switch res.Status {
	case regauth.OK:
		fmt.Fprintln(f.out, "  Credentials accepted.")
		return true, nil
	case regauth.NoAuth:
		fmt.Fprintf(f.out, "%s does not require a login; credentials not checked\n", name)
		return true, nil
	}
	fmt.Fprintf(f.out, "  Rejected by %s (%s).\n", name, res.Detail)
	return false, nil
}

func (f *credentialFlow) confirm(prompt string, def bool) (bool, error) {
	answer, err := f.term.ReadLine(prompt)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	return def, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/credentials.go internal/cmd/credentials_test.go
git commit -m "feat: check file credentials and prompt for auth: prompt with reuse and retries"
```

---

### Task 4: Wire into `create`, terminal, e2e, README

**Files:**
- Create: `internal/cmd/tty.go`, `test/e2e/registry-auth.bats`
- Modify: `internal/cmd/create.go`, `internal/cmd/registries.go`, `internal/cmd/registries_test.go`, `go.mod`, `go.sum`, `README.md`

**Interfaces:**
- Consumes: `credentialFlow`, `terminal`, `checkFunc` (Task 3); `config.AuthEndpoint`, `PullScopes`, `PromptRegistries` (Task 1); `regauth.Check`, `regauth.Endpoint` (Task 2).

- [ ] **Step 1: Dependency**

Run: `go get golang.org/x/term@v0.40.0 && go mod tidy`
Expected: `go.mod` requires `golang.org/x/term v0.40.0` and `golang.org/x/sys v0.41.0`; the `go` line stays `go 1.24.13` (if `go get` tries to raise it or pulls a newer x/sys, stop and report BLOCKED).

- [ ] **Step 2: Guard test** — append to `internal/cmd/registries_test.go`:

```go
func TestPrepareKindConfig_RefusesUnaskedPrompt(t *testing.T) {
	cfg := &config.Config{Registries: map[string]*config.Registry{
		"a.example.com": {Auth: &config.RegistryAuth{Prompt: true}},
	}}
	_, _, err := prepareKindConfig(writeTestKindConfig(t), cfg, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "a.example.com") {
		t.Fatalf("got %v", err)
	}
}
```

(`writeTestKindConfig` is the existing helper in `registries_test.go`.)

Run: `go test ./internal/cmd/ -run TestPrepareKindConfig_RefusesUnaskedPrompt -v` → FAIL (no error returned).

- [ ] **Step 3: Implement the guard** — at the top of `prepareKindConfig` in `internal/cmd/registries.go`:

```go
	if regs := cfg.PromptRegistries(); len(regs) > 0 {
		return "", noop, fmt.Errorf("credentials for %s were not entered", strings.Join(regs, ", "))
	}
```

(after `noop := func() {}`). Re-run the test → PASS.

- [ ] **Step 4: Terminal** — `internal/cmd/tty.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ttyTerminal asks on /dev/tty, so prompts work when stdin or stdout is
// redirected.
type ttyTerminal struct {
	f *os.File
}

func openTerminal() (*ttyTerminal, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &ttyTerminal{f: f}, nil
}

func (t *ttyTerminal) Close() error { return t.f.Close() }

// ReadLine reads one byte at a time so nothing typed ahead is buffered away
// from a following ReadPassword.
func (t *ttyTerminal) ReadLine(prompt string) (string, error) {
	fmt.Fprint(t.f, prompt)
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := t.f.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			if b.Len() > 0 {
				break
			}
			return "", err
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func (t *ttyTerminal) ReadPassword(prompt string) (string, error) {
	fmt.Fprint(t.f, prompt)
	p, err := term.ReadPassword(int(t.f.Fd()))
	fmt.Fprintln(t.f)
	return strings.TrimSpace(string(p)), err
}
```

- [ ] **Step 5: Wire into `create`** — in `internal/cmd/create.go`:
  - Add `var skipAuthCheck bool` next to `configFile`, and in `init()`: `createCmd.Flags().BoolVar(&skipAuthCheck, "skip-auth-check", false, "do not check registry credentials against the registries")`.
  - After the warnings loop and before `fmt.Printf("Creating cluster(s) …")`, add:

```go
		flow := &credentialFlow{cfg: res.Config, out: os.Stderr}
		if !skipAuthCheck {
			flow.check = func(reg, user, pass string) (regauth.Result, error) {
				ep := res.Config.AuthEndpoint(reg)
				return regauth.Check(cmd.Context(), regauth.Endpoint{URL: ep.URL, SkipVerify: ep.SkipVerify},
					user, pass, res.Config.PullScopes(reg))
			}
		}
		if len(res.Config.PromptRegistries()) > 0 {
			if t, err := openTerminal(); err == nil {
				defer t.Close()
				flow.term = t
			}
		}
		if err := flow.run(); err != nil {
			return err
		}
```

  (import `github.com/rophy/kindtks/internal/regauth`). `cmd.Context()` is non-nil under cobra's `Execute`; if a test calls `RunE` directly and it is nil, use `context.Background()` as a fallback.

- [ ] **Step 6: Run unit tests**

Run: `go vet ./... && go test ./... && bats test/unit/`
Expected: PASS.

- [ ] **Step 7: E2e** — `test/e2e/registry-auth.bats`:

```bash
#!/usr/bin/env bats
# Registry credentials end to end against a local registry:2 with htpasswd:
# checked before anything is created, and asked for on a terminal with
# `auth: prompt`. The test profile only prints its kind config; no cluster
# is created.

load '../helpers/test_helper'

REG_NAME="kindtks-auth-e2e-reg"
PROFILE="auth-e2e"

setup_file() {
  export E2E_HOME="${BATS_FILE_TMPDIR}/home"
  export KINDTKS_PROFILES_DIR="${BATS_FILE_TMPDIR}/profiles"
  mkdir -p "$E2E_HOME" "$KINDTKS_PROFILES_DIR/$PROFILE" "${BATS_FILE_TMPDIR}/auth"
  export KT="${BATS_FILE_TMPDIR}/kindtks"
  go build -o "$KT" "${BATS_TEST_DIRNAME}/../../"

  docker run --rm --entrypoint htpasswd httpd:2 -Bbn e2euser e2epass > "${BATS_FILE_TMPDIR}/auth/htpasswd"
  docker rm -f "$REG_NAME" >/dev/null 2>&1 || true
  docker run -d --name "$REG_NAME" -p 127.0.0.1::5000 \
    -v "${BATS_FILE_TMPDIR}/auth:/auth:ro" \
    -e REGISTRY_AUTH=htpasswd -e REGISTRY_AUTH_HTPASSWD_REALM=e2e \
    -e REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd registry:2 >/dev/null
  local port
  port=$(docker port "$REG_NAME" 5000/tcp | head -n 1 | cut -d: -f2)
  export REG="127.0.0.1:${port}"
  for _ in $(seq 1 30); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "http://${REG}/v2/")" = 401 ] && break
    sleep 1
  done

  cat > "$KINDTKS_PROFILES_DIR/$PROFILE/install.sh" <<'EOF'
REQUIRES=""
create() { echo "--- kind config"; cat "$KINDTKS_KIND_CONFIG"; }
delete() { :; }
EOF
  printf 'kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnodes:\n- role: control-plane\n' \
    > "$KINDTKS_PROFILES_DIR/$PROFILE/kind-config.yaml"
  printf 'images:\n  probe: %s/probe:v1\n' "$REG" > "$KINDTKS_PROFILES_DIR/$PROFILE/config.yaml"
}

teardown_file() {
  docker rm -f "$REG_NAME" >/dev/null 2>&1 || true
}

kt() {
  HOME="$E2E_HOME" "$KT" "$@"
}

write_config() { # write_config AUTH_YAML
  printf 'registries:\n  "%s":\n    auth: %s\n' "$REG" "$1" > "${BATS_TEST_TMPDIR}/config.yaml"
}

@test "wrong credentials in the file stop create before the profile runs" {
  write_config '{username: e2euser, password: wrong}'
  run kt create "$PROFILE" --config "${BATS_TEST_TMPDIR}/config.yaml"
  assert_failure
  assert_output --partial "rejected by the registry"
  refute_output --partial "--- kind config"
}

@test "right credentials in the file are checked and applied" {
  write_config '{username: e2euser, password: e2epass}'
  run kt create "$PROFILE" --config "${BATS_TEST_TMPDIR}/config.yaml"
  assert_success
  assert_output --partial "configs.\"${REG}\".auth"
  assert_output --partial 'username = "e2euser"'
}

@test "--skip-auth-check creates with unchecked credentials" {
  write_config '{username: e2euser, password: wrong}'
  run kt create "$PROFILE" --config "${BATS_TEST_TMPDIR}/config.yaml" --skip-auth-check
  assert_success
  assert_output --partial "--- kind config"
}

@test "auth: prompt without a terminal fails" {
  write_config prompt
  run setsid -w env HOME="$E2E_HOME" "$KT" create "$PROFILE" --config "${BATS_TEST_TMPDIR}/config.yaml" </dev/null
  assert_failure
  assert_output --partial 'is "prompt", but there is no terminal to ask on'
}

@test "auth: prompt asks on the terminal and asks again after a rejection" {
  write_config prompt
  local cmd="HOME='$E2E_HOME' '$KT' create '$PROFILE' --config '${BATS_TEST_TMPDIR}/config.yaml'"
  run bash -c "{ sleep 1; printf 'e2euser\n'; sleep 1; printf 'wrong\n'; sleep 2; printf 'e2euser\n'; sleep 1; printf 'e2epass\n'; sleep 3; } | script -qec \"$cmd\" /dev/null"
  assert_success
  assert_output --partial "Rejected by ${REG}"
  assert_output --partial 'username = "e2euser"'
  assert_output --partial 'password = "e2epass"'
}
```

Notes: the pseudo-terminal test feeds input with pauses so each line arrives after its prompt; `script -e` returns the command's exit code. If timing or quoting needs adjusting on this host, adjust only the test mechanics (not the assertions) and explain in the report.

Run: `bats test/e2e/registry-auth.bats` (foreground). Expected: 5/5; afterwards `docker ps -a | grep kindtks-auth-e2e` prints nothing.

- [ ] **Step 8: README** — in `README.md`, `### Per-registry settings`, after the "Authentication:" YAML example (the `svc-account` / `secret-token` block), insert:

````markdown
To keep the password out of the file, set `auth: prompt`. `kindtks create` then asks for the username and password on the terminal (from the second such registry on, it offers to reuse the previous one) and fails when there is no terminal:

```yaml
registries:
    registry.example.com:
        auth: prompt
```

`kindtks create` checks every registry's credentials against the registry before creating anything, the way `docker login` does. Prompted credentials that are rejected are asked for again (up to three times); rejected credentials from the file stop `create`. `--skip-auth-check` turns the check off, for example when the host cannot reach a registry that the Kind nodes can.
````

- [ ] **Step 9: Verify and commit**

Run: `go vet ./... && go test ./... && bats test/unit/ && grep -n 'x/term\|x/sys\|^go ' go.mod`
Expected: PASS; `golang.org/x/term v0.40.0`, `golang.org/x/sys v0.41.0`, `go 1.24.13`.

```bash
git add go.mod go.sum internal/cmd/tty.go internal/cmd/create.go internal/cmd/registries.go internal/cmd/registries_test.go test/e2e/registry-auth.bats README.md
git commit -m "feat: ask for auth: prompt credentials on create and check them first"
```

---

### Controller verification after Task 4

- `bats test/e2e/registry-auth.bats`, `bats test/e2e/registry-hosts.bats`, `bats test/unit/`, `go test ./...`.
- Manual: with a real Docker Hub account (user's choice), `auth: prompt` on `docker.io` exercises the token path with correct credentials, which the tests cover only against a local fake.
