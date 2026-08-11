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
	containerChartsDir  = "/charts"
	hostBinDir          = "/.local/bin"
	hostDataDir         = "/.local/share/kindtks"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install kindtks binary and profiles to the host (run inside Docker)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if fi, err := os.Stat(hostBinDir); err != nil || !fi.IsDir() {
			fmt.Println("Error: volume mounts required for install.")
			fmt.Println()
			fmt.Println("Usage:")
			fmt.Println("  docker run --rm \\")
			fmt.Println("    -v \"$HOME/.local/bin:/.local/bin\" \\")
			fmt.Println("    -v \"$HOME/.local/share:/.local/share\" \\")
			fmt.Println("    kindtks install")
			return fmt.Errorf("/.local/bin is not mounted")
		}

		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding own executable: %w", err)
		}

		destBin := filepath.Join(hostBinDir, "kindtks")
		fmt.Printf("Copying kindtks to %s...\n", destBin)
		if err := copyFile(self, destBin, 0755); err != nil {
			return fmt.Errorf("copying binary: %w", err)
		}

		profileDst := filepath.Join(hostDataDir, "profiles")
		fmt.Println("Copying profiles...")
		if err := copyDirRecursive(containerProfileDir, profileDst); err != nil {
			return fmt.Errorf("copying profiles: %w", err)
		}

		chartsDst := filepath.Join(hostDataDir, "charts")
		if _, err := os.Stat(containerChartsDir); err == nil {
			fmt.Println("Copying charts...")
			if err := copyDirRecursive(containerChartsDir, chartsDst); err != nil {
				return fmt.Errorf("copying charts: %w", err)
			}
		}

		fmt.Println()
		fmt.Println("Installation complete!")
		fmt.Println("Run 'kindtks list' to see available profiles.")
		return nil
	},
}

func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		fmt.Printf("  %s\n", rel)
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

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
