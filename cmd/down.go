package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/cleanup"
	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the dev environment for the current directory",
	Long: `Stop the container and companion services started by podspawn dev.

  podspawn down          -> stop, keep volumes
  podspawn down --clean  -> stop and remove volumes`,
	RunE: runDown,
}

func runDown(cmd *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	nameOverride, _ := cmd.Flags().GetString("name")

	sessionName := nameOverride
	if sessionName == "" {
		// Try to read Podfile for name field
		pfName := ""
		if raw, findErr := podfile.FindAndRead(cwd); findErr == nil {
			if pf, parseErr := podfile.Parse(bytes.NewReader(raw)); parseErr == nil {
				pfName = pf.Name
			}
		}
		sessionName = podfile.SessionName(cwd, pfName)
	}

	ls, err := buildLocalSession(sessionName)
	if err != nil {
		return err
	}
	defer ls.Close()

	return destroySessionByName(ls, sessionName)
}

func destroySessionByName(ls *localSession, sessionName string) error {
	sess, err := ls.Store.GetSession(ls.Session.Username, sessionName)
	if err != nil {
		return fmt.Errorf("looking up session: %w", err)
	}
	if sess == nil {
		return fmt.Errorf("no active session %q for user %s", sessionName, ls.Session.Username)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cleanup.DestroySession(ctx, ls.Session.Runtime, ls.Store, sess); err != nil {
		return fmt.Errorf("stopping session: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Stopped %s\n", sessionName)
	return nil
}

func init() {
	downCmd.Flags().Bool("clean", false, "also remove named volumes")
	downCmd.Flags().String("name", "", "target specific session")
	rootCmd.AddCommand(downCmd)
}
