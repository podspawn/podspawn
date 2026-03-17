package runtime

import (
	"context"
	"io"
	"time"
)

type ContainerOpts struct {
	Name        string
	Image       string
	Hostname    string
	User        string // container user (e.g., "root" or OS username)
	Cmd         []string
	Env         []string
	Mounts      []Mount
	CPUs        float64
	Memory      int64
	Labels      map[string]string
	NetworkID   string // Docker network to attach to
	NetworkName string // DNS alias on the network

	// Security options
	CapDrop        []string // capabilities to drop (e.g., ["ALL"])
	CapAdd         []string // capabilities to re-add after drop
	SecurityOpt    []string // e.g., ["no-new-privileges:true"]
	PidsLimit      int64    // max PIDs in container (0 = unlimited)
	ReadonlyRootfs bool
	Tmpfs          map[string]string // tmpfs mounts (target -> options)
	RuntimeName    string            // container runtime (e.g., "runsc" for gVisor)
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
	Env    []string // per-exec environment variables

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
