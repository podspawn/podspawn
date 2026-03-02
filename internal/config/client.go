package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ClientConfig struct {
	Servers ServerRouting `yaml:"servers"`
}

type ServerRouting struct {
	Default  string            `yaml:"default"`
	Mappings map[string]string `yaml:"mappings"`
}

// LoadClient reads a client config from the given path. Unlike the server
// config, a missing file is an error since there are no useful defaults.
func LoadClient(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("client config not found: %s", path)
		}
		return nil, err
	}

	var cfg ClientConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &cfg, nil
}

// ResolveHost maps a .pod hostname to a real server address.
// Non-.pod hostnames pass through unchanged. For .pod hostnames,
// it checks Mappings first, then falls back to Default.
func (c *ClientConfig) ResolveHost(hostname string) (string, error) {
	if !strings.HasSuffix(hostname, ".pod") {
		return hostname, nil
	}

	if server, ok := c.Servers.Mappings[hostname]; ok {
		return server, nil
	}

	if c.Servers.Default != "" {
		return c.Servers.Default, nil
	}

	return "", fmt.Errorf("no server configured for %q (add it to servers.mappings or set servers.default)", hostname)
}
