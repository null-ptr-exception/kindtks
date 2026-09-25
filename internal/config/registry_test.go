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
