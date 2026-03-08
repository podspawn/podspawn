package podfile

import (
	"strings"
	"testing"
)

func TestParsePackageVersioned(t *testing.T) {
	pkg := ParsePackage("nodejs@22")
	if pkg.Name != "nodejs" || pkg.Version != "22" {
		t.Errorf("got %+v, want {nodejs, 22}", pkg)
	}
}

func TestParsePackageUnversioned(t *testing.T) {
	pkg := ParsePackage("ripgrep")
	if pkg.Name != "ripgrep" || pkg.Version != "" {
		t.Errorf("got %+v, want {ripgrep, ''}", pkg)
	}
}

func TestParsePackageDottedVersion(t *testing.T) {
	pkg := ParsePackage("python@3.12")
	if pkg.Name != "python" || pkg.Version != "3.12" {
		t.Errorf("got %+v, want {python, 3.12}", pkg)
	}
}

func TestInstallCommandsKnownVersioned(t *testing.T) {
	pkgs := []Package{{Name: "nodejs", Version: "22"}}
	apt, special, err := InstallCommands(pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(apt) != 0 {
		t.Errorf("apt packages = %v, want empty", apt)
	}
	if len(special) != 2 {
		t.Fatalf("special runs = %d, want 2", len(special))
	}
	if !strings.Contains(special[0], "nodesource") {
		t.Errorf("expected nodesource setup, got %q", special[0])
	}
}

func TestInstallCommandsUnversioned(t *testing.T) {
	pkgs := []Package{{Name: "ripgrep"}, {Name: "fzf"}}
	apt, special, err := InstallCommands(pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(apt) != 2 || apt[0] != "ripgrep" || apt[1] != "fzf" {
		t.Errorf("apt = %v, want [ripgrep fzf]", apt)
	}
	if len(special) != 0 {
		t.Errorf("special = %v, want empty", special)
	}
}

func TestInstallCommandsWildcardVersion(t *testing.T) {
	pkgs := []Package{{Name: "go", Version: "1.22.0"}}
	apt, special, err := InstallCommands(pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(apt) != 0 {
		t.Errorf("apt = %v, want empty", apt)
	}
	if len(special) != 2 {
		t.Fatalf("special runs = %d, want 2", len(special))
	}
	if !strings.Contains(special[0], "go1.22.0") {
		t.Errorf("expected version substitution, got %q", special[0])
	}
	if strings.Contains(special[0], "${VERSION}") {
		t.Error("${VERSION} placeholder was not substituted")
	}
}

func TestInstallCommandsUnknownVersioned(t *testing.T) {
	pkgs := []Package{{Name: "libcurl", Version: "7.88"}}
	apt, special, err := InstallCommands(pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(apt) != 1 || apt[0] != "libcurl=7.88*" {
		t.Errorf("unknown versioned should fallback to apt glob, got %v", apt)
	}
	if len(special) != 0 {
		t.Errorf("special = %v, want empty", special)
	}
}

func TestInstallCommandsMixed(t *testing.T) {
	pkgs := []Package{
		{Name: "nodejs", Version: "22"},
		{Name: "ripgrep"},
		{Name: "go", Version: "1.23.0"},
	}
	apt, special, err := InstallCommands(pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(apt) != 1 || apt[0] != "ripgrep" {
		t.Errorf("apt = %v, want [ripgrep]", apt)
	}
	// nodejs (2 commands) + go (2 commands) = 4
	if len(special) != 4 {
		t.Errorf("special runs = %d, want 4", len(special))
	}
}

func TestInstallCommandsUnsupportedVersion(t *testing.T) {
	pkgs := []Package{{Name: "nodejs", Version: "99"}}
	_, _, err := InstallCommands(pkgs)
	if err == nil {
		t.Fatal("expected error for unsupported nodejs version")
	}
	if !strings.Contains(err.Error(), "unsupported version nodejs@99") {
		t.Errorf("error = %q", err)
	}
}
