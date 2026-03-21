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
	Local   LocalConfig   `yaml:"local"`
}

type LocalConfig struct {
	Image       string  `yaml:"image"`
	Shell       string  `yaml:"shell"`
	CPUs        float64 `yaml:"cpus"`
	Memory      string  `yaml:"memory"`
	GracePeriod string  `yaml:"grace_period"`
	MaxLifetime string  `yaml:"max_lifetime"`
	Mode        string  `yaml:"mode"`
}

func (lc *LocalConfig) ApplyTo(cfg *Config) {
	if lc.Image != "" {
		cfg.Defaults.Image = lc.Image
	}
	if lc.Shell != "" {
		cfg.Defaults.Shell = lc.Shell
	}
	if lc.CPUs > 0 {
		cfg.Defaults.CPUs = lc.CPUs
	}
	if lc.Memory != "" {
		cfg.Defaults.Memory = lc.Memory
	}
	if lc.GracePeriod != "" {
		cfg.Session.GracePeriod = lc.GracePeriod
	}
	if lc.MaxLifetime != "" {
		cfg.Session.MaxLifetime = lc.MaxLifetime
	}
	if lc.Mode != "" {
		cfg.Session.Mode = lc.Mode
	}
}

type ServerRouting struct {
	Default  string                  `yaml:"default"`
	Mappings map[string]*ServerEntry `yaml:"mappings"`
}

type ServerEntry struct {
	Host string `yaml:"host"`
	Mode string `yaml:"mode"` // "ephemeral" | "persistent" | "" (unknown)
}

// UnmarshalYAML supports both string values ("server.com") and object values ({host: server.com, mode: persistent}).
func (e *ServerEntry) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		e.Host = value.Value
		return nil
	}
	type plain ServerEntry
	return value.Decode((*plain)(e))
}

// Missing file is an error, unlike Load(), since there are no useful client defaults.
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

func (c *ClientConfig) ResolveHost(hostname string) (string, error) {
	if !strings.HasSuffix(hostname, ".pod") {
		return hostname, nil
	}

	if entry, ok := c.Servers.Mappings[hostname]; ok {
		return entry.Host, nil
	}

	if c.Servers.Default != "" {
		return c.Servers.Default, nil
	}

	return "", fmt.Errorf("no server configured for %q (add it to servers.mappings or set servers.default)", hostname)
}
