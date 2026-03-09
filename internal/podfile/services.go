package podfile

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/podspawn/podspawn/internal/runtime"
)

// StartServices creates and starts companion service containers on the
// given network. Returns container IDs for later cleanup. On partial
// failure, already-started services are removed before returning the error.
func StartServices(ctx context.Context, rt runtime.Runtime, services []ServiceConfig, networkID, sessionPrefix string) ([]string, error) {
	var ids []string

	for _, svc := range services {
		name := sessionPrefix + "-" + svc.Name

		var env []string
		for k, v := range svc.Env {
			env = append(env, k+"="+v)
		}

		id, err := rt.CreateContainer(ctx, runtime.ContainerOpts{
			Name:        name,
			Image:       svc.Image,
			NetworkID:   networkID,
			NetworkName: svc.Name,
			Env:         env,
			Labels: map[string]string{
				"managed-by":       "podspawn",
				"podspawn-service": svc.Name,
			},
		})
		if err != nil {
			StopServices(ctx, rt, ids)
			return nil, fmt.Errorf("creating service %s: %w", svc.Name, err)
		}

		if err := rt.StartContainer(ctx, name); err != nil {
			StopServices(ctx, rt, append(ids, name))
			return nil, fmt.Errorf("starting service %s: %w", svc.Name, err)
		}

		ids = append(ids, id)
		slog.Info("started service", "name", svc.Name, "container", name)
	}

	return ids, nil
}

// StopServices removes service containers. Best-effort: logs failures
// but continues through the list.
func StopServices(ctx context.Context, rt runtime.Runtime, containerIDs []string) {
	for _, id := range containerIDs {
		if err := rt.RemoveContainer(ctx, id); err != nil {
			slog.Warn("failed to remove service container", "id", id, "error", err)
		}
	}
}
