package config

import (
	"fmt"
	"net"
	"strings"
)

var wellKnownRegistries = map[string]bool{
	"docker.io":       true,
	"index.docker.io": true,
	"registry-1.docker.io": true,
	"quay.io":         true,
	"ghcr.io":         true,
	"gcr.io":          true,
	"registry.k8s.io": true,
	"k8s.gcr.io":      true,
	"mcr.microsoft.com": true,
	"public.ecr.aws":  true,
}

// RegistryFromImage extracts the registry host (with optional port) from an
// image reference. Returns empty string for Docker Hub implicit references
// (e.g. "nginx:latest").
func RegistryFromImage(image string) string {
	// Strip tag or digest
	ref := image
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon >= 0 {
		// Only strip if the part after colon looks like a tag (no slashes)
		after := ref[colon+1:]
		if !strings.Contains(after, "/") {
			ref = ref[:colon]
		}
	}

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 1 {
		return ""
	}

	first := parts[0]
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		return first
	}
	return ""
}

// PrivateRegistries returns the set of unique non-well-known registries found
// in the config's image references.
func (c *Config) PrivateRegistries() []string {
	seen := make(map[string]bool)
	var result []string

	for _, image := range c.Images {
		reg := RegistryFromImage(image)
		if reg == "" {
			continue
		}
		host := reg
		if h, _, err := net.SplitHostPort(reg); err == nil {
			host = h
		}
		// Check both the bare host and the full host:port
		if wellKnownRegistries[host] || wellKnownRegistries[reg] {
			continue
		}
		if !seen[reg] {
			seen[reg] = true
			result = append(result, reg)
		}
	}
	return result
}

// ContainerdPatch generates a containerd config TOML patch that configures
// insecure (HTTP) access for the given registries and authentication for
// registries with credentials.
func ContainerdPatch(registries []string, auth map[string]*RegistryAuth) string {
	if len(registries) == 0 && len(auth) == 0 {
		return ""
	}

	insecureSet := make(map[string]bool)
	var b strings.Builder
	for _, reg := range registries {
		insecureSet[reg] = true
		fmt.Fprintf(&b, "[plugins.\"io.containerd.grpc.v1.cri\".registry.configs.%q.tls]\n", reg)
		fmt.Fprintf(&b, "  insecure_skip_verify = true\n")
		fmt.Fprintf(&b, "[plugins.\"io.containerd.grpc.v1.cri\".registry.mirrors.%q]\n", reg)
		fmt.Fprintf(&b, "  endpoint = [\"http://%s\"]\n", reg)
	}
	for reg, cred := range auth {
		if cred == nil {
			continue
		}
		fmt.Fprintf(&b, "[plugins.\"io.containerd.grpc.v1.cri\".registry.configs.%q.auth]\n", reg)
		fmt.Fprintf(&b, "  username = %q\n", cred.Username)
		fmt.Fprintf(&b, "  password = %q\n", cred.Password)
	}
	return b.String()
}
