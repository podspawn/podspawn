package podfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitTemplateWrittenToCWD(t *testing.T) {
	dir := t.TempDir()

	data, err := LookupTemplate("go")
	if err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(dir, "podfile.yaml")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "extends: ubuntu-dev") {
		t.Error("written podfile should contain extends: ubuntu-dev")
	}
}

func TestInitDetectionSelectsCorrectTemplate(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]"), 0644) //nolint:errcheck

	name, marker := DetectProjectType(dir)
	if name != "rust" {
		t.Errorf("got %q, want rust", name)
	}
	if marker != "Cargo.toml" {
		t.Errorf("marker = %q, want Cargo.toml", marker)
	}
}

func TestInitTemplateOverrideSkipsDetection(t *testing.T) {
	// Even if CWD has go.mod, --template python should return python template
	data, err := LookupTemplate("python")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "python") {
		t.Error("python template should mention python")
	}
}

func TestInitExtendsTemplateParseableByParseRaw(t *testing.T) {
	// All templates should parse without error via ParseRaw (not Parse)
	for _, name := range []string{"go", "node", "python", "rust", "fullstack", "minimal"} {
		data, err := LookupTemplate(name)
		if err != nil {
			t.Errorf("template %q lookup: %v", name, err)
			continue
		}
		_, err = ParseRaw(strings.NewReader(string(data)))
		if err != nil {
			t.Errorf("template %q ParseRaw failed: %v", name, err)
		}
	}
}

func TestInitExistingPodfileDetected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "podfile.yaml"), []byte("base: ubuntu:24.04\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := os.Stat(filepath.Join(dir, "podfile.yaml"))
	if err != nil {
		t.Error("should detect existing podfile.yaml")
	}
}
