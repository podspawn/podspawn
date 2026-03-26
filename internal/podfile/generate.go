package podfile

import (
	"fmt"
	"sort"
	"strings"
)

// Generate produces a Dockerfile from a Podfile.
// Runtime concerns (repos, dotfiles, services) are handled at container creation.
func Generate(pf *Podfile) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "FROM %s\n", pf.Base)

	var pkgs []Package
	for _, spec := range pf.Packages {
		pkgs = append(pkgs, ParsePackage(spec))
	}

	aptPkgs, specialRuns, err := InstallCommands(pkgs)
	if err != nil {
		return "", err
	}

	// Every Podfile-built image gets sudo so the non-root container user can elevate
	hasSudo := false
	for _, p := range aptPkgs {
		if p == "sudo" {
			hasSudo = true
			break
		}
	}
	if !hasSudo {
		aptPkgs = append(aptPkgs, "sudo")
	}

	needsBash := len(specialRuns) > 0
	if needsBash {
		b.WriteString("\nSHELL [\"/bin/bash\", \"-euo\", \"pipefail\", \"-c\"]\n")
	}

	if len(aptPkgs) > 0 {
		b.WriteString("\nRUN apt-get update && apt-get install -y \\\n")
		sort.Strings(aptPkgs)
		for _, pkg := range aptPkgs {
			fmt.Fprintf(&b, "    %s \\\n", pkg)
		}
		b.WriteString("    && rm -rf /var/lib/apt/lists/*\n")
	}

	for _, cmd := range specialRuns {
		fmt.Fprintf(&b, "\nRUN %s\n", cmd)
	}

	staticEnv := filterStaticEnv(pf.Env)
	if len(staticEnv) > 0 {
		keys := sortedKeys(staticEnv)
		b.WriteString("\n")
		for i, k := range keys {
			prefix := "ENV "
			if i > 0 {
				prefix = "    "
			}
			sep := " \\"
			if i == len(keys)-1 {
				sep = ""
			}
			fmt.Fprintf(&b, "%s%s=%q%s\n", prefix, k, staticEnv[k], sep)
		}
	}

	if pf.Shell != "/bin/bash" && pf.Shell != "" {
		fmt.Fprintf(&b, "\nSHELL [\"%s\", \"-c\"]\n", pf.Shell)
	}

	if len(pf.Ports.Expose) > 0 {
		parts := make([]string, len(pf.Ports.Expose))
		for i, p := range pf.Ports.Expose {
			parts[i] = fmt.Sprintf("%d", p)
		}
		fmt.Fprintf(&b, "\nEXPOSE %s\n", strings.Join(parts, " "))
	}

	for _, cmd := range pf.ExtraCommands {
		fmt.Fprintf(&b, "\nRUN %s\n", cmd)
	}

	return b.String(), nil
}

func filterStaticEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string)
	for k, v := range env {
		if !strings.Contains(v, "${PODSPAWN_") {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
