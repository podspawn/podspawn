package cmd

import (
	"log/slog"
	"os"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfg     *config.Config
	logFile *os.File
)

var configOptionalCommands = map[string]bool{
	"help": true, "completion": true, "connect": true,
	"setup": true, "server-setup": true, "version": true,
	"list": true, "stop": true, "cleanup": true, "status": true,
	"list-users": true, "remove-user": true, "remove-project": true,
	"doctor": true,
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
				slog.Warn("config load failed, using defaults", "path", configPath, "error", err)
				loaded = config.Defaults()
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

func CloseLog() {
	if logFile != nil {
		_ = logFile.Close()
	}
}

func Execute() error {
	return rootCmd.Execute()
}
