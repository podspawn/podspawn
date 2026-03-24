package podfile

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/podspawn/podspawn/internal/runtime"
)

// ResolvePortBindings determines which ports to forward based on the Podfile
// port configuration and the selected strategy.
//
// Strategies:
//   - "auto": forward to same host port; if taken, pick next available
//   - "manual": only forward ports explicitly in flagPorts
//   - "expose": forward to same host port; fail if taken
//
// Empty strategy defaults to "auto".
func ResolvePortBindings(expose []int, strategy string, flagPorts []int) ([]runtime.PortBinding, error) {
	if strategy == "" {
		strategy = "auto"
	}

	ports := expose
	if strategy == "manual" {
		ports = flagPorts
	} else if len(flagPorts) > 0 {
		seen := map[int]bool{}
		for _, p := range ports {
			seen[p] = true
		}
		for _, p := range flagPorts {
			if !seen[p] {
				ports = append(ports, p)
			}
		}
	}

	if len(ports) == 0 {
		return nil, nil
	}

	var bindings []runtime.PortBinding
	for _, containerPort := range ports {
		hostPort, err := findHostPort(containerPort, strategy)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, runtime.PortBinding{
			ContainerPort: containerPort,
			HostPort:      hostPort,
			Protocol:      "tcp",
		})
	}
	return bindings, nil
}

func findHostPort(preferred int, strategy string) (int, error) {
	if portAvailable(preferred) {
		return preferred, nil
	}

	if strategy == "expose" {
		return 0, fmt.Errorf("port %d already in use (strategy: expose)", preferred)
	}

	// auto: scan for next available
	for candidate := preferred + 1; candidate < preferred+100; candidate++ {
		if portAvailable(candidate) {
			slog.Info("port conflict, using alternative", "wanted", preferred, "using", candidate)
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("no available port near %d", preferred)
}

func portAvailable(port int) bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
