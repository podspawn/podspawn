package podfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/podspawn/podspawn/internal/config"
	"gopkg.in/yaml.v3"
)

// Parse decodes a Podfile from an io.Reader, applies defaults, and validates.
func Parse(r io.Reader) (*Podfile, error) {
	var pf Podfile
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&pf); err != nil {
		return nil, fmt.Errorf("decoding podfile: %w", err)
	}

	if pf.Shell == "" {
		pf.Shell = "/bin/bash"
	}
	for i := range pf.Repos {
		if pf.Repos[i].Branch == "" {
			pf.Repos[i].Branch = "main"
		}
	}

	if err := pf.validate(); err != nil {
		return nil, err
	}
	return &pf, nil
}

// ParseFile reads and parses a Podfile from a filesystem path.
func ParseFile(path string) (*Podfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file
	return Parse(f)
}

// FindAndRead searches for a Podfile in a project directory and returns
// the raw bytes. Checks .podspawn/podfile.yaml first, then podfile.yaml.
func FindAndRead(projectDir string) ([]byte, error) {
	candidates := []string{
		filepath.Join(projectDir, ".podspawn", "podfile.yaml"),
		filepath.Join(projectDir, "podfile.yaml"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
	}
	return nil, fmt.Errorf("no podfile.yaml found in %s", projectDir)
}

func (pf *Podfile) validate() error {
	if pf.Base == "" {
		return fmt.Errorf("base image is required")
	}

	if pf.Shell != "" && !strings.HasPrefix(pf.Shell, "/") {
		return fmt.Errorf("shell must be absolute path, got %q", pf.Shell)
	}

	if pf.Resources.Memory != "" {
		if _, err := config.ParseMemory(pf.Resources.Memory); err != nil {
			return fmt.Errorf("invalid resources.memory: %w", err)
		}
	}

	for _, repo := range pf.Repos {
		if repo.URL == "" {
			return fmt.Errorf("repo url is required")
		}
		if repo.Path != "" && !strings.HasPrefix(repo.Path, "/") {
			return fmt.Errorf("repo path must be absolute, got %q", repo.Path)
		}
	}

	seen := make(map[string]bool)
	for _, svc := range pf.Services {
		if svc.Name == "" {
			return fmt.Errorf("service name is required")
		}
		if svc.Image == "" {
			return fmt.Errorf("service %q: image is required", svc.Name)
		}
		if seen[svc.Name] {
			return fmt.Errorf("duplicate service name %q", svc.Name)
		}
		seen[svc.Name] = true
	}

	return nil
}
