package profile

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Profile struct {
	Name     string
	Dir      string
	Path     string
	Requires []string
}

func Load(profileDir string, name string) (*Profile, error) {
	dir := filepath.Join(profileDir, name)
	path := filepath.Join(dir, "install.sh")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("profile %q not found (expected %s): %w", name, path, err)
	}

	requires, err := parseRequires(path)
	if err != nil {
		return nil, fmt.Errorf("parsing profile %q: %w", name, err)
	}

	return &Profile{
		Name:     name,
		Dir:      dir,
		Path:     path,
		Requires: requires,
	}, nil
}

func ListAll(profileDir string) ([]string, error) {
	entries, err := os.ReadDir(profileDir)
	if err != nil {
		return nil, fmt.Errorf("reading profile directory: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		installSh := filepath.Join(profileDir, e.Name(), "install.sh")
		if _, err := os.Stat(installSh); err == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func parseRequires(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "REQUIRES=") {
			value := strings.TrimPrefix(line, "REQUIRES=")
			value = strings.Trim(value, `"'`)
			if value == "" {
				return nil, nil
			}
			return strings.Fields(value), nil
		}
	}
	return nil, nil
}
