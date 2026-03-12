package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/spf13/cobra"
)

var updateProjectCmd = &cobra.Command{
	Use:   "update-project <name>",
	Short: "Pull latest repo and rebuild Podfile image if changed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		projects, err := config.LoadProjects(cfg.ProjectsFile)
		if err != nil {
			return fmt.Errorf("loading projects: %w", err)
		}
		proj, exists := projects[name]
		if !exists {
			return fmt.Errorf("project %q not registered; use add-project first", name)
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
		defer cancel()

		if err := podfile.PullRepo(ctx, proj.LocalPath); err != nil {
			return err
		}

		raw, err := podfile.FindAndRead(proj.LocalPath)
		if err != nil {
			return err
		}

		newHash := podfile.ComputeTag(name, raw)
		if newHash == proj.PodfileHash {
			fmt.Fprintf(os.Stderr, "project %s: podfile unchanged, skipping rebuild\n", name)
			return nil
		}

		pf, err := podfile.Parse(bytes.NewReader(raw))
		if err != nil {
			return err
		}

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			return err
		}

		tag, err := podfile.BuildImageFromPodfile(ctx, rt, pf, raw, name)
		if err != nil {
			return err
		}

		proj.PodfileHash = newHash
		proj.ImageTag = tag
		projects[name] = proj
		if err := config.SaveProjects(cfg.ProjectsFile, projects); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "project %s rebuilt, image: %s\n", name, tag)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateProjectCmd)
}
