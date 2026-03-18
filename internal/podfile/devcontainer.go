package podfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DevContainer represents the subset of devcontainer.json we support.
type DevContainer struct {
	Image             string            `json:"image"`
	Features          map[string]any    `json:"features"`
	PostCreateCommand any               `json:"postCreateCommand"`
	PostStartCommand  any               `json:"postStartCommand"`
	RemoteUser        string            `json:"remoteUser"`
	ForwardPorts      []int             `json:"forwardPorts"`
	ContainerEnv      map[string]string `json:"containerEnv"`
	RemoteEnv         map[string]string `json:"remoteEnv"`
}

// FindDevContainer locates devcontainer.json in projectDir.
func FindDevContainer(projectDir string) (string, error) {
	candidates := []string{
		filepath.Join(projectDir, ".devcontainer", "devcontainer.json"),
		filepath.Join(projectDir, ".devcontainer.json"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no devcontainer.json found in %s", projectDir)
}

// ParseDevContainer reads and parses a devcontainer.json.
func ParseDevContainer(path string) (*DevContainer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// devcontainer.json is JSONC (allows comments)
	data = stripJSONComments(data)

	var dc DevContainer
	if err := json.Unmarshal(data, &dc); err != nil {
		return nil, fmt.Errorf("parsing devcontainer.json: %w", err)
	}
	return &dc, nil
}

// ToPodfile converts to a Podfile. Lossy; features are not expanded.
func (dc *DevContainer) ToPodfile() *Podfile {
	pf := &Podfile{
		Base:  dc.Image,
		Shell: "/bin/bash",
	}

	if pf.Base == "" {
		pf.Base = "ubuntu:24.04"
	}

	if len(dc.ContainerEnv) > 0 || len(dc.RemoteEnv) > 0 {
		pf.Env = make(map[string]string)
		for k, v := range dc.ContainerEnv {
			pf.Env[k] = v
		}
		for k, v := range dc.RemoteEnv {
			pf.Env[k] = v
		}
	}

	if len(dc.ForwardPorts) > 0 {
		pf.Ports.Expose = dc.ForwardPorts
	}

	pf.OnCreate = commandToString(dc.PostCreateCommand)
	pf.OnStart = commandToString(dc.PostStartCommand)

	return pf
}

func commandToString(cmd any) string {
	if cmd == nil {
		return ""
	}
	switch v := cmd.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " && ")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(v))
		for _, k := range keys {
			if s, ok := v[k].(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " && ")
	default:
		return fmt.Sprintf("%v", cmd)
	}
}

func stripJSONComments(data []byte) []byte {
	var result []byte
	i := 0
	for i < len(data) {
		if data[i] == '"' {
			result = append(result, data[i])
			i++
			for i < len(data) && data[i] != '"' {
				if data[i] == '\\' && i+1 < len(data) {
					result = append(result, data[i], data[i+1])
					i += 2
					continue
				}
				result = append(result, data[i])
				i++
			}
			if i < len(data) {
				result = append(result, data[i])
				i++
			}
			continue
		}
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '/' {
			i += 2
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '*' {
			i += 2
			for i+1 < len(data) && (data[i] != '*' || data[i+1] != '/') {
				i++
			}
			if i+1 < len(data) {
				i += 2 // skip closing */
			} else {
				i = len(data) // unclosed block comment, skip to end
			}
			continue
		}
		result = append(result, data[i])
		i++
	}
	return result
}
