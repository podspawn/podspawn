package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfg         *config.Config
	logFile     *os.File
	isLocalMode bool
)

var configOptionalCommands = map[string]bool{
	"help": true, "completion": true, "connect": true,
	"setup": true, "server-setup": true, "version": true,
	"list": true, "stop": true, "cleanup": true, "status": true,
	"list-users": true, "remove-user": true, "remove-project": true,
	"doctor": true, "update": true,
	"config": true, "servers": true, "ssh": true, "open": true, "ping": true,
	"create": true, "run": true, "shell": true,
	"dev": true, "down": true, "init": true, "prebuild": true,
}

var localModeCommands = map[string]bool{
	"create": true, "run": true, "shell": true,
	"list": true, "stop": true, "doctor": true,
	"dev": true, "down": true, "init": true, "prebuild": true,
}

var rootCmd = &cobra.Command{
	Use:   "podspawn",
	Short: "Ephemeral dev containers, locally or over SSH",
	Long:  "Podspawn creates ephemeral Docker containers. Locally via create/run/shell, or remotely via SSH hooking into native sshd.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		configPath, _ := cmd.Flags().GetString("config")

		configExists, statErr := fileExists(configPath)
		if statErr != nil {
			return fmt.Errorf("checking config %s: %w", configPath, statErr)
		}

		if !configExists && localModeCommands[cmd.Name()] && !sshdHasPodspawn() {
			isLocalMode = true
			loaded := config.LocalDefaults()
			loadLocalOverrides(loaded)
			cfg = loaded
		} else {
			loaded, err := config.Load(configPath)
			if err != nil {
				if configOptionalCommands[cmd.Name()] || !cmd.HasParent() {
					slog.Warn("config load failed, using defaults", "path", configPath, "error", err)
					loaded = config.Defaults()
				} else {
					return err
				}
			}
			cfg = loaded
		}

		if isLocalMode {
			slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		}

		logPath, _ := cmd.Flags().GetString("log-file")
		if logPath == "" {
			logPath = cfg.Log.File
		}
		if logPath != "" {
			f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				slog.Warn("failed to open log file, using stderr", "path", logPath, "error", err)
			} else {
				logFile = f
				slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().String("config", "/etc/podspawn/config.yaml", "config file path")
	rootCmd.PersistentFlags().String("log-file", "", "log to file instead of stderr")
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func loadLocalOverrides(cfg *config.Config) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	clientCfg, err := config.LoadClient(filepath.Join(home, ".podspawn", "config.yaml"))
	if err != nil {
		return
	}
	clientCfg.Local.ApplyTo(cfg)
}

func CloseLog() {
	if logFile != nil {
		_ = logFile.Close()
	}
}

func Execute() error {
	return rootCmd.Execute()
}

// sshdHasPodspawn checks whether sshd_config has an AuthorizedKeysCommand
// pointing to podspawn. Used as a fallback to detect server mode when
// /etc/podspawn/config.yaml is missing.
func sshdHasPodspawn() bool {
	data, err := os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "authorizedkeyscommand") && strings.Contains(trimmed, "podspawn auth-keys") {
			return true
		}
	}
	return false
}
