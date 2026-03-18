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
		if err := validateMachineName(name); err != nil {
			return err
		}
		repo, _ := cmd.Flags().GetString("repo")
		branch, _ := cmd.Flags().GetString("branch")

		projects, err := config.LoadProjects(cfg.ProjectsFile)
		if err != nil {
			return fmt.Errorf("loading projects: %w", err)
		}
		if _, exists := projects[name]; exists {
			return fmt.Errorf("project %q already registered; use update-project to rebuild", name)
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
		defer cancel()

		localPath := filepath.Join("/var/lib/podspawn/projects", name)
		if err := podfile.CloneRepo(ctx, repo, localPath, branch); err != nil {
			os.RemoveAll(localPath) //nolint:errcheck
			return fmt.Errorf("cloning %s: %w", repo, err)
		}

		raw, err := podfile.FindAndRead(localPath)
		if err != nil {
			os.RemoveAll(localPath) //nolint:errcheck
			return fmt.Errorf("no podfile found in %s: %w", localPath, err)
		}

		pf, err := podfile.Parse(bytes.NewReader(raw))
		if err != nil {
			os.RemoveAll(localPath) //nolint:errcheck
			return fmt.Errorf("parsing podfile for %s: %w", name, err)
		}

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			os.RemoveAll(localPath) //nolint:errcheck
			return fmt.Errorf("connecting to docker: %w", err)
		}

		tag, err := podfile.BuildImageFromPodfile(ctx, rt, pf, raw, name)
		if err != nil {
			os.RemoveAll(localPath) //nolint:errcheck
			return fmt.Errorf("building image for %s: %w", name, err)
		}

		projects[name] = config.ProjectConfig{
			Repo:        repo,
			LocalPath:   localPath,
			PodfileHash: podfile.ComputeTag(name, raw),
			ImageTag:    tag,
		}
		if err := config.SaveProjects(cfg.ProjectsFile, projects); err != nil {
			return fmt.Errorf("saving projects: %w", err)
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
