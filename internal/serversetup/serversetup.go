package serversetup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var osName = runtime.GOOS

type Paths struct {
	SSHDConfig   string
	BinaryPath   string
	PodspawnDir  string
	KeyDir       string
	StateDir     string
	LockDir      string
	EmergencyKey string
}

func DefaultPaths(binaryPath string) Paths {
	return Paths{
		SSHDConfig:   "/etc/ssh/sshd_config",
		BinaryPath:   binaryPath,
		PodspawnDir:  "/etc/podspawn",
		KeyDir:       "/etc/podspawn/keys",
		StateDir:     "/var/lib/podspawn",
		LockDir:      "/var/lib/podspawn/locks",
		EmergencyKey: "/etc/podspawn/emergency.keys",
	}
}

type Options struct {
	DryRun      bool
	ServiceName string
}

func Run(paths Paths, cmd Commander, opts Options, out io.Writer) (retErr error) {
	if err := cmd.Run("sshd", "-t"); err != nil {
		return fmt.Errorf("current sshd config is invalid, fix it before running server-setup: %w", err)
	}

	data, err := os.ReadFile(paths.SSHDConfig)
	if err != nil {
		return fmt.Errorf("reading %s: %w", paths.SSHDConfig, err)
	}

	alreadyConfigured := hasOurAuthKeysCommand(data)
	if alreadyConfigured {
		fmt.Fprintln(out, "AuthorizedKeysCommand already configured, skipping sshd_config modification") //nolint:errcheck
	} else {
		if hasOtherAuthKeysCommand(data) {
			return fmt.Errorf("another AuthorizedKeysCommand is already set in %s; remove it first or configure podspawn manually", paths.SSHDConfig)
		}

		for _, w := range checkSafety(data) {
			fmt.Fprintf(out, "WARNING: %s\n", w) //nolint:errcheck
		}

		if opts.DryRun {
			fmt.Fprintln(out, "[dry-run] would back up", paths.SSHDConfig)                         //nolint:errcheck
			fmt.Fprintln(out, "[dry-run] would append AuthorizedKeysCommand to", paths.SSHDConfig) //nolint:errcheck
			fmt.Fprintln(out, "[dry-run] would validate new config with sshd -t")                  //nolint:errcheck
		} else {
			if err := modifySSHDConfig(paths, cmd, out); err != nil {
				return err
			}
		}
	}

	if opts.DryRun {
		fmt.Fprintln(out, "[dry-run] would create directories:", paths.PodspawnDir, paths.KeyDir, paths.StateDir, paths.LockDir) //nolint:errcheck
		fmt.Fprintln(out, "[dry-run] would create", paths.EmergencyKey, "(if missing)")                                          //nolint:errcheck
		fmt.Fprintf(out, "[dry-run] would reload sshd (%s)\n", reloadHint(opts.ServiceName))                                     //nolint:errcheck
		return nil
	}

	if err := createDirectories(paths); err != nil {
		return err
	}

	if !alreadyConfigured {
		if err := reloadSSHD(cmd, opts.ServiceName); err != nil {
			hint := reloadHint(opts.ServiceName)
			return fmt.Errorf("failed to reload sshd (config is valid, reload manually with: %s): %w", hint, err)
		}
		fmt.Fprintln(out, "reloaded sshd") //nolint:errcheck
	}

	fmt.Fprintln(out, "server-setup complete") //nolint:errcheck
	return nil
}

func modifySSHDConfig(paths Paths, cmd Commander, out io.Writer) (retErr error) {
	backupPath := paths.SSHDConfig + ".podspawn.bak"

	data, err := os.ReadFile(paths.SSHDConfig)
	if err != nil {
		return fmt.Errorf("reading %s: %w", paths.SSHDConfig, err)
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("creating backup at %s: %w", backupPath, err)
	}
	fmt.Fprintln(out, "backed up", paths.SSHDConfig, "to", backupPath) //nolint:errcheck

	modified := false
	defer func() {
		if modified && retErr != nil {
			if restoreErr := os.WriteFile(paths.SSHDConfig, data, 0644); restoreErr != nil {
				fmt.Fprintf(out, "CRITICAL: failed to restore backup: %v\n", restoreErr) //nolint:errcheck
			} else {
				fmt.Fprintln(out, "restored sshd_config from backup") //nolint:errcheck
			}
		}
	}()

	block := fmt.Sprintf("\n# Added by podspawn server-setup\nAuthorizedKeysCommand %s auth-keys %%u %%t %%k\nAuthorizedKeysCommandUser nobody\nAcceptEnv PODSPAWN_PROJECT\n", paths.BinaryPath)

	f, err := os.OpenFile(paths.SSHDConfig, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening %s for append: %w", paths.SSHDConfig, err)
	}
	if _, err := f.WriteString(block); err != nil {
		f.Close() //nolint:errcheck
		return fmt.Errorf("appending to %s: %w", paths.SSHDConfig, err)
	}
	f.Close() //nolint:errcheck
	modified = true

	fmt.Fprintln(out, "appended AuthorizedKeysCommand to", paths.SSHDConfig) //nolint:errcheck

	if err := cmd.Run("sshd", "-t"); err != nil {
		return fmt.Errorf("sshd config validation failed after changes (AuthorizedKeysCommand may conflict with an included file): %w", err)
	}

	return nil
}

func createDirectories(paths Paths) error {
	dirs := []struct {
		path string
		perm os.FileMode
	}{
		{paths.PodspawnDir, 0755},
		{paths.KeyDir, 0755}, // readable by AuthorizedKeysCommandUser (nobody)
		// State and lock dirs are 1777 (sticky + world-writable) so that
		// multiple users spawned via sshd can create SQLite WAL/SHM and
		// lock files. Sticky bit prevents users from deleting each other's files.
		{paths.StateDir, os.ModeSticky | 0777},
		{paths.LockDir, os.ModeSticky | 0777},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.perm); err != nil {
			return fmt.Errorf("creating %s: %w", d.path, err)
		}
		// MkdirAll doesn't update permissions on existing directories
		if err := os.Chmod(d.path, d.perm); err != nil {
			return fmt.Errorf("setting permissions on %s: %w", d.path, err)
		}
	}

	if _, err := os.Stat(paths.EmergencyKey); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(paths.EmergencyKey, nil, 0600); err != nil {
			return fmt.Errorf("creating %s: %w", paths.EmergencyKey, err)
		}
	}

	// Pre-create state DB so non-root users (via sshd ForceCommand) can use it.
	// SQLite needs write access to the DB file and its directory (for WAL/SHM).
	stateDB := filepath.Join(paths.StateDir, "state.db")
	if _, err := os.Stat(stateDB); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(stateDB, nil, 0666); err != nil {
			return fmt.Errorf("creating %s: %w", stateDB, err)
		}
	}

	// Write default config.yaml so doctor and other commands detect server mode
	configPath := filepath.Join(paths.PodspawnDir, "config.yaml")
	if _, err := os.Stat(configPath); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(configPath, defaultConfigYAML(), 0644); err != nil {
			return fmt.Errorf("creating %s: %w", configPath, err)
		}
	}

	return nil
}

func defaultConfigYAML() []byte {
	return []byte(`# Podspawn server configuration
# See https://podspawn.dev/docs/configuration for all options.

defaults:
  image: ubuntu:24.04
  shell: /bin/bash
  cpus: 2.0
  memory: 2g

session:
  grace_period: 60s
  max_lifetime: 8h
  mode: grace-period
`)
}

func hasOurAuthKeysCommand(data []byte) bool {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "authorizedkeyscommand") && strings.Contains(line, "podspawn auth-keys") {
			return true
		}
	}
	return false
}

func hasOtherAuthKeysCommand(data []byte) bool {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "authorizedkeyscommand") && !strings.Contains(line, "podspawn auth-keys") {
			return true
		}
	}
	return false
}

func checkSafety(data []byte) []string {
	var warnings []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	passwordAuthOff := false
	keysFileNone := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "passwordauthentication") && strings.Contains(lower, "no") {
			passwordAuthOff = true
		}
		if strings.HasPrefix(lower, "authorizedkeysfile") && strings.Contains(lower, "none") {
			keysFileNone = true
		}
	}

	if keysFileNone {
		warnings = append(warnings, "AuthorizedKeysFile is set to none; real system users will lose SSH key authentication")
	}
	if passwordAuthOff && keysFileNone {
		warnings = append(warnings, "both password and key-file auth appear disabled; verify you have another way to access this server")
	}

	return warnings
}

func reloadSSHD(cmd Commander, serviceOverride string) error {
	if osName == "darwin" {
		return cmd.Run("launchctl", "kickstart", "-k", "system/com.openssh.sshd")
	}
	svc := serviceOverride
	if svc == "" {
		svc = detectSSHService(cmd)
	}
	return cmd.Run("systemctl", "reload", svc)
}

func reloadHint(serviceOverride string) string {
	if osName == "darwin" {
		return "launchctl kickstart -k system/com.openssh.sshd"
	}
	svc := serviceOverride
	if svc == "" {
		svc = "sshd"
	}
	return "systemctl reload " + svc
}

func detectSSHService(cmd Commander) string {
	if err := cmd.Run("systemctl", "is-active", "ssh"); err == nil {
		return "ssh"
	}
	return "sshd"
}

// ResolveBinaryPath returns the absolute path of the running binary,
// resolving symlinks.
func ResolveBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("determining binary path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolving binary path: %w", err)
	}
	return resolved, nil
}
