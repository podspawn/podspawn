package podfile

import "testing"

func TestLoadRegistry(t *testing.T) {
	reg, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if reg.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", reg.SchemaVersion)
	}
	if len(reg.Bases) < 2 {
		t.Errorf("bases count = %d, want >= 2", len(reg.Bases))
	}
	if len(reg.Templates) < 6 {
		t.Errorf("templates count = %d, want >= 6", len(reg.Templates))
	}
}

func TestLookupBase(t *testing.T) {
	data, err := LookupBase("ubuntu-dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("empty base file")
	}
}

func TestLookupBaseUnknown(t *testing.T) {
	_, err := LookupBase("nonexistent")
	if err == nil {
		t.Error("expected error for unknown base")
	}
}

func TestLookupTemplate(t *testing.T) {
	for _, name := range []string{"go", "node", "python", "rust", "fullstack", "minimal"} {
		data, err := LookupTemplate(name)
		if err != nil {
			t.Errorf("template %q: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("template %q: empty", name)
		}
	}
}
