package podfile

import (
	"reflect"
	"strings"
	"testing"
)

func TestMergeScalarOverride(t *testing.T) {
	base := &Podfile{Base: "ubuntu:22.04", Shell: "/bin/bash"}
	child := &RawPodfile{Podfile: Podfile{Base: "ubuntu:24.04"}}

	got := Merge(base, child)
	if got.Base != "ubuntu:24.04" {
		t.Errorf("base = %q, want ubuntu:24.04", got.Base)
	}
	if got.Shell != "/bin/bash" {
		t.Errorf("shell = %q, want /bin/bash (inherited)", got.Shell)
	}
}

func TestMergeScalarEmptyChildPreservesBase(t *testing.T) {
	base := &Podfile{
		Base: "ubuntu:24.04", Shell: "/bin/zsh",
		Mount: "bind", Mode: "persistent", Workspace: "/opt/app",
	}
	child := &RawPodfile{}

	got := Merge(base, child)
	if got.Base != "ubuntu:24.04" {
		t.Errorf("base = %q", got.Base)
	}
	if got.Shell != "/bin/zsh" {
		t.Errorf("shell = %q", got.Shell)
	}
	if got.Mount != "bind" {
		t.Errorf("mount = %q", got.Mount)
	}
	if got.Mode != "persistent" {
		t.Errorf("mode = %q", got.Mode)
	}
	if got.Workspace != "/opt/app" {
		t.Errorf("workspace = %q", got.Workspace)
	}
}

func TestMergePackagesAppend(t *testing.T) {
	base := &Podfile{Packages: []string{"go", "git", "curl"}}
	child := &RawPodfile{Podfile: Podfile{Packages: []string{"ripgrep", "git"}}}

	got := Merge(base, child)
	want := []string{"go", "git", "curl", "ripgrep"}
	if !reflect.DeepEqual(got.Packages, want) {
		t.Errorf("packages = %v, want %v", got.Packages, want)
	}
}

func TestMergePackagesBangReplace(t *testing.T) {
	base := &Podfile{Packages: []string{"go", "git", "curl"}}
	child := &RawPodfile{
		Podfile: Podfile{Packages: []string{"only-this"}},
		Flags:   MergeFlags{PackagesReplace: true},
	}

	got := Merge(base, child)
	want := []string{"only-this"}
	if !reflect.DeepEqual(got.Packages, want) {
		t.Errorf("packages = %v, want %v", got.Packages, want)
	}
}

func TestMergePackagesItemRemoval(t *testing.T) {
	base := &Podfile{Packages: []string{"go", "golangci-lint", "delve"}}
	child := &RawPodfile{
		Podfile: Podfile{Packages: []string{"!golangci-lint", "ripgrep"}},
	}

	got := Merge(base, child)
	want := []string{"go", "delve", "ripgrep"}
	if !reflect.DeepEqual(got.Packages, want) {
		t.Errorf("packages = %v, want %v", got.Packages, want)
	}
}

func TestMergeEnvMerge(t *testing.T) {
	base := &Podfile{Env: map[string]string{"A": "1", "B": "2", "C": "3"}}
	child := &RawPodfile{
		Podfile: Podfile{Env: map[string]string{"B": "override", "D": "new"}},
	}

	got := Merge(base, child)
	if got.Env["A"] != "1" {
		t.Errorf("env A = %q, want 1", got.Env["A"])
	}
	if got.Env["B"] != "override" {
		t.Errorf("env B = %q, want override", got.Env["B"])
	}
	if got.Env["C"] != "3" {
		t.Errorf("env C = %q, want 3", got.Env["C"])
	}
	if got.Env["D"] != "new" {
		t.Errorf("env D = %q, want new", got.Env["D"])
	}
}

func TestMergeEnvRemoval(t *testing.T) {
	base := &Podfile{Env: map[string]string{"A": "1", "B": "2"}}
	child := &RawPodfile{
		Podfile: Podfile{Env: map[string]string{"B": ""}},
	}

	got := Merge(base, child)
	if _, ok := got.Env["B"]; ok {
		t.Errorf("env B should be removed, got %q", got.Env["B"])
	}
	if got.Env["A"] != "1" {
		t.Errorf("env A = %q, want 1", got.Env["A"])
	}
}

func TestMergeEnvBangReplace(t *testing.T) {
	base := &Podfile{Env: map[string]string{"A": "1", "B": "2"}}
	child := &RawPodfile{
		Podfile: Podfile{Env: map[string]string{"X": "new"}},
		Flags:   MergeFlags{EnvReplace: true},
	}

	got := Merge(base, child)
	if len(got.Env) != 1 || got.Env["X"] != "new" {
		t.Errorf("env = %v, want {X: new}", got.Env)
	}
}

func TestMergeHooksConcatenate(t *testing.T) {
	base := &Podfile{OnCreate: "make deps", OnStart: "echo base"}
	child := &RawPodfile{
		Podfile: Podfile{OnCreate: "npm install", OnStart: "echo child"},
	}

	got := Merge(base, child)
	if got.OnCreate != "make deps\nnpm install" {
		t.Errorf("on_create = %q, want concatenated", got.OnCreate)
	}
	if got.OnStart != "echo base\necho child" {
		t.Errorf("on_start = %q, want concatenated", got.OnStart)
	}
}

func TestMergeHooksBangReplace(t *testing.T) {
	base := &Podfile{OnCreate: "make deps"}
	child := &RawPodfile{
		Podfile: Podfile{OnCreate: "only-this"},
		Flags:   MergeFlags{OnCreateReplace: true},
	}

	got := Merge(base, child)
	if got.OnCreate != "only-this" {
		t.Errorf("on_create = %q, want only-this", got.OnCreate)
	}
}

func TestMergeServicesSameName(t *testing.T) {
	base := &Podfile{Services: []ServiceConfig{
		{Name: "pg", Image: "postgres:15", Ports: []int{5432}},
		{Name: "redis", Image: "redis:6"},
	}}
	child := &RawPodfile{Podfile: Podfile{Services: []ServiceConfig{
		{Name: "pg", Image: "postgres:16", Ports: []int{5432, 5433}},
	}}}

	got := Merge(base, child)
	if len(got.Services) != 2 {
		t.Fatalf("services count = %d, want 2", len(got.Services))
	}
	if got.Services[0].Image != "postgres:16" {
		t.Errorf("pg image = %q, want postgres:16", got.Services[0].Image)
	}
	if got.Services[1].Name != "redis" {
		t.Errorf("second service = %q, want redis", got.Services[1].Name)
	}
}

func TestMergeServicesRemoval(t *testing.T) {
	base := &Podfile{Services: []ServiceConfig{
		{Name: "pg", Image: "postgres:16"},
		{Name: "redis", Image: "redis:7"},
	}}
	child := &RawPodfile{Podfile: Podfile{Services: []ServiceConfig{
		{Name: "!redis"},
	}}}

	got := Merge(base, child)
	if len(got.Services) != 1 {
		t.Fatalf("services count = %d, want 1", len(got.Services))
	}
	if got.Services[0].Name != "pg" {
		t.Errorf("remaining service = %q, want pg", got.Services[0].Name)
	}
}

func TestMergeReposSameURL(t *testing.T) {
	base := &Podfile{Repos: []RepoConfig{
		{URL: "gh/co/api", Path: "/w/api", Branch: "main"},
	}}
	child := &RawPodfile{Podfile: Podfile{Repos: []RepoConfig{
		{URL: "gh/co/api", Branch: "develop"},
		{URL: "gh/co/web", Path: "/w/web"},
	}}}

	got := Merge(base, child)
	if len(got.Repos) != 2 {
		t.Fatalf("repos count = %d, want 2", len(got.Repos))
	}
	if got.Repos[0].Branch != "develop" {
		t.Errorf("api branch = %q, want develop", got.Repos[0].Branch)
	}
	if got.Repos[0].Path != "/w/api" {
		t.Errorf("api path = %q, want /w/api (inherited)", got.Repos[0].Path)
	}
}

func TestMergeResourcesPerField(t *testing.T) {
	base := &Podfile{Resources: ResourcesConfig{CPUs: 2, Memory: "4g"}}
	child := &RawPodfile{Podfile: Podfile{Resources: ResourcesConfig{Memory: "16g"}}}

	got := Merge(base, child)
	if got.Resources.CPUs != 2 {
		t.Errorf("cpus = %f, want 2", got.Resources.CPUs)
	}
	if got.Resources.Memory != "16g" {
		t.Errorf("memory = %q, want 16g", got.Resources.Memory)
	}
}

func TestMergeDotfilesReplace(t *testing.T) {
	base := &Podfile{Dotfiles: &DotfilesConfig{Repo: "gh/a/dots", Install: "setup.sh"}}
	child := &RawPodfile{Podfile: Podfile{Dotfiles: &DotfilesConfig{Repo: "gh/b/dots"}}}

	got := Merge(base, child)
	if got.Dotfiles.Repo != "gh/b/dots" {
		t.Errorf("dotfiles repo = %q, want gh/b/dots", got.Dotfiles.Repo)
	}
	if got.Dotfiles.Install != "" {
		t.Errorf("dotfiles install = %q, want empty (full replace)", got.Dotfiles.Install)
	}
}

func TestMergePortsDedup(t *testing.T) {
	base := &Podfile{Ports: PortsConfig{Expose: []int{3000, 8080}}}
	child := &RawPodfile{Podfile: Podfile{Ports: PortsConfig{Expose: []int{8080, 9090}}}}

	got := Merge(base, child)
	want := []int{3000, 8080, 9090}
	if !reflect.DeepEqual(got.Ports.Expose, want) {
		t.Errorf("ports = %v, want %v", got.Ports.Expose, want)
	}
}

func TestMergeNameNotInherited(t *testing.T) {
	base := &Podfile{Name: "base-name"}
	child := &RawPodfile{}

	got := Merge(base, child)
	if got.Name != "" {
		t.Errorf("name = %q, want empty (not inherited)", got.Name)
	}
}

func TestMergeDoesNotMutateInputs(t *testing.T) {
	base := &Podfile{Packages: []string{"go", "git"}, Env: map[string]string{"A": "1"}}
	child := &RawPodfile{Podfile: Podfile{Packages: []string{"npm"}, Env: map[string]string{"B": "2"}}}

	Merge(base, child)

	if len(base.Packages) != 2 {
		t.Errorf("base packages mutated: %v", base.Packages)
	}
	if len(base.Env) != 1 {
		t.Errorf("base env mutated: %v", base.Env)
	}
}

func TestParseRawBangDetection(t *testing.T) {
	input := `
base: ubuntu:24.04
packages!:
  - only-this
env!:
  CLEAN: slate
on_create!: "replaced"
`
	raw, err := ParseRaw(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if !raw.Flags.PackagesReplace {
		t.Error("expected PackagesReplace flag")
	}
	if !raw.Flags.EnvReplace {
		t.Error("expected EnvReplace flag")
	}
	if !raw.Flags.OnCreateReplace {
		t.Error("expected OnCreateReplace flag")
	}
	if len(raw.Packages) != 1 || raw.Packages[0] != "only-this" {
		t.Errorf("packages = %v, want [only-this]", raw.Packages)
	}
}

func TestParseRawUnsupportedBangField(t *testing.T) {
	input := `
base!: ubuntu:24.04
`
	_, err := ParseRaw(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for unsupported bang field")
	}
	if !strings.Contains(err.Error(), "bang-replace not supported") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseRawNoBangFieldsPassThrough(t *testing.T) {
	input := `
base: ubuntu:24.04
packages:
  - go
  - git
`
	raw, err := ParseRaw(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if raw.Flags.PackagesReplace {
		t.Error("PackagesReplace should be false for non-bang packages")
	}
	if len(raw.Packages) != 2 {
		t.Errorf("packages count = %d, want 2", len(raw.Packages))
	}
}
