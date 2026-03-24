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

// Parse decodes a Podfile, applies defaults, and validates.
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

// ParseFile reads and parses a Podfile from path.
func ParseFile(path string) (*Podfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file
	return Parse(f)
}

// FindAndRead locates and reads a Podfile (or devcontainer.json fallback) from projectDir.
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

	dcPath, err := FindDevContainer(projectDir)
	if err == nil {
		dc, parseErr := ParseDevContainer(dcPath)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", dcPath, parseErr)
		}
		pf := dc.ToPodfile()
		data, marshalErr := yaml.Marshal(pf)
		if marshalErr != nil {
			return nil, fmt.Errorf("converting devcontainer to podfile: %w", marshalErr)
		}
		return data, nil
	}

	return nil, fmt.Errorf("no podfile.yaml or devcontainer.json found in %s", projectDir)
}

var validMounts = map[string]bool{"": true, "bind": true, "copy": true, "none": true}
var validModes = map[string]bool{"": true, "grace-period": true, "destroy-on-disconnect": true, "persistent": true}
var validPortStrategies = map[string]bool{"": true, "auto": true, "manual": true, "expose": true}

func (pf *Podfile) validate() error {
	if pf.Base == "" && pf.Extends == "" {
		return fmt.Errorf("base image is required (or use extends to inherit one)")
	}

	if pf.Shell != "" && !strings.HasPrefix(pf.Shell, "/") {
		return fmt.Errorf("shell must be absolute path, got %q", pf.Shell)
	}

	if pf.Resources.Memory != "" {
		if _, err := config.ParseMemory(pf.Resources.Memory); err != nil {
			return fmt.Errorf("invalid resources.memory: %w", err)
		}
	}

	if !validMounts[pf.Mount] {
		return fmt.Errorf("mount must be bind, copy, or none, got %q", pf.Mount)
	}

	if !validModes[pf.Mode] {
		return fmt.Errorf("mode must be grace-period, destroy-on-disconnect, or persistent, got %q", pf.Mode)
	}

	if !validPortStrategies[pf.Ports.Strategy] {
		return fmt.Errorf("ports.strategy must be auto, manual, or expose, got %q", pf.Ports.Strategy)
	}

	if pf.Workspace != "" && !strings.HasPrefix(pf.Workspace, "/") {
		return fmt.Errorf("workspace must be absolute path, got %q", pf.Workspace)
	}

	if pf.Extends == "" {
		for _, pkg := range pf.Packages {
			if strings.HasPrefix(pkg, "!") {
				return fmt.Errorf("item removal %q only valid when extends is set", pkg)
			}
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
