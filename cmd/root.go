package cmd

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

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
}

var localModeCommands = map[string]bool{
	"create": true, "run": true, "shell": true,
	"list": true, "stop": true, "doctor": true,
}

var rootCmd = &cobra.Command{
	Use:   "podspawn",
	Short: "Ephemeral dev containers, locally or over SSH",
	Long:  "Podspawn creates ephemeral Docker containers. Locally via create/run/shell, or remotely via SSH hooking into native sshd.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		configPath, _ := cmd.Flags().GetString("config")

		// Detect local mode: server config doesn't exist and command supports local mode
		serverConfigExists := fileExists(configPath)

		if !serverConfigExists && localModeCommands[cmd.Name()] {
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

		// Suppress slog for local-mode commands (user-facing, not server internals)
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
