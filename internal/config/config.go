package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

type RegistryAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Config struct {
	Images       map[string]string           `yaml:"images"`
	RegistryAuth map[string]*RegistryAuth    `yaml:"registryAuth,omitempty"`
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

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Merge(base, override *Config) *Config {
	merged := &Config{
		Images:       make(map[string]string),
		RegistryAuth: make(map[string]*RegistryAuth),
	}
	for k, v := range base.Images {
		merged.Images[k] = v
	}
	for k, v := range override.Images {
		merged.Images[k] = v
	}
	for k, v := range base.RegistryAuth {
		merged.RegistryAuth[k] = v
	}
	for k, v := range override.RegistryAuth {
		merged.RegistryAuth[k] = v
	}
	return merged
}
