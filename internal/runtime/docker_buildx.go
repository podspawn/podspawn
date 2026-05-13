package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// hasBuildx reports whether the local `docker buildx` plugin is callable.
// We route BuildImage through buildx when it's available so the currently
// selected buildx builder context is actually used. The Docker Go SDK's
// ImageBuild call goes through the daemon's classic builder regardless of
// which buildx context is active, which silently bypasses any cache the
// buildx context provides (e.g. a Blacksmith remote builder set up by
// useblacksmith/setup-docker-builder@v1).
//
// Set PODSPAWN_DISABLE_BUILDX=1 to force the SDK path.
func hasBuildx() bool {
	if os.Getenv("PODSPAWN_DISABLE_BUILDX") != "" {
		return false
	}
	cmd := exec.Command("docker", "buildx", "version")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// buildImageBuildx streams the tar build context to `docker buildx build` on
// stdin. --load brings the resulting image back into the local daemon so
// subsequent ImageInspect / container creates see it the same way an
// SDK-built image would.
func buildImageBuildx(ctx context.Context, buildCtx io.Reader, tag string) error {
	cmd := exec.CommandContext(ctx, "docker", "buildx", "build",
		"--load",
		"--tag", tag,
		"-",
	)
	cmd.Stdin = buildCtx
	// Buildx writes its build progress to stderr; let it pass through so users
	// (and CI logs) can see what's happening.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("buildx build %s: %w", tag, err)
	}
	return nil
}
