package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

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

// Validate checks registry names and that hosts entries are valid TOML.
func (c *Config) Validate() error {
	for name, r := range c.Registries {
		if err := validateRegistryName(name); err != nil {
			return err
		}
		if r == nil || r.Hosts == "" {
			continue
		}
		var v map[string]any
		if err := toml.Unmarshal([]byte(r.Hosts), &v); err != nil {
			return fmt.Errorf("registries.%s.hosts is not valid TOML: %w", name, err)
		}
	}
	return nil
}

// validateRegistryName rejects names that are unsafe as a directory name.
func validateRegistryName(name string) error {
	if name == "" || name == "." || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid registry name %q: must be a host or host:port", name)
	}
	return nil
}
