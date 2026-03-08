package podfile

import (
	"fmt"
	"strings"
)

type Package struct {
	Name    string
	Version string
}

func ParsePackage(spec string) Package {
	if i := strings.IndexByte(spec, '@'); i > 0 {
		return Package{Name: spec[:i], Version: spec[i+1:]}
	}
	return Package{Name: spec}
}

// knownPackages maps tool name to version to install commands.
// "*" as a version key means any version, with ${VERSION} substituted.
var knownPackages = map[string]map[string][]string{
	"nodejs": {
		"18": {"curl -fsSL https://deb.nodesource.com/setup_18.x | bash -", "apt-get install -y nodejs"},
		"20": {"curl -fsSL https://deb.nodesource.com/setup_20.x | bash -", "apt-get install -y nodejs"},
		"22": {"curl -fsSL https://deb.nodesource.com/setup_22.x | bash -", "apt-get install -y nodejs"},
	},
	"python": {
		"3.11": {"add-apt-repository -y ppa:deadsnakes/ppa", "apt-get install -y python3.11 python3.11-venv"},
		"3.12": {"add-apt-repository -y ppa:deadsnakes/ppa", "apt-get install -y python3.12 python3.12-venv"},
		"3.13": {"add-apt-repository -y ppa:deadsnakes/ppa", "apt-get install -y python3.13 python3.13-venv"},
	},
	"go": {
		"*": {
			"curl -fsSL https://go.dev/dl/go${VERSION}.linux-$(dpkg --print-architecture).tar.gz | tar -C /usr/local -xzf -",
			"ln -sf /usr/local/go/bin/go /usr/local/bin/go",
		},
	},
	"rust": {
		"*": {"curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain ${VERSION}"},
	},
}

// InstallCommands splits parsed packages into plain apt packages and
// special install sequences from the version map. Returns (aptPackages, specialRuns, error).
func InstallCommands(pkgs []Package) ([]string, []string, error) {
	var aptPkgs []string
	var specialRuns []string

	for _, pkg := range pkgs {
		if pkg.Version == "" {
			aptPkgs = append(aptPkgs, pkg.Name)
			continue
		}

		versions, known := knownPackages[pkg.Name]
		if !known {
			// Unknown versioned package: try apt with version glob
			aptPkgs = append(aptPkgs, pkg.Name+"="+pkg.Version+"*")
			continue
		}

		// Exact version match
		if cmds, ok := versions[pkg.Version]; ok {
			specialRuns = append(specialRuns, cmds...)
			continue
		}

		// Wildcard version
		if cmds, ok := versions["*"]; ok {
			for _, cmd := range cmds {
				specialRuns = append(specialRuns, strings.ReplaceAll(cmd, "${VERSION}", pkg.Version))
			}
			continue
		}

		return nil, nil, fmt.Errorf("unsupported version %s@%s; available: %s",
			pkg.Name, pkg.Version, availableVersions(versions))
	}

	return aptPkgs, specialRuns, nil
}

func availableVersions(versions map[string][]string) string {
	var vs []string
	for v := range versions {
		if v != "*" {
			vs = append(vs, v)
		}
	}
	if _, ok := versions["*"]; ok {
		vs = append(vs, "<any>")
	}
	return strings.Join(vs, ", ")
}
