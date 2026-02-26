package runtime

import (
	"context"
	"fmt"
	"io"
	"time"

	"log/slog"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type DockerRuntime struct {
	cli *client.Client
}

var _ Runtime = (*DockerRuntime)(nil)

func NewDockerRuntime() (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("connecting to docker: %w", err)
	}
	return &DockerRuntime{cli: cli}, nil
}

func (d *DockerRuntime) ContainerExists(ctx context.Context, name string) (bool, error) {
	_, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspecting container %s: %w", name, err)
	}
	return true, nil
}

func (d *DockerRuntime) pullIfMissing(ctx context.Context, ref string) error {
	_, err := d.cli.ImageInspect(ctx, ref)
	if err == nil {
		return nil
	}
	slog.Info("pulling image", "image", ref)
	reader, err := d.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %s: %w", ref, err)
	}
	defer reader.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func (d *DockerRuntime) CreateContainer(ctx context.Context, opts ContainerOpts) (string, error) {
	if err := d.pullIfMissing(ctx, opts.Image); err != nil {
		return "", err
	}
	resp, err := d.cli.ContainerCreate(ctx, &container.Config{
		Image:     opts.Image,
		Cmd:       opts.Cmd,
		OpenStdin: true,
		Tty:       false,
	}, nil, nil, nil, opts.Name)
	if err != nil {
		return "", fmt.Errorf("creating container %s: %w", opts.Name, err)
	}
	return resp.ID, nil
}

func (d *DockerRuntime) StartContainer(ctx context.Context, id string) error {
	if err := d.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container %s: %w", id, err)
	}
	return nil
}

func (d *DockerRuntime) Exec(ctx context.Context, containerID string, opts ExecOpts) (int, error) {
	execCfg := container.ExecOptions{
		Cmd:          opts.Cmd,
		Tty:          opts.TTY,
		AttachStdin:  opts.Stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
	}

	exec, err := d.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return -1, fmt.Errorf("creating exec in %s: %w", containerID, err)
	}

	if opts.ExecIDCallback != nil {
		opts.ExecIDCallback(exec.ID)
	}

	attach, err := d.cli.ContainerExecAttach(ctx, exec.ID, container.ExecAttachOptions{
		Tty: opts.TTY,
	})
	if err != nil {
		return -1, fmt.Errorf("attaching to exec %s: %w", exec.ID, err)
	}
	defer attach.Close()

	if opts.Stdin != nil {
		go func() {
			_, _ = io.Copy(attach.Conn, opts.Stdin)
			_ = attach.CloseWrite()
		}()
	}

	// Wait for output to complete. When the exec process exits,
	// Docker closes the output stream and this returns.
	var outputErr error
	if opts.TTY {
		_, outputErr = io.Copy(opts.Stdout, attach.Reader)
	} else {
		_, outputErr = stdcopy.StdCopy(opts.Stdout, opts.Stderr, attach.Reader)
	}
	if outputErr != nil {
		return -1, fmt.Errorf("reading exec output: %w", outputErr)
	}

	inspect, err := d.cli.ContainerExecInspect(ctx, exec.ID)
	if err != nil {
		return -1, fmt.Errorf("inspecting exec %s: %w", exec.ID, err)
	}
	return inspect.ExitCode, nil
}

func (d *DockerRuntime) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	secs := int(timeout.Seconds())
	if err := d.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &secs}); err != nil {
		return fmt.Errorf("stopping container %s: %w", id, err)
	}
	return nil
}

func (d *DockerRuntime) RemoveContainer(ctx context.Context, id string) error {
	err := d.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("removing container %s: %w", id, err)
	}
	return nil
}

func (d *DockerRuntime) ResizeExec(ctx context.Context, execID string, height, width uint) error {
	return d.cli.ContainerExecResize(ctx, execID, container.ResizeOptions{
		Height: height,
		Width:  width,
	})
}
