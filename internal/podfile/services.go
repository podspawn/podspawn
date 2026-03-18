package podfile

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/podspawn/podspawn/internal/runtime"
)

// On partial failure, already-started services are removed before returning.
func StartServices(ctx context.Context, rt runtime.Runtime, services []ServiceConfig, networkID, sessionPrefix string) ([]string, error) {
	var ids []string

	for _, svc := range services {
		name := sessionPrefix + "-" + svc.Name

		var env []string
		for k, v := range svc.Env {
			env = append(env, k+"="+v)
		}
		sort.Strings(env)

		var mounts []runtime.Mount
		for _, vol := range svc.Volumes {
			parts := strings.SplitN(vol, ":", 2)
			if len(parts) == 2 {
				mounts = append(mounts, runtime.Mount{
					Source: parts[0],
					Target: parts[1],
				})
			} else {
				slog.Warn("skipping malformed volume spec, expected source:target", "volume", vol, "service", svc.Name)
			}
		}

		id, err := rt.CreateContainer(ctx, runtime.ContainerOpts{
			Name:        name,
			Image:       svc.Image,
			NetworkID:   networkID,
			NetworkName: svc.Name,
			Env:         env,
			Mounts:      mounts,
			Labels: map[string]string{
				"managed-by":       "podspawn",
				"podspawn-service": svc.Name,
			},
		})
		if err != nil {
			StopServices(ctx, rt, ids)
			return nil, fmt.Errorf("creating service %s: %w", svc.Name, err)
		}

		if err := rt.StartContainer(ctx, id); err != nil {
			StopServices(ctx, rt, append(ids, id))
			return nil, fmt.Errorf("starting service %s: %w", svc.Name, err)
		}

		ids = append(ids, id)
		slog.Info("started service", "name", svc.Name, "container", name)
	}

	return ids, nil
}

// StopServices removes service containers. Best-effort.
func StopServices(ctx context.Context, rt runtime.Runtime, containerIDs []string) {
	for _, id := range containerIDs {
		if err := rt.RemoveContainer(ctx, id); err != nil {
			slog.Warn("failed to remove service container", "id", id, "error", err)
		}
	}
}
