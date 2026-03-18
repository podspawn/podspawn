package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	Repo        string `yaml:"repo"`
	LocalPath   string `yaml:"local_path"`
	PodfileHash string `yaml:"podfile_hash"`
	ImageTag    string `yaml:"image_tag"`
}

func LoadProjects(path string) (map[string]ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return make(map[string]ProjectConfig), nil
		}
		return nil, fmt.Errorf("reading projects file: %w", err)
	}

	var projects map[string]ProjectConfig
	if err := yaml.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("parsing projects file: %w", err)
	}
	if projects == nil {
		projects = make(map[string]ProjectConfig)
	}
	return projects, nil
}

func SaveProjects(path string, projects map[string]ProjectConfig) error {
	data, err := yaml.Marshal(projects)
	if err != nil {
		return fmt.Errorf("marshaling projects: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".projects-*.yaml")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()        //nolint:errcheck
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("renaming %s to %s: %w", tmpPath, path, err)
	}
	return nil
}
