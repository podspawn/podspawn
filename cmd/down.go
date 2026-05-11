package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/session"
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
	clean, _ := cmd.Flags().GetBool("clean")

	sessionName := nameOverride
	if sessionName == "" {
		pfName := ""
		if raw, findErr := podfile.FindAndRead(cwd); findErr == nil {
			if rawPf, parseErr := podfile.ParseRaw(bytes.NewReader(raw)); parseErr == nil {
				pfName = rawPf.Name
			}
		}
		sessionName = podfile.SessionName(cwd, pfName)
	}

	ls, err := buildLocalSession(sessionName)
	if err != nil {
		return err
	}
	defer ls.Close()

	if err := destroySessionByName(ls, sessionName); err != nil {
		return err
	}

	if clean {
		removeServiceVolumes(ls, cwd)
	}
	return nil
}

func removeServiceVolumes(ls *localSession, podfileDir string) {
	raw, err := podfile.FindAndRead(podfileDir)
	if err != nil {
		return
	}
	rawPf, err := podfile.ParseRaw(bytes.NewReader(raw))
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for _, svc := range rawPf.Services {
		for _, vol := range svc.Volumes {
			// Named volumes are "name:/path"; bind mounts start with "/".
			parts := strings.SplitN(vol, ":", 2)
			if len(parts) != 2 || strings.HasPrefix(parts[0], "/") {
				continue
			}
			if err := ls.Session.Runtime.RemoveVolume(ctx, parts[0]); err != nil {
				slog.Warn("failed to remove volume", "volume", parts[0], "error", err)
			} else {
				fmt.Fprintf(os.Stderr, "Removed volume %s\n", parts[0])
			}
		}
	}
}

// destroySessionByName resolves a name to its (user, project) tuple and
// ends the live session. Used by `podspawn down` and by
// `podspawn dev --fresh`. The name is historical (predates the Stage 5
// session plane); functionally it delegates to Service.End.
func destroySessionByName(ls *localSession, sessionName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := ls.Service.End(ctx, session.Ref{
		User: ls.Session.Username,
		Name: sessionName,
	})
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			return fmt.Errorf("no active session %q for user %s", sessionName, ls.Session.Username)
		}
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
