package kindconfig

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteHostsDir replaces dir with one <registry>/hosts.toml per entry in files.
func WriteHostsDir(dir string, files map[string]string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clearing %s: %w", dir, err)
	}
	for reg, content := range files {
		regDir := filepath.Join(dir, reg)
		if err := os.MkdirAll(regDir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(regDir, "hosts.toml"), []byte(content), 0644); err != nil {
			return err
		}
	}
	return nil
}
