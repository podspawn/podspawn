package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/podspawn/podspawn/internal/spawn"
	"github.com/podspawn/podspawn/internal/ui"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a named machine",
	Long: `Create a persistent container that you can attach to later with podspawn shell.

  podspawn create mydev                    # default image
  podspawn create mydev --image alpine:3.20  # custom image`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		image, _ := cmd.Flags().GetString("image")
		projectName, _ := cmd.Flags().GetString("project")
		branch, _ := cmd.Flags().GetString("branch")
		if branch != "" && projectName == "" {
			return errors.New("--branch requires --project")
		}

		ls, err := buildLocalSession(name)
		if err != nil {
			return err
		}
		defer ls.Close()

		if image != "" {
			ls.Session.Image = image
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		createdMachine, err := setupNamedMachine(ctx, ls, name, projectName, branch)
		if err != nil {
			return err
		}

		spin := ui.NewSpinner("Creating %s (%s)", name, ls.Session.Image)

		_, err = ls.Session.Ensure(ctx)
		if err != nil {
			spin.Fail()
			var hookErr *spawn.HookError
			if createdMachine && !errors.As(err, &hookErr) {
				cleanupNewMachineOnFailure(ls)
			}
			return wrapCreateEnsureError(name, ls.Session.WorkspacePath, err)
		}

		spin.Stop()
		return nil
	},
}

func init() {
	createCmd.Flags().String("image", "", "base image (default from config)")
	createCmd.Flags().String("project", "", "registered project name")
	createCmd.Flags().String("branch", "", "git branch for project-backed machines")
	rootCmd.AddCommand(createCmd)
}

func wrapCreateEnsureError(name, workspacePath string, err error) error {
	var hookErr *spawn.HookError
	if !errors.As(err, &hookErr) || hookErr.Hook != "on_create" || workspacePath == "" {
		return err
	}

	return fmt.Errorf("%w; workspace preserved at %q; rerun \"podspawn create %s\" to retry, or \"podspawn rm %s\" to discard", err, workspacePath, name, name)
}
