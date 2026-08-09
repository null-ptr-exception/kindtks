package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	containerProfileDir = "/profiles"
	hostBinDir          = "/.local/bin"
	hostProfileDir      = "/.local/share/kindtks/profiles"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install kindtks binary and profiles to the host (run inside Docker)",
	RunE: func(cmd *cobra.Command, args []string) error {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding own executable: %w", err)
		}

		destBin := filepath.Join(hostBinDir, "kindtks")
		fmt.Printf("Copying kindtks to %s...\n", destBin)
		if err := copyFile(self, destBin, 0755); err != nil {
			return fmt.Errorf("copying binary: %w", err)
		}

		if err := os.MkdirAll(hostProfileDir, 0755); err != nil {
			return fmt.Errorf("creating profile directory: %w", err)
		}

		entries, err := os.ReadDir(containerProfileDir)
		if err != nil {
			return fmt.Errorf("reading profiles from %s: %w", containerProfileDir, err)
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			src := filepath.Join(containerProfileDir, e.Name())
			dst := filepath.Join(hostProfileDir, e.Name())
			fmt.Printf("Copying profile %s...\n", e.Name())
			if err := copyFile(src, dst, 0644); err != nil {
				return fmt.Errorf("copying profile %s: %w", e.Name(), err)
			}
		}

		fmt.Println()
		fmt.Println("Installation complete!")
		fmt.Println("Run 'kindtks list' to see available profiles.")
		return nil
	},
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func init() {
	rootCmd.AddCommand(installCmd)
}
