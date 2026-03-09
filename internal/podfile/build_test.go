package podfile

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/podspawn/podspawn/internal/runtime"
)

func TestComputeTagDeterministic(t *testing.T) {
	raw := []byte("base: ubuntu:24.04\n")
	tag1 := ComputeTag("backend", raw)
	tag2 := ComputeTag("backend", raw)
	if tag1 != tag2 {
		t.Errorf("tags differ: %q vs %q", tag1, tag2)
	}
}

func TestComputeTagChangesWithContent(t *testing.T) {
	tag1 := ComputeTag("backend", []byte("base: ubuntu:24.04\n"))
	tag2 := ComputeTag("backend", []byte("base: node:22\n"))
	if tag1 == tag2 {
		t.Error("different content should produce different tags")
	}
}

func TestComputeTagFormat(t *testing.T) {
	tag := ComputeTag("backend", []byte("base: ubuntu:24.04\n"))
	matched, _ := regexp.MatchString(`^podspawn/backend:podfile-[0-9a-f]{12}$`, tag)
	if !matched {
		t.Errorf("tag format unexpected: %q", tag)
	}
}

func TestBuildImageCacheHit(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	raw := []byte("base: ubuntu:24.04\n")
	tag := ComputeTag("myproject", raw)
	rt.Images[tag] = true

	pf := &Podfile{Base: "ubuntu:24.04", Shell: "/bin/bash"}
	got, err := BuildImageFromPodfile(context.Background(), rt, pf, raw, "myproject")
	if err != nil {
		t.Fatal(err)
	}
	if got != tag {
		t.Errorf("tag = %q, want %q", got, tag)
	}
	if len(rt.BuildCalls) != 0 {
		t.Error("should not build on cache hit")
	}
}

func TestBuildImageCacheMiss(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	raw := []byte("base: ubuntu:24.04\n")
	pf := &Podfile{Base: "ubuntu:24.04", Shell: "/bin/bash"}

	tag, err := BuildImageFromPodfile(context.Background(), rt, pf, raw, "myproject")
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.BuildCalls) != 1 {
		t.Fatalf("expected 1 build call, got %d", len(rt.BuildCalls))
	}
	if rt.BuildCalls[0] != tag {
		t.Errorf("build tag = %q, want %q", rt.BuildCalls[0], tag)
	}
	if !rt.Images[tag] {
		t.Error("image should exist after build")
	}
}

func TestBuildImageError(t *testing.T) {
	rt := runtime.NewFakeRuntime()
	rt.BuildErr = fmt.Errorf("disk full")
	raw := []byte("base: ubuntu:24.04\n")
	pf := &Podfile{Base: "ubuntu:24.04", Shell: "/bin/bash"}

	_, err := BuildImageFromPodfile(context.Background(), rt, pf, raw, "myproject")
	if err == nil {
		t.Fatal("expected error")
	}
}
