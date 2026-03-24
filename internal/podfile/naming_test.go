package podfile

import "testing"

func TestSessionNameBasic(t *testing.T) {
	name := SessionName("/home/user/code/myapp", "")
	if name == "" {
		t.Fatal("empty session name")
	}
	if name[:5] != "myapp" {
		t.Errorf("expected prefix 'myapp', got %q", name)
	}
}

func TestSessionNameDifferentPathsSameBasename(t *testing.T) {
	a := SessionName("/home/user/code/myapp", "")
	b := SessionName("/home/user/work/myapp", "")
	if a == b {
		t.Errorf("same session name for different paths: %q", a)
	}
}

func TestSessionNameSamePathStable(t *testing.T) {
	a := SessionName("/home/user/code/myapp", "")
	b := SessionName("/home/user/code/myapp", "")
	if a != b {
		t.Errorf("different names for same path: %q vs %q", a, b)
	}
}

func TestSessionNamePodfileOverride(t *testing.T) {
	name := SessionName("/home/user/code/myapp", "backend")
	if name[:7] != "backend" {
		t.Errorf("expected prefix 'backend', got %q", name)
	}
}

func TestSessionNameSanitization(t *testing.T) {
	name := SessionName("/home/user/My App!!", "")
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			t.Errorf("invalid character %q in sanitized name %q", string(r), name)
		}
	}
}
