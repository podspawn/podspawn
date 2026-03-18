package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type UserOverrides struct {
	Image    string            `yaml:"image"`
	CPUs     float64           `yaml:"cpus"`
	Memory   string            `yaml:"memory"`
	Shell    string            `yaml:"shell"`
	Env      map[string]string `yaml:"env"`
	Dotfiles *DotfilesOverride `yaml:"dotfiles"`
}

type DotfilesOverride struct {
	Repo    string `yaml:"repo"`
	Install string `yaml:"install"`
}

func LoadUserOverrides(baseDir, username string) (*UserOverrides, error) {
	path := filepath.Join(baseDir, "users", username+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading user overrides for %s: %w", username, err)
	}

	var uo UserOverrides
	if err := yaml.Unmarshal(data, &uo); err != nil {
		return nil, fmt.Errorf("parsing user overrides for %s: %w", username, err)
	}
	return &uo, nil
}
