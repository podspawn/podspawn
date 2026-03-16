package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var listUsersCmd = &cobra.Command{
	Use:   "list-users",
	Short: "List registered container users",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := os.ReadDir(cfg.Auth.KeyDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No users registered.")
				return nil
			}
			return fmt.Errorf("reading key directory: %w", err)
		}

		var users []string
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			users = append(users, entry.Name())
		}
		sort.Strings(users)

		if len(users) == 0 {
			fmt.Println("No users registered.")
			return nil
		}

		keyDir := cfg.Auth.KeyDir
		for _, user := range users {
			data, err := os.ReadFile(filepath.Join(keyDir, user))
			if err != nil {
				slog.Warn("could not read key file", "user", user, "error", err)
				fmt.Printf("%s  (unreadable)\n", user)
				continue
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			keyCount := 0
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					keyCount++
				}
			}
			fmt.Printf("%s  (%d key%s)\n", user, keyCount, pluralS(keyCount))
		}
		return nil
	},
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func init() {
	rootCmd.AddCommand(listUsersCmd)
}
