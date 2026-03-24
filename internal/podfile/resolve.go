package podfile

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxExtendsDepth = 10

// ResolveExtends recursively resolves a Podfile's extends chain and returns
// the fully merged result. If the Podfile has no extends field, it is returned
// as-is. Circular references and chains deeper than 10 levels are errors.
func ResolveExtends(raw *RawPodfile, podfileDir string) (*Podfile, error) {
	return resolveChain(raw, podfileDir, nil)
}

func resolveChain(raw *RawPodfile, podfileDir string, visited map[string]bool) (*Podfile, error) {
	if raw.Extends == "" {
		return &raw.Podfile, nil
	}

	if visited == nil {
		visited = make(map[string]bool)
	}
	if len(visited) >= maxExtendsDepth {
		return nil, fmt.Errorf("extends chain too deep (max %d)", maxExtendsDepth)
	}

	ref := raw.Extends
	key := normalizeRef(ref, podfileDir)
	if visited[key] {
		return nil, fmt.Errorf("circular extends: %q already in chain", ref)
	}
	visited[key] = true

	baseBytes, baseDir, err := ResolveSource(ref, podfileDir)
	if err != nil {
		return nil, fmt.Errorf("resolving extends %q: %w", ref, err)
	}

	baseRaw, err := ParseRaw(bytes.NewReader(baseBytes))
	if err != nil {
		return nil, fmt.Errorf("parsing base %q: %w", ref, err)
	}

	basePf, err := resolveChain(baseRaw, baseDir, visited)
	if err != nil {
		return nil, err
	}

	return Merge(basePf, raw), nil
}

// ResolveSource loads a Podfile from an extends reference.
// Resolution order:
//  1. Local file path (starts with . or / or ends in .yaml)
//  2. Registry shorthand (bare name like "go" or "ubuntu-dev")
//  3. GitHub URL (starts with github.com/)
func ResolveSource(ref string, podfileDir string) ([]byte, string, error) {
	if isLocalRef(ref) {
		return resolveLocalPath(ref, podfileDir)
	}

	if !strings.Contains(ref, "/") {
		return resolveRegistryShorthand(ref)
	}

	if strings.HasPrefix(ref, "github.com/") {
		return resolveGitHubURL(ref)
	}

	return nil, "", fmt.Errorf("cannot resolve extends reference %q: expected local path, registry name, or github.com/ URL", ref)
}

func isLocalRef(ref string) bool {
	return strings.HasPrefix(ref, ".") || strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, ".yaml") || strings.HasSuffix(ref, ".yml")
}

func resolveLocalPath(ref string, podfileDir string) ([]byte, string, error) {
	path := ref
	if !filepath.IsAbs(path) {
		path = filepath.Join(podfileDir, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading local podfile %s: %w", path, err)
	}
	return data, filepath.Dir(path), nil
}

func resolveRegistryShorthand(name string) ([]byte, string, error) {
	// Try bases first, then templates
	if data, err := LookupBase(name); err == nil {
		return data, "", nil
	}
	if data, err := LookupTemplate(name); err == nil {
		return data, "", nil
	}

	// Try cached registry (from podspawn init --update)
	if data, dir, err := resolveCachedRegistry(name); err == nil {
		return data, dir, nil
	}

	return nil, "", fmt.Errorf("unknown extends reference %q: not found in embedded bases or templates", name)
}

func resolveCachedRegistry(name string) ([]byte, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}
	cacheDir := filepath.Join(home, ".podspawn", "cache", "podfiles")

	// Check bases/ then templates/
	for _, subdir := range []string{"bases", "templates"} {
		path := filepath.Join(cacheDir, subdir, name+".yaml")
		if data, err := os.ReadFile(path); err == nil {
			return data, filepath.Dir(path), nil
		}
	}
	return nil, "", fmt.Errorf("not in cache")
}

func resolveGitHubURL(ref string) ([]byte, string, error) {
	// github.com/org/repo/path/to/podfile.yaml -> raw content URL
	rawURL := "https://raw." + strings.Replace(ref, "github.com/", "githubusercontent.com/", 1)
	if !strings.Contains(rawURL, "/main/") && !strings.Contains(rawURL, "/master/") {
		// If no branch specified, assume the path is relative to repo root on main
		parts := strings.SplitN(ref, "/", 4)
		if len(parts) >= 3 {
			rawURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/%s",
				parts[1], parts[2], strings.Join(parts[3:], "/"))
			if len(parts) == 3 {
				rawURL = fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/main/podfile.yaml",
					parts[1], parts[2])
			}
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetching %s: %w", rawURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetching %s: HTTP %d", rawURL, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading response from %s: %w", rawURL, err)
	}

	return data, "", nil
}

// MarshalCanonical produces deterministic YAML from a Podfile for hashing.
// Map keys are sorted so the same logical Podfile always hashes identically.
func MarshalCanonical(pf *Podfile) ([]byte, error) {
	return yaml.Marshal(pf)
}

func normalizeRef(ref string, podfileDir string) string {
	if isLocalRef(ref) {
		if filepath.IsAbs(ref) {
			return ref
		}
		return filepath.Join(podfileDir, ref)
	}
	return ref
}
