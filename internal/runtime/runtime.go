package runtime

import (
	"context"
	"io"
	"time"
)

type ContainerOpts struct {
	Name        string
	Image       string
	Cmd         []string
	Env         []string
	Mounts      []Mount
	CPUs        float64
	Memory      int64
	Labels      map[string]string
	NetworkID   string // Docker network to attach to
	NetworkName string // DNS alias on the network
}

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type ExecOpts struct {
	Cmd    []string
	TTY    bool
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// ExecIDCallback is called with the exec ID before I/O piping
	// starts. Spawn uses this to set up terminal resize handling
	// while the exec is still running. Nil means no callback.
	ExecIDCallback func(execID string)
}

type Runtime interface {
	ContainerExists(ctx context.Context, name string) (bool, error)
	CreateContainer(ctx context.Context, opts ContainerOpts) (string, error)
	StartContainer(ctx context.Context, id string) error
	Exec(ctx context.Context, containerID string, opts ExecOpts) (int, error) // returns exit code
	StopContainer(ctx context.Context, id string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, id string) error
	ResizeExec(ctx context.Context, execID string, height, width uint) error

	BuildImage(ctx context.Context, buildCtx io.Reader, tag string) error
	ImageExists(ctx context.Context, ref string) (bool, error)
	CreateNetwork(ctx context.Context, name string) (string, error)
	RemoveNetwork(ctx context.Context, id string) error
	ListContainers(ctx context.Context, labelFilter map[string]string) ([]ContainerInfo, error)
	InspectContainer(ctx context.Context, id string) (*ContainerInfo, error)
}

type ContainerInfo struct {
	ID     string
	Name   string
	Image  string
	State  string // "running", "exited", etc.
	Labels map[string]string
}
