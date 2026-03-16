package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update podspawn to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		check, _ := cmd.Flags().GetBool("check")

		latest, err := fetchLatestVersion()
		if err != nil {
			return fmt.Errorf("checking for updates: %w", err)
		}

		current := Version
		if current == latest || "v"+current == latest {
			fmt.Printf("already on latest version (%s)\n", current)
			return nil
		}

		fmt.Printf("current: %s, latest: %s\n", current, latest)

		if check {
			return nil
		}

		goos := runtime.GOOS
		goarch := runtime.GOARCH
		filename := fmt.Sprintf("podspawn_%s_%s_%s.tar.gz", strings.TrimPrefix(latest, "v"), goos, goarch)
		url := fmt.Sprintf("https://github.com/podspawn/podspawn/releases/download/%s/%s", latest, filename)

		fmt.Printf("downloading %s...\n", latest)

		tmpDir, err := os.MkdirTemp("", "podspawn-update-")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		tarPath := filepath.Join(tmpDir, "podspawn.tar.gz")
		if err := downloadFile(url, tarPath); err != nil {
			return fmt.Errorf("downloading %s: %w", latest, err)
		}

		tarCmd := exec.Command("tar", "-xzf", tarPath, "-C", tmpDir)
		if err := tarCmd.Run(); err != nil {
			return fmt.Errorf("extracting: %w", err)
		}

		newBinary := filepath.Join(tmpDir, "podspawn")
		currentBinary, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding current binary: %w", err)
		}
		currentBinary, err = filepath.EvalSymlinks(currentBinary)
		if err != nil {
			return fmt.Errorf("resolving binary path: %w", err)
		}

		// Atomic replace: rename new over old
		if err := os.Rename(newBinary, currentBinary); err != nil {
			// Cross-device rename; copy instead
			if err := copyFile(newBinary, currentBinary); err != nil {
				return fmt.Errorf("replacing binary: %w", err)
			}
		}

		fmt.Printf("updated to %s\n", latest)
		return nil
	},
}

func init() {
	updateCmd.Flags().Bool("check", false, "Only check for updates, don't download")
	rootCmd.AddCommand(updateCmd)
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/podspawn/podspawn/releases/latest")
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = io.Copy(f, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}
