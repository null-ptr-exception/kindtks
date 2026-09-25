package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Resolved is the effective configuration for one profile.
type Resolved struct {
	// Config is the typed view used by kindtks itself.
	Config *Config
	// Profile is the merged profiles.<name> section passed to the profile script.
	Profile map[string]any
	// Warnings are non-fatal issues, e.g. sections for profiles not installed.
	Warnings []string
}

// Resolve merges the profile's defaults with the optional user config file,
// validates the result against the common and profile schemas, and returns
// the effective configuration for profile. profilesDir holds the installed
// profiles, one directory each.
func Resolve(profilesDir, profile, userFile string) (*Resolved, error) {
	defaults, err := LoadDocument(filepath.Join(profilesDir, profile, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("loading profile defaults: %w", err)
	}
	merged := map[string]any{"profiles": map[string]any{profile: defaults}}

	if userFile != "" {
		user, err := LoadDocument(userFile)
		if err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
		if err := checkMovedKeys(user, userFile); err != nil {
			return nil, err
		}
		merged = MergeValues(merged, user).(map[string]any)
	}

	if err := ValidateCommon(merged); err != nil {
		return nil, err
	}

	res := &Resolved{}
	sections, _ := merged["profiles"].(map[string]any)
	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dir := filepath.Join(profilesDir, name)
		if _, err := os.Stat(filepath.Join(dir, "install.sh")); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("profiles.%s: profile not installed, ignoring", name))
			continue
		}
		schema, err := ProfileSchema(dir)
		if err != nil {
			return nil, fmt.Errorf("loading profiles.%s schema: %w", name, err)
		}
		if err := ValidateProfile(name, schema, sections[name]); err != nil {
			return nil, err
		}
	}

	res.Profile, _ = sections[profile].(map[string]any)
	if res.Profile == nil {
		res.Profile = map[string]any{}
	}
	cfg, err := typedConfig(merged, res.Profile)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	res.Config = cfg
	return res, nil
}

// checkMovedKeys gives migration hints for keys removed from the top level.
func checkMovedKeys(doc map[string]any, path string) error {
	if _, ok := doc["images"]; ok {
		return fmt.Errorf("%s: top-level `images` moved to `profiles.<profile>.images`; see README", path)
	}
	if _, ok := doc["registryAuth"]; ok {
		return fmt.Errorf("%s: `registryAuth` was replaced by `registries.<registry>.auth`; see README", path)
	}
	return nil
}

// typedConfig decodes the common registries and the active profile's images.
// The document has already passed schema validation.
func typedConfig(doc, section map[string]any) (*Config, error) {
	cfg := &Config{Images: map[string]string{}}
	if regs, ok := doc["registries"]; ok {
		data, err := json.Marshal(regs)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &cfg.Registries); err != nil {
			return nil, fmt.Errorf("decoding registries: %w", err)
		}
	}
	if imgs, ok := section["images"].(map[string]any); ok {
		for k, v := range imgs {
			s, _ := v.(string)
			cfg.Images[k] = s
		}
	}
	return cfg, nil
}
