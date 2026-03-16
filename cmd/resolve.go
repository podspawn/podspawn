package cmd

import (
	"fmt"
	"strings"

	"github.com/podspawn/podspawn/internal/config"
)

// parseUserHost splits "user@host" into its components.
// Returns an error if the format is invalid.
func parseUserHost(target string) (user, host string, err error) {
	parts := strings.SplitN(target, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected user@host, got %q", target)
	}
	return parts[0], parts[1], nil
}

// appendPodSuffix adds ".pod" to bare hostnames that look like project names
// (no dots, no colons). Fully-qualified hostnames and IPs pass through unchanged.
func appendPodSuffix(host string) string {
	if !strings.Contains(host, ".") && !strings.Contains(host, ":") {
		return host + ".pod"
	}
	return host
}

// resolvePodHost resolves a .pod hostname to a real server address using the
// client config. Non-.pod hostnames pass through unchanged. localhost.pod
// resolves to 127.0.0.1 without requiring config.
func resolvePodHost(host string) (string, error) {
	if !strings.HasSuffix(host, ".pod") {
		return host, nil
	}
	if host == "localhost.pod" {
		return "127.0.0.1", nil
	}

	cfgPath, err := clientConfigPath()
	if err != nil {
		return "", err
	}
	clientCfg, err := config.LoadClient(cfgPath)
	if err != nil {
		return "", fmt.Errorf("resolving %s: configure servers in ~/.podspawn/config.yaml", host)
	}
	return clientCfg.ResolveHost(host)
}
