package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/podspawn/podspawn/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config [set <key> <value>]",
	Short: "Show or edit client configuration",
	Long:  `Without arguments, prints the current client config. With "set", updates a config value.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath, err := clientConfigPath()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			return showConfig(cfgPath)
		}

		if args[0] == "set" && len(args) >= 3 {
			return setConfig(cfgPath, args[1], args[2])
		}

		return fmt.Errorf("usage: podspawn config [set <key> <value>]")
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func clientConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".podspawn", "config.yaml"), nil
}

func showConfig(path string) error {
	clientCfg, err := config.LoadClient(path)
	if err != nil {
		fmt.Println("No client config found. Create one with:")
		fmt.Println("  podspawn config set servers.default yourserver.com")
		return nil
	}

	fmt.Printf("config: %s\n\n", path)

	if clientCfg.Servers.Default != "" {
		fmt.Printf("servers.default = %s\n", clientCfg.Servers.Default)
	}
	for host, entry := range clientCfg.Servers.Mappings {
		if entry.Mode != "" {
			fmt.Printf("servers.mappings.%s = %s (mode: %s)\n", host, entry.Host, entry.Mode)
		} else {
			fmt.Printf("servers.mappings.%s = %s\n", host, entry.Host)
		}
	}
	if clientCfg.Servers.Default == "" && len(clientCfg.Servers.Mappings) == 0 {
		fmt.Println("(empty)")
	}
	return nil
}

func setConfig(path, key, value string) error {
	clientCfg, _ := config.LoadClient(path)
	if clientCfg == nil {
		clientCfg = &config.ClientConfig{}
	}

	switch {
	case key == "servers.default":
		clientCfg.Servers.Default = value
	case strings.HasPrefix(key, "servers.mappings."):
		rest := strings.TrimPrefix(key, "servers.mappings.")
		if clientCfg.Servers.Mappings == nil {
			clientCfg.Servers.Mappings = make(map[string]*config.ServerEntry)
		}
		// Check if the key ends with .host or .mode (field access).
		// Otherwise treat the entire rest as hostname (backward compatible).
		var host, field string
		if strings.HasSuffix(rest, ".host") {
			host = strings.TrimSuffix(rest, ".host")
			field = "host"
		} else if strings.HasSuffix(rest, ".mode") {
			host = strings.TrimSuffix(rest, ".mode")
			field = "mode"
		} else {
			host = rest
			field = "host"
		}
		if clientCfg.Servers.Mappings[host] == nil {
			clientCfg.Servers.Mappings[host] = &config.ServerEntry{}
		}
		switch field {
		case "host":
			clientCfg.Servers.Mappings[host].Host = value
		case "mode":
			clientCfg.Servers.Mappings[host].Mode = value
		}
	default:
		return fmt.Errorf("unknown config key %q; valid keys: servers.default, servers.mappings.<host>[.host|.mode]", key)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(clientCfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("set %s = %s\n", key, value)
	return nil
}
