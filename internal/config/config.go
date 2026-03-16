package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Auth         AuthConfig     `yaml:"auth"`
	Defaults     DefaultsConfig `yaml:"defaults"`
	Session      SessionConfig  `yaml:"session"`
	State        StateConfig    `yaml:"state"`
	Log          LogConfig      `yaml:"log"`
	Security     SecurityConfig `yaml:"security"`
	ProjectsFile string         `yaml:"projects_file"`
}

type SecurityConfig struct {
	CapDrop      []string          `yaml:"cap_drop"`
	CapAdd       []string          `yaml:"cap_add"`
	NoNewPrivs   bool              `yaml:"no_new_privileges"`
	PidsLimit    int64             `yaml:"pids_limit"`
	ReadonlyRoot bool              `yaml:"readonly_rootfs"`
	Tmpfs        map[string]string `yaml:"tmpfs"`
	RuntimeName  string            `yaml:"runtime"` // "docker" (default), "runsc" (gVisor)
}

type AuthConfig struct {
	KeyDir string `yaml:"key_dir"`
}

type DefaultsConfig struct {
	Image  string  `yaml:"image"`
	Shell  string  `yaml:"shell"`
	CPUs   float64 `yaml:"cpus"`
	Memory string  `yaml:"memory"`
}

type SessionConfig struct {
	GracePeriod string `yaml:"grace_period"`
	MaxLifetime string `yaml:"max_lifetime"`
	Mode        string `yaml:"mode"`
}

type StateConfig struct {
	DBPath  string `yaml:"db_path"`
	LockDir string `yaml:"lock_dir"`
}

type LogConfig struct {
	File string `yaml:"file"`
}

func Defaults() *Config {
	return &Config{
		Auth: AuthConfig{
			KeyDir: "/etc/podspawn/keys",
		},
		Defaults: DefaultsConfig{
			Image:  "ubuntu:24.04",
			Shell:  "/bin/bash",
			CPUs:   2.0,
			Memory: "2g",
		},
		Session: SessionConfig{
			GracePeriod: "60s",
			MaxLifetime: "8h",
			Mode:        "grace-period",
		},
		State: StateConfig{
			DBPath:  "/var/lib/podspawn/state.db",
			LockDir: "/var/lib/podspawn/locks",
		},
		Security: SecurityConfig{
			CapDrop:    []string{"ALL"},
			CapAdd:     []string{"CHOWN", "SETUID", "SETGID", "DAC_OVERRIDE", "FOWNER", "NET_BIND_SERVICE"},
			NoNewPrivs: true,
			PidsLimit:  256,
		},
		ProjectsFile: "/etc/podspawn/projects.yaml",
	}
}

func (c *Config) Validate() error {
	if _, err := time.ParseDuration(c.Session.GracePeriod); err != nil {
		return fmt.Errorf("invalid session.grace_period %q: must include time unit (e.g. 60s, 8h)", c.Session.GracePeriod)
	}
	if _, err := time.ParseDuration(c.Session.MaxLifetime); err != nil {
		return fmt.Errorf("invalid session.max_lifetime %q: must include time unit (e.g. 60s, 8h)", c.Session.MaxLifetime)
	}
	if _, err := ParseMemory(c.Defaults.Memory); err != nil {
		return fmt.Errorf("invalid defaults.memory %q: %w", c.Defaults.Memory, err)
	}
	return nil
}

// Load reads a YAML config file and returns a Config with defaults
// applied for any missing fields. If the file doesn't exist, returns
// defaults without error.
func Load(path string) (*Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
