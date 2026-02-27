package config

import (
	"errors"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Auth     AuthConfig     `yaml:"auth"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Session  SessionConfig  `yaml:"session"`
	Log      LogConfig      `yaml:"log"`
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
	}
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

	return cfg, nil
}
