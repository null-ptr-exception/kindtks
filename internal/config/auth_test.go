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
