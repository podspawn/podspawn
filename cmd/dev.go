package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/podspawn/podspawn/internal/podfile"
	"github.com/podspawn/podspawn/internal/runtime"
	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev [-- command args...]",
	Short: "Start a dev environment from the current directory's Podfile",
	Long: `Auto-detect a podfile.yaml in the current directory, build the image,
start services, and drop into an interactive shell.

  podspawn dev                  -> shell in Podfile environment
  podspawn dev -- make test     -> run command and exit
  podspawn dev --ephemeral      -> destroy on exit`,
	RunE: runDev,
}

func runDev(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	podfilePath, _ := cmd.Flags().GetString("podfile")
	ephemeral, _ := cmd.Flags().GetBool("ephemeral")
	dotfilesFlag, _ := cmd.Flags().GetBool("dotfiles")
	nameOverride, _ := cmd.Flags().GetString("name")
	fresh, _ := cmd.Flags().GetBool("fresh")
	extraPorts, _ := cmd.Flags().GetIntSlice("ports")

	// Find and parse Podfile
	podfileDir := cwd
	if podfilePath != "" {
		podfileDir = filepath.Dir(podfilePath)
	}

	raw, err := podfile.FindAndRead(podfileDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No podfile.yaml found in %s\n", cwd)
		fmt.Fprintf(os.Stderr, "Run 'podspawn init' to create one.\n")
		return fmt.Errorf("no podfile found")
	}

	rawPf, err := podfile.ParseRaw(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("parsing podfile: %w", err)
	}
	pf, err := podfile.ResolveExtends(rawPf, podfileDir)
	if err != nil {
		return fmt.Errorf("resolving extends: %w", err)
	}

	// Session naming
	sessionName := nameOverride
	if sessionName == "" {
		sessionName = podfile.SessionName(cwd, pf.Name)
	}

	// Build local session
	ls, err := buildLocalSession(sessionName)
	if err != nil {
		return err
	}
	defer ls.Close()

	// Set project to CWD
	ls.Session.Project = &config.ProjectConfig{LocalPath: podfileDir}

	// Apply mode
	if pf.Mode != "" {
		ls.Session.Mode = pf.Mode
	}
	if ephemeral {
		ls.Session.Mode = "destroy-on-disconnect"
	}

	// Workspace mount
	mountMode := pf.Mount
	if mountMode == "" {
		mountMode = "bind"
	}
	workspaceTarget := pf.Workspace
	if workspaceTarget == "" {
		workspaceTarget = "/workspace/" + filepath.Base(cwd)
	}
	if mountMode == "bind" {
		ls.Session.WorkspaceMounts = []runtime.Mount{{
			Source: cwd,
			Target: workspaceTarget,
		}}
		ls.Session.WorkingDir = workspaceTarget
	}

	// Port forwarding
	strategy := pf.Ports.Strategy
	if strategy == "" {
		strategy = "auto"
	}
	bindings, err := podfile.ResolvePortBindings(pf.Ports.Expose, strategy, extraPorts)
	if err != nil {
		return fmt.Errorf("resolving port bindings: %w", err)
	}
	ls.Session.PortBindings = bindings

	// Dotfiles from user config
	if !dotfilesFlag {
		ls.Session.UserOverrides = nil
	}

	// Fresh: destroy existing session
	if fresh {
		_ = destroySessionByName(ls, sessionName)
	}

	// Check for command after --
	cmdArgs := cmd.ArgsLenAtDash()
	if cmdArgs >= 0 && len(args) > cmdArgs {
		execCmd := strings.Join(args[cmdArgs:], " ")
		_ = os.Setenv("SSH_ORIGINAL_COMMAND", execCmd)
	} else {
		_ = os.Unsetenv("SSH_ORIGINAL_COMMAND")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	// Check if re-attaching and print banner before entering the shell
	existing, _ := ls.Store.GetSession(ls.Session.Username, sessionName)
	if existing != nil {
		age := time.Since(existing.CreatedAt).Truncate(time.Second).String()
		fmt.Fprintf(os.Stderr, "Re-attaching to %s (started %s ago)\n", sessionName, age)
	} else {
		printDetailedBanner(sessionName, pf, workspaceTarget, cwd, bindings)
	}

	exitCode := ls.Session.RunAndCleanup(ctx)
	cancel()

	if exitCode != 0 {
		ls.Close()
		os.Exit(exitCode) //nolint:gocritic // matches shell.go pattern
	}
	return nil
}

func printDetailedBanner(name string, pf *podfile.Podfile, workspace, hostDir string, bindings []runtime.PortBinding) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  %s | %s\n", name, pf.Base)
	if len(pf.Packages) > 0 {
		pkgs := pf.Packages
		if len(pkgs) > 5 {
			pkgs = append(pkgs[:5], "...")
		}
		fmt.Fprintf(os.Stderr, "  packages: %s\n", strings.Join(pkgs, ", "))
	}
	if len(pf.Services) > 0 {
		var svcs []string
		for _, s := range pf.Services {
			if len(s.Ports) > 0 {
				svcs = append(svcs, fmt.Sprintf("%s (:%d)", s.Name, s.Ports[0]))
			} else {
				svcs = append(svcs, s.Name)
			}
		}
		fmt.Fprintf(os.Stderr, "  services: %s\n", strings.Join(svcs, ", "))
	}
	if len(bindings) > 0 {
		var ports []string
		for _, b := range bindings {
			ports = append(ports, fmt.Sprintf("localhost:%d -> :%d", b.HostPort, b.ContainerPort))
		}
		fmt.Fprintf(os.Stderr, "  ports:    %s\n", strings.Join(ports, ", "))
	}
	if workspace != "" {
		short := hostDir
		if home, err := os.UserHomeDir(); err == nil {
			short = strings.Replace(hostDir, home, "~", 1)
		}
		fmt.Fprintf(os.Stderr, "  mount:    %s -> %s\n", short, workspace)
	}
	fmt.Fprintf(os.Stderr, "\n  Run 'podspawn down' to stop.\n\n")
}

func init() {
	devCmd.Flags().String("podfile", "", "explicit Podfile path")
	devCmd.Flags().Bool("ephemeral", false, "destroy container on exit")
	devCmd.Flags().Bool("dotfiles", false, "apply user dotfiles from ~/.podspawn/config.yaml")
	devCmd.Flags().String("name", "", "override session name")
	devCmd.Flags().Bool("fresh", false, "destroy existing session and start new")
	devCmd.Flags().IntSlice("ports", nil, "additional ports to forward")
	rootCmd.AddCommand(devCmd)
}
