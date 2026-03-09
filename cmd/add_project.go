package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/spf13/cobra"
)

var addProjectCmd = &cobra.Command{
	Use:   "add-project <name>",
	Short: "Register a project and pre-build its Podfile image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		repo, _ := cmd.Flags().GetString("repo")
		branch, _ := cmd.Flags().GetString("branch")

		projects, err := config.LoadProjects(cfg.ProjectsFile)
		if err != nil {
			return fmt.Errorf("loading projects: %w", err)
		}
		if _, exists := projects[name]; exists {
			return fmt.Errorf("project %q already registered; use update-project to rebuild", name)
		}

		localPath := filepath.Join("/var/lib/podspawn/projects", name)
		if err := podfile.CloneRepo(repo, localPath, branch); err != nil {
			return err
		}

		raw, err := podfile.FindAndRead(localPath)
		if err != nil {
			os.RemoveAll(localPath) //nolint:errcheck
			return fmt.Errorf("no podfile found in %s: %w", repo, err)
		}

		pf, err := podfile.Parse(bytes.NewReader(raw))
		if err != nil {
			os.RemoveAll(localPath) //nolint:errcheck
			return err
		}

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
		defer cancel()

		tag, err := podfile.BuildImageFromPodfile(ctx, rt, pf, raw, name)
		if err != nil {
			os.RemoveAll(localPath) //nolint:errcheck
			return err
		}

		projects[name] = config.ProjectConfig{
			Repo:        repo,
			LocalPath:   localPath,
			PodfileHash: podfile.ComputeTag(name, raw),
			ImageTag:    tag,
		}
		if err := config.SaveProjects(cfg.ProjectsFile, projects); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "project %s registered, image: %s\n", name, tag)
		return nil
	},
}

func init() {
	addProjectCmd.Flags().String("repo", "", "git repository URL")
	addProjectCmd.Flags().String("branch", "", "git branch (default: repo default)")
	_ = addProjectCmd.MarkFlagRequired("repo")
	rootCmd.AddCommand(addProjectCmd)
}
