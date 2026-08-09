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
	Path     string
	Requires []string
}

func Load(profileDir string, name string) (*Profile, error) {
	path := filepath.Join(profileDir, name+".sh")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("profile %q not found: %w", name, err)
	}

	requires, err := parseRequires(path)
	if err != nil {
		return nil, fmt.Errorf("parsing profile %q: %w", name, err)
	}

	return &Profile{
		Name:     name,
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".sh"))
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
