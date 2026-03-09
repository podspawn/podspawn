package podfile

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"

	"github.com/podspawn/podspawn/internal/runtime"
)

// ComputeTag returns a deterministic image tag based on the project name
// and raw Podfile bytes. Format: podspawn/<project>:podfile-<sha256[:12]>.
func ComputeTag(project string, rawBytes []byte) string {
	h := sha256.Sum256(rawBytes)
	return fmt.Sprintf("podspawn/%s:podfile-%x", project, h[:6])
}

// BuildImageFromPodfile builds a Docker image from a Podfile, using the
// raw bytes for cache key computation. Returns the image tag. Skips the
// build if the image already exists (cache hit).
func BuildImageFromPodfile(ctx context.Context, rt runtime.Runtime, pf *Podfile, rawBytes []byte, project string) (string, error) {
	tag := ComputeTag(project, rawBytes)

	exists, err := rt.ImageExists(ctx, tag)
	if err != nil {
		return "", fmt.Errorf("checking cache for %s: %w", tag, err)
	}
	if exists {
		slog.Info("image cache hit", "tag", tag)
		return tag, nil
	}

	dockerfile, err := Generate(pf)
	if err != nil {
		return "", fmt.Errorf("generating dockerfile: %w", err)
	}

	buildCtx, err := createBuildContext(dockerfile)
	if err != nil {
		return "", fmt.Errorf("creating build context: %w", err)
	}

	slog.Info("building image", "tag", tag)
	if err := rt.BuildImage(ctx, buildCtx, tag); err != nil {
		return "", fmt.Errorf("building %s: %w", tag, err)
	}

	return tag, nil
}

func createBuildContext(dockerfile string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	hdr := &tar.Header{
		Name: "Dockerfile",
		Mode: 0644,
		Size: int64(len(dockerfile)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(dockerfile)); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}

	return &buf, nil
}
