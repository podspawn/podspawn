//go:build integration

package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
)

func tinyBuildCtx(t *testing.T, dockerfile string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: "Dockerfile", Mode: 0644, Size: int64(len(dockerfile))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write([]byte(dockerfile)); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	return &buf
}

func TestBuildImageBuildxPath(t *testing.T) {
	rt := mustDockerRuntime(t)
	if !rt.buildx {
		t.Skip("docker buildx not available on this host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tag := "podspawn-buildx-smoke:latest"
	t.Cleanup(func() {
		// best effort - rmi isn't on the Runtime interface
		_, _ = rt.cli.ImageRemove(context.Background(), tag, image.RemoveOptions{Force: true})
	})

	df := "FROM alpine:3.20\nRUN echo buildx-path > /smoke.txt\n"
	if err := rt.BuildImage(ctx, tinyBuildCtx(t, df), tag); err != nil {
		t.Fatalf("BuildImage via buildx: %v", err)
	}
	ok, err := rt.ImageExists(ctx, tag)
	if err != nil {
		t.Fatalf("ImageExists: %v", err)
	}
	if !ok {
		t.Fatalf("image %s missing after build (--load may not have worked)", tag)
	}
}

func TestBuildImageSDKPath(t *testing.T) {
	rt := mustDockerRuntime(t)
	// Force the SDK path for this test to verify the fallback still works.
	prev := rt.buildx
	rt.buildx = false
	t.Cleanup(func() { rt.buildx = prev })

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tag := "podspawn-sdk-smoke:latest"
	t.Cleanup(func() {
		_, _ = rt.cli.ImageRemove(context.Background(), tag, image.RemoveOptions{Force: true})
	})

	df := "FROM alpine:3.20\nRUN echo sdk-path > /smoke.txt\n"
	if err := rt.BuildImage(ctx, tinyBuildCtx(t, df), tag); err != nil {
		t.Fatalf("BuildImage via SDK: %v", err)
	}
	ok, err := rt.ImageExists(ctx, tag)
	if err != nil {
		t.Fatalf("ImageExists: %v", err)
	}
	if !ok {
		t.Fatalf("image %s missing after build", tag)
	}
}
