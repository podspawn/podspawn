package cmd

import (
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
}

var rootCmd = &cobra.Command{
	Use:   "podspawn",
	Short: "Ephemeral SSH dev containers via native sshd",
	Long:  "Podspawn spawns ephemeral Docker containers on SSH connection, using the host's native sshd. No custom SSH server, no daemon, just containers.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		configPath, _ := cmd.Flags().GetString("config")
		loaded, err := config.Load(configPath)
		if err != nil {
			if configOptionalCommands[cmd.Name()] || !cmd.HasParent() {
				if localModeCommands[cmd.Name()] {
					loaded = config.LocalDefaults()
					loadLocalOverrides(loaded)
					isLocalMode = true
				} else {
					loaded = config.Defaults()
				}
			} else {
				return err
			}
		}
		cfg = loaded

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
