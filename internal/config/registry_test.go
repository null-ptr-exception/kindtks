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
	patch := ContainerdPatch([]string{"registry.airgap:5000"})

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
	patch := ContainerdPatch(nil)
	if patch != "" {
		t.Errorf("expected empty patch for nil registries, got %q", patch)
	}
}
