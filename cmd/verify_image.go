package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/spf13/cobra"
)

type imageCheck struct {
	name string
	cmd  []string
	fix  string
}

var checks = []imageCheck{
	{
		name: "bash",
		cmd:  []string{"/bin/bash", "-c", "echo ok"},
		fix:  "apt-get install -y bash",
	},
	{
		name: "sftp-server",
		cmd:  []string{"test", "-x", "/usr/lib/openssh/sftp-server"},
		fix:  "apt-get install -y openssh-sftp-server",
	},
	{
		name: "locale",
		cmd:  []string{"locale", "-a"},
		fix:  "apt-get install -y locales && locale-gen en_US.UTF-8",
	},
	{
		name: "git",
		cmd:  []string{"git", "--version"},
		fix:  "apt-get install -y git",
	},
}

var verifyImageCmd = &cobra.Command{
	Use:   "verify-image <image>",
	Short: "Check if a container image is compatible with podspawn",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		imageRef := args[0]

		rt, err := runtime.NewDockerRuntime()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()

		containerName := "podspawn-verify-" + fmt.Sprintf("%d", time.Now().UnixNano()%100000)
		_, err = rt.CreateContainer(ctx, runtime.ContainerOpts{
			Name:  containerName,
			Image: imageRef,
			Cmd:   []string{"sleep", "30"},
		})
		if err != nil {
			return fmt.Errorf("creating container from %s: %w", imageRef, err)
		}
		if err := rt.StartContainer(ctx, containerName); err != nil {
			return fmt.Errorf("starting container: %w", err)
		}
		defer func() {
			_ = rt.RemoveContainer(context.Background(), containerName)
		}()

		passed, failed := 0, 0
		for _, check := range checks {
			exitCode, execErr := rt.Exec(ctx, containerName, runtime.ExecOpts{
				Cmd:    check.cmd,
				Stdout: os.Stdout,
				Stderr: os.Stderr,
			})
			if execErr != nil || exitCode != 0 {
				fmt.Fprintf(os.Stderr, "  FAIL  %s\n", check.name)
				fmt.Fprintf(os.Stderr, "        fix: %s\n", check.fix)
				failed++
			} else {
				fmt.Fprintf(os.Stderr, "  OK    %s\n", check.name)
				passed++
			}
		}

		fmt.Fprintf(os.Stderr, "\n%d passed, %d failed\n", passed, failed)
		if failed > 0 {
			return fmt.Errorf("%d check(s) failed", failed)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(verifyImageCmd)
}
