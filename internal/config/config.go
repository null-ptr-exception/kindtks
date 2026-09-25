package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type RegistryAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Registry holds per-registry containerd settings.
type Registry struct {
	// Hosts is the raw content of certs.d/<registry>/hosts.toml.
	Hosts string        `yaml:"hosts,omitempty"`
	Auth  *RegistryAuth `yaml:"auth,omitempty"`
}

type Config struct {
	Images     map[string]string    `yaml:"images"`
	Registries map[string]*Registry `yaml:"registries,omitempty"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, nil
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

func Merge(base, override *Config) *Config {
	merged := &Config{
		Images:     make(map[string]string),
		Registries: make(map[string]*Registry),
	}
	for k, v := range base.Images {
		merged.Images[k] = v
	}
	for k, v := range override.Images {
		merged.Images[k] = v
	}
	for _, src := range []*Config{base, override} {
		for name, r := range src.Registries {
			if r == nil {
				continue
			}
			m := merged.Registries[name]
			if m == nil {
				m = &Registry{}
				merged.Registries[name] = m
			}
			if r.Hosts != "" {
				m.Hosts = r.Hosts
			}
			if r.Auth != nil {
				auth := *r.Auth
				m.Auth = &auth
			}
		}
	}
	return merged
}
