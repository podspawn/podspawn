package podfile

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed templates
var embeddedFS embed.FS

// Registry is the machine-readable index of templates and bases.
type Registry struct {
	SchemaVersion int             `yaml:"schema_version"`
	Bases         []RegistryEntry `yaml:"bases"`
	Templates     []RegistryEntry `yaml:"templates"`
}

// RegistryEntry describes a single template or base in the registry.
type RegistryEntry struct {
	Name        string       `yaml:"name"`
	File        string       `yaml:"file"`
	Description string       `yaml:"description"`
	Tags        []string     `yaml:"tags"`
	Detect      *DetectRules `yaml:"detect"`
}

// DetectRules maps file patterns to template selection for podspawn init.
type DetectRules struct {
	Files    []string `yaml:"files"`
	Priority int      `yaml:"priority"`
	All      bool     `yaml:"all"` // true = all files must be present
}

// LoadRegistry parses the embedded registry.yaml.
func LoadRegistry() (*Registry, error) {
	data, err := embeddedFS.ReadFile("templates/registry.yaml")
	if err != nil {
		return nil, fmt.Errorf("reading embedded registry: %w", err)
	}
	var reg Registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing embedded registry: %w", err)
	}
	return &reg, nil
}

// LookupBase returns the raw YAML bytes for a base by name.
func LookupBase(name string) ([]byte, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	for _, b := range reg.Bases {
		if b.Name == name {
			return embeddedFS.ReadFile("templates/" + b.File)
		}
	}
	return nil, fmt.Errorf("unknown base %q", name)
}

// LookupTemplate returns the raw YAML bytes for a template by name.
func LookupTemplate(name string) ([]byte, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	for _, t := range reg.Templates {
		if t.Name == name {
			return embeddedFS.ReadFile("templates/" + t.File)
		}
	}
	return nil, fmt.Errorf("unknown template %q", name)
}
