package runtime

import (
	"context"
	"io"
	"time"
)

type PortBinding struct {
	ContainerPort int
	HostPort      int
	Protocol      string // "tcp" or "udp"
}

type ContainerOpts struct {
	Name        string
	Image       string
	Hostname    string
	User        string
	Cmd         []string
	Env         []string
	Mounts      []Mount
	CPUs        float64
	Memory      int64
	Labels      map[string]string
	NetworkID   string
	NetworkName string // DNS alias on the network

	PortBindings   []PortBinding
	CapDrop        []string
	CapAdd         []string
	SecurityOpt    []string
	PidsLimit      int64
	ReadonlyRootfs bool
	Tmpfs          map[string]string // target -> mount options
	RuntimeName    string            // e.g., "runsc" for gVisor
}

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type ExecOpts struct {
	Cmd        []string
	User       string
	WorkingDir string
	TTY        bool
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Env        []string

	// Called with the exec ID before I/O piping starts, so the
	// caller can set up terminal resize handling while exec runs.
	ExecIDCallback func(execID string)
}

type Runtime interface {
	ContainerExists(ctx context.Context, name string) (bool, error)
	CreateContainer(ctx context.Context, opts ContainerOpts) (string, error)
	StartContainer(ctx context.Context, id string) error
	Exec(ctx context.Context, containerID string, opts ExecOpts) (int, error)
	StopContainer(ctx context.Context, id string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, id string) error
	ResizeExec(ctx context.Context, execID string, height, width uint) error

	BuildImage(ctx context.Context, buildCtx io.Reader, tag string) error
	ImageExists(ctx context.Context, ref string) (bool, error)
	CreateNetwork(ctx context.Context, name string) (string, error)
	RemoveNetwork(ctx context.Context, id string) error
	ListContainers(ctx context.Context, labelFilter map[string]string) ([]ContainerInfo, error)
	InspectContainer(ctx context.Context, id string) (*ContainerInfo, error)
	RemoveVolume(ctx context.Context, name string) error
	CopyToContainer(ctx context.Context, containerID, destPath string, content io.Reader) error
	TagImage(ctx context.Context, source, target string) error
}

type ContainerInfo struct {
	ID     string
	Name   string
	Image  string
	State  string
	Labels map[string]string
}
