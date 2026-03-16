package cmd

import (
	"fmt"
	"os"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/spf13/cobra"
)

var removeProjectCmd = &cobra.Command{
	Use:   "remove-project <name>",
	Short: "Deregister a project and remove its local clone",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		projects, err := config.LoadProjects(cfg.ProjectsFile)
		if err != nil {
			return fmt.Errorf("loading projects: %w", err)
		}

		proj, exists := projects[name]
		if !exists {
			return fmt.Errorf("project %q is not registered", name)
		}

		if proj.LocalPath != "" {
			if err := os.RemoveAll(proj.LocalPath); err != nil {
				return fmt.Errorf("removing project directory %s: %w", proj.LocalPath, err)
			}
		}

		delete(projects, name)
		if err := config.SaveProjects(cfg.ProjectsFile, projects); err != nil {
			return fmt.Errorf("saving projects: %w", err)
		}

		fmt.Fprintf(os.Stderr, "removed project %s\n", name)
		if proj.ImageTag != "" {
			fmt.Fprintf(os.Stderr, "note: image %s still exists in Docker; run 'docker rmi %s' to remove it\n", proj.ImageTag, proj.ImageTag)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(removeProjectCmd)
}
