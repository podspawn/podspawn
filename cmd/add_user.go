package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/adduser"
	"github.com/spf13/cobra"
)

var addUserCmd = &cobra.Command{
	Use:   "add-user <username>",
	Short: "Register an SSH key for a container user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		username := args[0]
		if err := adduser.ValidateUsername(username); err != nil {
			return err
		}

		keyStrings, _ := cmd.Flags().GetStringSlice("key")
		keyFiles, _ := cmd.Flags().GetStringSlice("key-file")
		github, _ := cmd.Flags().GetString("github")

		if len(keyStrings) == 0 && len(keyFiles) == 0 && github == "" {
			return fmt.Errorf("at least one of --key, --key-file, or --github is required")
		}

		var keys []string
		keys = append(keys, keyStrings...)

		for _, path := range keyFiles {
			fileKeys, err := adduser.ReadKeyFile(path)
			if err != nil {
				return fmt.Errorf("reading %s: %w", path, err)
			}
			if len(fileKeys) == 0 {
				return fmt.Errorf("no valid SSH keys found in %s", path)
			}
			keys = append(keys, fileKeys...)
		}

		if github != "" {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			ghKeys, err := adduser.FetchGitHubKeys(ctx, http.DefaultClient, github)
			if err != nil {
				return fmt.Errorf("fetching GitHub keys: %w", err)
			}
			if len(ghKeys) == 0 {
				return fmt.Errorf("no SSH keys found for GitHub user %q", github)
			}
			keys = append(keys, ghKeys...)
		}

		keyDir := cfg.Auth.KeyDir
		n, err := adduser.WriteKeys(keyDir, username, keys)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "added %d key(s) for %s\n", n, username)
		return nil
	},
}

func init() {
	addUserCmd.Flags().StringSlice("key", nil, "SSH public key string (repeatable)")
	addUserCmd.Flags().StringSlice("key-file", nil, "path to SSH public key file (repeatable)")
	addUserCmd.Flags().String("github", "", "GitHub username to import keys from")
	rootCmd.AddCommand(addUserCmd)
}
