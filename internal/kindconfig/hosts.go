package kindconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteHostsDir writes one <registry>/hosts.toml per entry in files under
// dir, removing any stale entries already there. It never removes dir
// itself: running nodes bind-mount it, and replacing the inode would leave
// them looking at an empty directory.
func WriteHostsDir(dir string, files map[string]string) error {
	for reg := range files {
		if err := validateRegistryName(reg); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", dir, err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("clearing %s: %w", dir, err)
		}
	}

	for reg, content := range files {
		regDir := filepath.Join(dir, reg)
		if err := os.MkdirAll(regDir, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(regDir, "hosts.toml"), []byte(content), 0600); err != nil {
			return err
		}
	}
	return nil
}

// validateRegistryName rejects names that are unsafe as a directory name.
// Kept in sync with config.validateRegistryName; duplicated here to avoid an
// import cycle (internal/cmd imports both packages).
func validateRegistryName(name string) error {
	if name == "" || name == "." || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid registry name %q: must be a host or host:port", name)
	}
	return nil
}
