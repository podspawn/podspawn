package podfile

import (
	"strings"
	"testing"
)

func TestGenerateBaseOnly(t *testing.T) {
	pf := &Podfile{Base: "ubuntu:24.04", Shell: "/bin/bash"}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "FROM ubuntu:24.04") {
		t.Error("expected FROM ubuntu:24.04")
	}
	if !strings.Contains(got, "sudo") {
		t.Error("every Podfile image should include sudo")
	}
}

func TestGeneratePackagesOnly(t *testing.T) {
	pf := &Podfile{
		Base:     "ubuntu:24.04",
		Shell:    "/bin/bash",
		Packages: []string{"ripgrep", "fzf"},
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "apt-get install -y") {
		t.Error("expected apt-get install")
	}
	if !strings.Contains(got, "fzf") || !strings.Contains(got, "ripgrep") {
		t.Error("expected both packages")
	}
	if !strings.Contains(got, "rm -rf /var/lib/apt/lists/*") {
		t.Error("expected apt cache cleanup")
	}
}

func TestGenerateVersionedPackages(t *testing.T) {
	pf := &Podfile{
		Base:     "ubuntu:24.04",
		Shell:    "/bin/bash",
		Packages: []string{"nodejs@22", "go@1.22.0", "ripgrep"},
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "nodesource") {
		t.Error("expected nodesource setup for nodejs@22")
	}
	if !strings.Contains(got, "go1.22.0") {
		t.Error("expected go version in tarball URL")
	}
	if !strings.Contains(got, "pipefail") {
		t.Error("expected pipefail SHELL for special installs")
	}
	if !strings.Contains(got, "ripgrep") {
		t.Error("expected ripgrep in apt-get")
	}
}

func TestGenerateEnvSorted(t *testing.T) {
	pf := &Podfile{
		Base:  "ubuntu:24.04",
		Shell: "/bin/bash",
		Env: map[string]string{
			"ZEBRA":  "1",
			"ALPHA":  "2",
			"MIDDLE": "3",
		},
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	alphaIdx := strings.Index(got, "ALPHA")
	middleIdx := strings.Index(got, "MIDDLE")
	zebraIdx := strings.Index(got, "ZEBRA")
	if alphaIdx > middleIdx || middleIdx > zebraIdx {
		t.Errorf("env vars not sorted alphabetically:\n%s", got)
	}
}

func TestGenerateDynamicEnvExcluded(t *testing.T) {
	pf := &Podfile{
		Base:  "ubuntu:24.04",
		Shell: "/bin/bash",
		Env: map[string]string{
			"STATIC":  "value",
			"DYNAMIC": "user-${PODSPAWN_USER}",
		},
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "STATIC") {
		t.Error("static env should be included")
	}
	if strings.Contains(got, "DYNAMIC") {
		t.Error("dynamic env (${PODSPAWN_*}) should be excluded from Dockerfile")
	}
}

func TestGenerateAllDynamicEnv(t *testing.T) {
	pf := &Podfile{
		Base:  "ubuntu:24.04",
		Shell: "/bin/bash",
		Env: map[string]string{
			"ONLY": "${PODSPAWN_USER}",
		},
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "ENV") {
		t.Error("no ENV line expected when all env vars are dynamic")
	}
}

func TestGenerateExtraCommands(t *testing.T) {
	pf := &Podfile{
		Base:          "ubuntu:24.04",
		Shell:         "/bin/bash",
		ExtraCommands: []string{"custom-tool-install", "echo done"},
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "RUN custom-tool-install") {
		t.Error("expected first extra command")
	}
	if !strings.Contains(got, "RUN echo done") {
		t.Error("expected second extra command")
	}
}

func TestGenerateShell(t *testing.T) {
	pf := &Podfile{
		Base:  "ubuntu:24.04",
		Shell: "/bin/zsh",
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `SHELL ["/bin/zsh", "-c"]`) {
		t.Errorf("expected zsh SHELL directive, got:\n%s", got)
	}
}

func TestGenerateShellBashNoDirective(t *testing.T) {
	pf := &Podfile{
		Base:  "ubuntu:24.04",
		Shell: "/bin/bash",
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `SHELL ["/bin/bash", "-c"]`) {
		t.Error("bash shell should not emit SHELL directive (it's the default)")
	}
}

func TestGeneratePorts(t *testing.T) {
	pf := &Podfile{
		Base:  "ubuntu:24.04",
		Shell: "/bin/bash",
		Ports: PortsConfig{Expose: []int{3000, 8080}},
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "EXPOSE 3000 8080") {
		t.Errorf("expected EXPOSE line, got:\n%s", got)
	}
}

func TestGenerateNoPackagesStillIncludesSudo(t *testing.T) {
	pf := &Podfile{
		Base:     "ubuntu:24.04",
		Shell:    "/bin/bash",
		Packages: []string{},
	}
	got, err := Generate(pf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "sudo") {
		t.Error("even with no packages, sudo should be included")
	}
}

func TestGenerateUnsupportedVersionReturnsError(t *testing.T) {
	pf := &Podfile{
		Base:     "ubuntu:24.04",
		Shell:    "/bin/bash",
		Packages: []string{"nodejs@99"},
	}
	_, err := Generate(pf)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	pf := &Podfile{
		Base:     "ubuntu:24.04",
		Shell:    "/bin/bash",
		Packages: []string{"fzf", "ripgrep", "curl"},
		Env:      map[string]string{"B": "2", "A": "1"},
	}
	out1, _ := Generate(pf)
	out2, _ := Generate(pf)
	if out1 != out2 {
		t.Errorf("non-deterministic output:\n--- run 1 ---\n%s\n--- run 2 ---\n%s", out1, out2)
	}
}
