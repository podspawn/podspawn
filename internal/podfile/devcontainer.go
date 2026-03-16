package podfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DevContainer represents a subset of devcontainer.json that podspawn supports.
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

// FindDevContainer searches for a devcontainer.json in a project directory.
// Checks .devcontainer/devcontainer.json first, then .devcontainer.json.
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

// ParseDevContainer reads and parses a devcontainer.json file.
func ParseDevContainer(path string) (*DevContainer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Strip JSON comments (// and /* */) since devcontainer.json allows them (JSONC)
	data = stripJSONComments(data)

	var dc DevContainer
	if err := json.Unmarshal(data, &dc); err != nil {
		return nil, fmt.Errorf("parsing devcontainer.json: %w", err)
	}
	return &dc, nil
}

// ToPodfile converts a DevContainer to a Podfile representation.
// This is a lossy conversion; features are recorded but not fully expanded.
func (dc *DevContainer) ToPodfile() *Podfile {
	pf := &Podfile{
		Base:  dc.Image,
		Shell: "/bin/bash",
	}

	if pf.Base == "" {
		pf.Base = "ubuntu:24.04"
	}

	// Merge container and remote env vars
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

// commandToString converts a devcontainer command field (string, []string, or map)
// to a shell command string.
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
		parts := make([]string, 0, len(v))
		for _, val := range v {
			if s, ok := val.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " && ")
	default:
		return fmt.Sprintf("%v", cmd)
	}
}

// stripJSONComments removes // and /* */ comments from JSONC content.
// Naive implementation that handles the common cases.
func stripJSONComments(data []byte) []byte {
	var result []byte
	i := 0
	for i < len(data) {
		// Inside a string literal, skip to end
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
		// Line comment
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '/' {
			i += 2
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		// Block comment
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
