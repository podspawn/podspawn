package podfile

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// bangFields lists YAML keys that support the ! suffix for bang-replace.
var bangFields = map[string]func(*MergeFlags){
	"packages":       func(f *MergeFlags) { f.PackagesReplace = true },
	"env":            func(f *MergeFlags) { f.EnvReplace = true },
	"services":       func(f *MergeFlags) { f.ServicesReplace = true },
	"repos":          func(f *MergeFlags) { f.ReposReplace = true },
	"extra_commands": func(f *MergeFlags) { f.ExtraCommandsReplace = true },
	"on_create":      func(f *MergeFlags) { f.OnCreateReplace = true },
	"on_start":       func(f *MergeFlags) { f.OnStartReplace = true },
}

// ParseRaw decodes a Podfile with bang-replace syntax support.
// Keys ending in ! (e.g. packages!:) are detected, the ! is stripped,
// and the corresponding MergeFlags field is set. The resulting RawPodfile
// can be passed to Merge().
func ParseRaw(r io.Reader) (*RawPodfile, error) {
	var doc yaml.Node
	if err := yaml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decoding podfile: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("expected YAML document")
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected YAML mapping at top level")
	}

	var flags MergeFlags
	seen := make(map[string]bool)
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		key := mapping.Content[i]
		if key.Kind != yaml.ScalarNode {
			continue
		}

		keyName := key.Value
		if strings.HasSuffix(keyName, "!") {
			base := strings.TrimSuffix(keyName, "!")
			setter, ok := bangFields[base]
			if !ok {
				return nil, fmt.Errorf("bang-replace not supported on field %q", base)
			}
			if seen[base] {
				return nil, fmt.Errorf("cannot specify both %q and %q", base, keyName)
			}
			setter(&flags)
			key.Value = base
			keyName = base
		}

		if seen[keyName] {
			return nil, fmt.Errorf("duplicate key %q in podfile", keyName)
		}
		seen[keyName] = true
	}

	var pf Podfile
	if err := mapping.Decode(&pf); err != nil {
		return nil, fmt.Errorf("decoding podfile fields: %w", err)
	}

	if pf.Shell == "" {
		pf.Shell = "/bin/bash"
	}
	for i := range pf.Repos {
		if pf.Repos[i].Branch == "" {
			pf.Repos[i].Branch = "main"
		}
	}

	return &RawPodfile{Podfile: pf, Flags: flags}, nil
}
