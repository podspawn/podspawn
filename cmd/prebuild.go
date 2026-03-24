package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/podspawn/podspawn/internal/ui"
	"github.com/spf13/cobra"
)

var prebuildCmd = &cobra.Command{
	Use:   "prebuild",
	Short: "Pre-build a container image from a Podfile",
	Long: `Build the Docker image defined by a Podfile without starting a container.
Use this in CI or locally to cache the image so podspawn dev starts instantly.

  podspawn prebuild                     -> build from CWD podfile
  podspawn prebuild --podfile path.yaml -> build from specific file
  podspawn prebuild --tag myimg:latest  -> custom image tag`,
	RunE: runPrebuild,
}

func runPrebuild(cmd *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	podfilePath, _ := cmd.Flags().GetString("podfile")
	tag, _ := cmd.Flags().GetString("tag")

	podfileDir := cwd
	if podfilePath != "" {
		podfileDir = filepath.Dir(podfilePath)
	}

	raw, err := podfile.FindAndRead(podfileDir)
	if err != nil {
		return fmt.Errorf("no podfile found in %s", podfileDir)
	}

	rawPf, err := podfile.ParseRaw(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parsing podfile: %w", err)
	}

	pf, err := podfile.ResolveExtends(rawPf, podfileDir)
	if err != nil {
		return fmt.Errorf("resolving extends: %w", err)
	}

	rt, err := runtime.NewDockerRuntime()
	if err != nil {
		return fmt.Errorf("connecting to docker: %w", err)
	}

	// Compute image tag
	projectName := podfile.SessionName(cwd, pf.Name)
	hashInput := raw
	if rawPf.Extends != "" {
		mergedRaw, marshalErr := podfile.MarshalCanonical(pf)
		if marshalErr != nil {
			return fmt.Errorf("marshaling podfile: %w", marshalErr)
		}
		hashInput = mergedRaw
	}

	imageTag := tag
	if imageTag == "" {
		imageTag = podfile.ComputeTag(projectName, hashInput)
	}

	spin := ui.NewSpinner("Building %s", imageTag)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if _, err := podfile.BuildImageFromPodfile(ctx, rt, pf, hashInput, projectName); err != nil {
		spin.Fail()
		return fmt.Errorf("building image: %w", err)
	}

	// If custom tag requested, also tag with that name
	if tag != "" && tag != podfile.ComputeTag(projectName, hashInput) {
		defaultTag := podfile.ComputeTag(projectName, hashInput)
		if tagErr := rt.TagImage(ctx, defaultTag, tag); tagErr != nil {
			spin.Fail()
			return fmt.Errorf("tagging image as %s: %w", tag, tagErr)
		}
	}

	spin.Stop()
	fmt.Fprintf(os.Stderr, "Built %s\n", imageTag)
	return nil
}

func init() {
	prebuildCmd.Flags().String("podfile", "", "path to Podfile")
	prebuildCmd.Flags().String("tag", "", "custom image tag (default: content-addressed)")
	rootCmd.AddCommand(prebuildCmd)
}
