package config

import (
	"sort"
	"strings"
	"testing"
)

func TestRegistryFromImage(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"nginx:latest", ""},
		{"library/nginx:latest", ""},
		{"docker.io/library/nginx:latest", "docker.io"},
		{"quay.io/cilium/cilium:v1.13.10", "quay.io"},
		{"registry.airgap:5000/quay.io/cilium/cilium:v1.13.10", "registry.airgap:5000"},
		{"myregistry.example.com/app:v1", "myregistry.example.com"},
		{"localhost:5000/myimage:latest", "localhost:5000"},
		{"ghcr.io/owner/repo:sha-abc123", "ghcr.io"},
		{"registry.k8s.io/ingress-nginx/controller:v1.12.3@sha256:abc123", "registry.k8s.io"},
		{"10.0.0.1:5000/myimage:v1", "10.0.0.1:5000"},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := RegistryFromImage(tt.image)
			if got != tt.want {
				t.Errorf("RegistryFromImage(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

func TestPrivateRegistries(t *testing.T) {
	cfg := &Config{Images: map[string]string{
		"cilium":          "registry.airgap:5000/quay.io/cilium/cilium:v1.13.10",
		"cilium-operator": "registry.airgap:5000/quay.io/cilium/operator:v1.13.10",
		"istio-pilot":     "registry.airgap:5000/docker.io/istio/pilot:1.16.7",
		"vault":           "docker.io/hashicorp/vault:2.0.3",
	}}

	regs := cfg.PrivateRegistries()
	if len(regs) != 1 {
		t.Fatalf("expected 1 private registry, got %v", regs)
	}
	if regs[0] != "registry.airgap:5000" {
		t.Errorf("expected registry.airgap:5000, got %q", regs[0])
	}
}

func TestPrivateRegistriesNone(t *testing.T) {
	cfg := &Config{Images: map[string]string{
		"cilium": "quay.io/cilium/cilium:v1.13.10",
		"vault":  "docker.io/hashicorp/vault:2.0.3",
		"vso":    "ghcr.io/ricoberger/vault-secrets-operator:v1.26.0",
	}}

	regs := cfg.PrivateRegistries()
	if len(regs) != 0 {
		t.Errorf("expected no private registries, got %v", regs)
	}
}

func TestPrivateRegistriesMultiple(t *testing.T) {
	cfg := &Config{Images: map[string]string{
		"a": "registry.airgap:5000/foo:v1",
		"b": "corp.example.com/bar:v2",
		"c": "quay.io/public/baz:v3",
	}}

	regs := cfg.PrivateRegistries()
	sort.Strings(regs)
	if len(regs) != 2 {
		t.Fatalf("expected 2 private registries, got %v", regs)
	}
	if regs[0] != "corp.example.com" || regs[1] != "registry.airgap:5000" {
		t.Errorf("unexpected registries: %v", regs)
	}
}

func TestContainerdPatch(t *testing.T) {
	patch := ContainerdPatch([]string{"registry.airgap:5000"}, nil)

	if !strings.Contains(patch, `insecure_skip_verify = true`) {
		t.Error("patch should contain insecure_skip_verify")
	}
	if !strings.Contains(patch, `endpoint = ["http://registry.airgap:5000"]`) {
		t.Error("patch should contain HTTP endpoint")
	}
	if !strings.Contains(patch, `"registry.airgap:5000"`) {
		t.Error("patch should reference the registry")
	}
}

func TestContainerdPatchEmpty(t *testing.T) {
	patch := ContainerdPatch(nil, nil)
	if patch != "" {
		t.Errorf("expected empty patch for nil registries, got %q", patch)
	}
}

func TestContainerdPatchAuth(t *testing.T) {
	auth := map[string]*RegistryAuth{
		"registry.corp.com": {Username: "svc", Password: "secret"},
	}
	patch := ContainerdPatch(nil, auth)

	if !strings.Contains(patch, `[plugins."io.containerd.grpc.v1.cri".registry.configs."registry.corp.com".auth]`) {
		t.Error("patch should contain auth section for registry")
	}
	if !strings.Contains(patch, `username = "svc"`) {
		t.Error("patch should contain username")
	}
	if !strings.Contains(patch, `password = "secret"`) {
		t.Error("patch should contain password")
	}
}

func TestContainerdPatchInsecureAndAuth(t *testing.T) {
	auth := map[string]*RegistryAuth{
		"registry.airgap:5000": {Username: "user", Password: "pass"},
	}
	patch := ContainerdPatch([]string{"registry.airgap:5000"}, auth)

	if !strings.Contains(patch, `insecure_skip_verify = true`) {
		t.Error("patch should contain insecure config")
	}
	if !strings.Contains(patch, `username = "user"`) {
		t.Error("patch should contain auth config")
	}
}

func TestContainerdPatchNilAuthEntry(t *testing.T) {
	auth := map[string]*RegistryAuth{
		"registry.corp.com": nil,
	}
	patch := ContainerdPatch(nil, auth)
	if patch != "" {
		t.Errorf("expected empty patch for nil auth entry, got %q", patch)
	}
}

func TestContainerdPatchEmptyCredentials(t *testing.T) {
	auth := map[string]*RegistryAuth{
		"registry.corp.com": {Username: "", Password: ""},
	}
	patch := ContainerdPatch(nil, auth)

	if !strings.Contains(patch, `username = ""`) {
		t.Error("patch should contain empty username")
	}
	if !strings.Contains(patch, `password = ""`) {
		t.Error("patch should contain empty password")
	}
}

func TestContainerdPatchMultipleAuth(t *testing.T) {
	auth := map[string]*RegistryAuth{
		"registry.a.com":   {Username: "a-user", Password: "a-pass"},
		"registry.b.com:5000": {Username: "b-user", Password: "b-pass"},
	}
	patch := ContainerdPatch(nil, auth)

	if !strings.Contains(patch, `username = "a-user"`) {
		t.Error("patch should contain auth for registry.a.com")
	}
	if !strings.Contains(patch, `username = "b-user"`) {
		t.Error("patch should contain auth for registry.b.com:5000")
	}
}

func TestContainerdPatchSpecialCharsInPassword(t *testing.T) {
	auth := map[string]*RegistryAuth{
		"registry.corp.com": {Username: "svc", Password: `p@ss"w0rd\n`},
	}
	patch := ContainerdPatch(nil, auth)

	if !strings.Contains(patch, `password = "p@ss\"w0rd\\n"`) {
		t.Errorf("patch should properly escape special chars, got:\n%s", patch)
	}
}

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
