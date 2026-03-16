package podfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDevContainerBasic(t *testing.T) {
	dir := t.TempDir()
	dcPath := filepath.Join(dir, "devcontainer.json")
	content := `{
		"image": "mcr.microsoft.com/devcontainers/go:1.22",
		"forwardPorts": [8080, 3000],
		"containerEnv": {
			"GO111MODULE": "on",
			"EDITOR": "code"
		},
		"postCreateCommand": "go mod download",
		"postStartCommand": "echo ready"
	}`
	if err := os.WriteFile(dcPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	dc, err := ParseDevContainer(dcPath)
	if err != nil {
		t.Fatal(err)
	}

	if dc.Image != "mcr.microsoft.com/devcontainers/go:1.22" {
		t.Errorf("Image = %q", dc.Image)
	}
	if len(dc.ForwardPorts) != 2 || dc.ForwardPorts[0] != 8080 {
		t.Errorf("ForwardPorts = %v", dc.ForwardPorts)
	}
	if dc.ContainerEnv["GO111MODULE"] != "on" {
		t.Errorf("ContainerEnv = %v", dc.ContainerEnv)
	}
}

func TestParseDevContainerJSONC(t *testing.T) {
	dir := t.TempDir()
	dcPath := filepath.Join(dir, "devcontainer.json")
	content := `{
		// This is a comment
		"image": "ubuntu:24.04",
		/* block comment */
		"forwardPorts": [3000]
	}`
	if err := os.WriteFile(dcPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	dc, err := ParseDevContainer(dcPath)
	if err != nil {
		t.Fatalf("JSONC parsing failed: %v", err)
	}
	if dc.Image != "ubuntu:24.04" {
		t.Errorf("Image = %q after comment stripping", dc.Image)
	}
}

func TestDevContainerToPodfile(t *testing.T) {
	dc := &DevContainer{
		Image:             "node:22",
		ForwardPorts:      []int{3000, 5173},
		ContainerEnv:      map[string]string{"NODE_ENV": "development"},
		PostCreateCommand: "npm install",
		PostStartCommand:  "echo welcome",
	}

	pf := dc.ToPodfile()

	if pf.Base != "node:22" {
		t.Errorf("Base = %q, want node:22", pf.Base)
	}
	if len(pf.Ports.Expose) != 2 {
		t.Errorf("Ports = %v, want [3000 5173]", pf.Ports.Expose)
	}
	if pf.Env["NODE_ENV"] != "development" {
		t.Errorf("Env = %v", pf.Env)
	}
	if pf.OnCreate != "npm install" {
		t.Errorf("OnCreate = %q, want 'npm install'", pf.OnCreate)
	}
	if pf.OnStart != "echo welcome" {
		t.Errorf("OnStart = %q, want 'echo welcome'", pf.OnStart)
	}
}

func TestDevContainerToPodfileDefaultImage(t *testing.T) {
	dc := &DevContainer{}
	pf := dc.ToPodfile()
	if pf.Base != "ubuntu:24.04" {
		t.Errorf("Base = %q, want ubuntu:24.04 (default)", pf.Base)
	}
}

func TestDevContainerCommandArray(t *testing.T) {
	dc := &DevContainer{
		Image:             "ubuntu:24.04",
		PostCreateCommand: []any{"npm install", "npm run build"},
	}
	pf := dc.ToPodfile()
	if pf.OnCreate != "npm install && npm run build" {
		t.Errorf("OnCreate = %q, want joined commands", pf.OnCreate)
	}
}

func TestFindDevContainer(t *testing.T) {
	dir := t.TempDir()

	// .devcontainer/devcontainer.json takes priority
	dcDir := filepath.Join(dir, ".devcontainer")
	if err := os.MkdirAll(dcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dcDir, "devcontainer.json"), []byte(`{"image":"found"}`), 0644); err != nil {
		t.Fatal(err)
	}

	path, err := FindDevContainer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(path)) != ".devcontainer" {
		t.Errorf("should find .devcontainer/devcontainer.json, got %s", path)
	}
}

func TestFindAndReadFallsBackToDevContainer(t *testing.T) {
	dir := t.TempDir()

	// No podfile.yaml, only devcontainer.json
	dcContent := `{"image": "python:3.12", "containerEnv": {"PYTHONPATH": "/app"}}`
	if err := os.WriteFile(filepath.Join(dir, ".devcontainer.json"), []byte(dcContent), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := FindAndRead(dir)
	if err != nil {
		t.Fatalf("FindAndRead should fall back to devcontainer: %v", err)
	}

	// The returned data should be valid podfile YAML
	pf, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("converted devcontainer should be valid podfile: %v", err)
	}
	if pf.Base != "python:3.12" {
		t.Errorf("Base = %q, want python:3.12", pf.Base)
	}
}

func TestPodfileTakesPrecedenceOverDevContainer(t *testing.T) {
	dir := t.TempDir()

	// Both exist; podfile should win
	if err := os.WriteFile(filepath.Join(dir, "podfile.yaml"), []byte("base: golang:1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".devcontainer.json"), []byte(`{"image": "python:3.12"}`), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := FindAndRead(dir)
	if err != nil {
		t.Fatal(err)
	}

	pf, err := Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if pf.Base != "golang:1.22" {
		t.Errorf("Base = %q, want golang:1.22 (podfile should take precedence)", pf.Base)
	}
}

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "line comment",
			input: `{"key": "value" // comment` + "\n}",
			want:  `{"key": "value" ` + "\n}",
		},
		{
			name:  "block comment",
			input: `{"key": /* skip */ "value"}`,
			want:  `{"key":  "value"}`,
		},
		{
			name:  "comment-like inside string",
			input: `{"url": "https://example.com"}`,
			want:  `{"url": "https://example.com"}`,
		},
	}
	for _, tt := range tests {
		got := string(stripJSONComments([]byte(tt.input)))
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
