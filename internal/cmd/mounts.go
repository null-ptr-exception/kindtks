package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runHint points at the full docker run command, printed by running the
// image without arguments.
func runHint() string {
	return fmt.Sprintf("Run `docker run --rm %s` for the full command.", os.Getenv("KINDTKS_IMAGE"))
}

// checkMounts reports what the docker run command is missing. It only
// applies inside the image (KINDTKS_IMAGE set).
func checkMounts(dockerOK func() error) error {
	if os.Getenv("KINDTKS_IMAGE") == "" {
		return nil
	}
	var problems []string
	home := os.Getenv("HOME")
	if !filepath.IsAbs(home) || home == "/" {
		problems = append(problems, `HOME is not your host home directory; pass -e HOME="$HOME"`)
	} else {
		for _, dir := range []string{filepath.Join(home, ".kube"), stateDir()} {
			if err := writableDir(dir); err != nil {
				problems = append(problems, fmt.Sprintf("%s is not mounted writable at the same path (create it on the host first): %v", dir, err))
			}
		}
	}
	if err := dockerOK(); err != nil {
		problems = append(problems, fmt.Sprintf("docker socket is not usable (mount /var/run/docker.sock, add its group with --group-add): %v", err))
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s\n\n%s", strings.Join(problems, "\n"), runHint())
}

func writableDir(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errors.New("not a directory")
	}
	f, err := os.CreateTemp(dir, ".kindtks-check-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

func dockerReachable() error {
	var stderr bytes.Buffer
	c := exec.Command("docker", "version")
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// checkConfigVisible explains a --config file missing inside the image,
// which almost always means its directory is not mounted.
func checkConfigVisible(path string) error {
	if path == "" || os.Getenv("KINDTKS_IMAGE") == "" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config file %s not found inside the container; mount its directory (the documented command mounts $PWD)\n\n%s", path, runHint())
	}
	return nil
}
