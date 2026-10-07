package config

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

var wellKnownRegistries = map[string]bool{
	"docker.io":            true,
	"index.docker.io":      true,
	"registry-1.docker.io": true,
	"quay.io":              true,
	"ghcr.io":              true,
	"gcr.io":               true,
	"registry.k8s.io":      true,
	"k8s.gcr.io":           true,
	"mcr.microsoft.com":    true,
	"public.ecr.aws":       true,
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

// HostsFiles returns the hosts.toml content to write per registry: an
// insecure (HTTP) config for each private registry found in images, replaced
// by the registry's explicit hosts entry when one is set.
func (c *Config) HostsFiles() map[string]string {
	files := make(map[string]string)
	for _, reg := range c.PrivateRegistries() {
		files[reg] = insecureHosts(reg)
	}
	for name, r := range c.Registries {
		if r != nil && r.Hosts != "" {
			files[name] = r.Hosts
		}
	}
	return files
}

func insecureHosts(reg string) string {
	url := "http://" + reg
	return fmt.Sprintf("server = %q\n\n[host.%q]\n  capabilities = [\"pull\", \"resolve\"]\n  skip_verify = true\n", url, url)
}

// AuthRegistries returns the sorted names of registries with credentials.
func (c *Config) AuthRegistries() []string {
	var names []string
	for name, r := range c.Registries {
		if r != nil && r.Auth != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// AuthPatch returns a containerd config patch with credentials for every
// registry that has auth, or "" when there are none.
func (c *Config) AuthPatch() string {
	var b strings.Builder
	for _, name := range c.AuthRegistries() {
		auth := c.Registries[name].Auth
		fmt.Fprintf(&b, "[plugins.\"io.containerd.grpc.v1.cri\".registry.configs.%q.auth]\n", name)
		fmt.Fprintf(&b, "  username = %q\n", auth.Username)
		fmt.Fprintf(&b, "  password = %q\n", auth.Password)
	}
	return b.String()
}

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
	// The registry's own hosts file wins over other registries' mirror entries.
	if _, ok := files[name]; ok {
		regs = append([]string{name}, slices.DeleteFunc(regs, func(r string) bool { return r == name })...)
	}
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
