package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
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

func (d *DockerRuntime) Close() error {
	return d.cli.Close()
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
	hostCfg := &container.HostConfig{}
	if opts.CPUs > 0 {
		hostCfg.NanoCPUs = int64(opts.CPUs * 1e9)
	}
	if opts.Memory > 0 {
		hostCfg.Memory = opts.Memory
	}
	for _, m := range opts.Mounts {
		hostCfg.Mounts = append(hostCfg.Mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}
	if len(opts.CapDrop) > 0 {
		hostCfg.CapDrop = opts.CapDrop
	}
	if len(opts.CapAdd) > 0 {
		hostCfg.CapAdd = opts.CapAdd
	}
	if len(opts.SecurityOpt) > 0 {
		hostCfg.SecurityOpt = opts.SecurityOpt
	}
	if opts.PidsLimit > 0 {
		hostCfg.PidsLimit = &opts.PidsLimit
	}
	if opts.ReadonlyRootfs {
		hostCfg.ReadonlyRootfs = true
	}
	if len(opts.Tmpfs) > 0 {
		hostCfg.Tmpfs = opts.Tmpfs
	}
	if opts.RuntimeName != "" {
		hostCfg.Runtime = opts.RuntimeName
	}

	var networkCfg *network.NetworkingConfig
	if opts.NetworkID != "" {
		networkCfg = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				opts.NetworkID: {
					Aliases: []string{opts.NetworkName},
				},
			},
		}
	}

	containerCfg := &container.Config{
		Image:     opts.Image,
		Cmd:       opts.Cmd,
		Env:       opts.Env,
		Labels:    opts.Labels,
		OpenStdin: true,
	}
	if opts.Hostname != "" {
		containerCfg.Hostname = opts.Hostname
	}
	if opts.User != "" {
		containerCfg.User = opts.User
	}

	resp, err := d.cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, opts.Name)
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
		User:         opts.User,
		WorkingDir:   opts.WorkingDir,
		Tty:          opts.TTY,
		AttachStdin:  opts.Stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
		Env:          opts.Env,
	}

	exec, err := d.cli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return -1, fmt.Errorf("creating exec in %s: %w", containerID, err)
	}

	attach, err := d.cli.ContainerExecAttach(ctx, exec.ID, container.ExecAttachOptions{
		Tty: opts.TTY,
	})
	if err != nil {
		return -1, fmt.Errorf("attaching to exec %s: %w", exec.ID, err)
	}
	defer attach.Close()

	if opts.ExecIDCallback != nil {
		opts.ExecIDCallback(exec.ID)
	}

	if opts.Stdin != nil {
		go func() {
			_, _ = io.Copy(attach.Conn, opts.Stdin)
			_ = attach.CloseWrite()
		}()
	}

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

func (d *DockerRuntime) ImageExists(ctx context.Context, ref string) (bool, error) {
	_, err := d.cli.ImageInspect(ctx, ref)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspecting image %s: %w", ref, err)
	}
	return true, nil
}

func (d *DockerRuntime) BuildImage(ctx context.Context, buildCtx io.Reader, tag string) error {
	resp, err := d.cli.ImageBuild(ctx, buildCtx, build.ImageBuildOptions{
		Tags:        []string{tag},
		Remove:      true,
		ForceRemove: true,
	})
	if err != nil {
		return fmt.Errorf("building image %s: %w", tag, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	return consumeBuildOutput(resp.Body)
}

func consumeBuildOutput(r io.Reader) error {
	dec := json.NewDecoder(r)
	for {
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&msg); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		if msg.Error != "" {
			return fmt.Errorf("build error: %s", msg.Error)
		}
		if msg.Stream != "" {
			slog.Debug("docker build", "output", strings.TrimSpace(msg.Stream))
		}
	}
}

func (d *DockerRuntime) CreateNetwork(ctx context.Context, name string) (string, error) {
	resp, err := d.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{"managed-by": "podspawn"},
	})
	if err != nil {
		// If network already exists (e.g., from a crash), reuse it.
		// Only match networks we created, not random user networks with the same name.
		networks, listErr := d.cli.NetworkList(ctx, network.ListOptions{
			Filters: filters.NewArgs(filters.Arg("label", "managed-by=podspawn")),
		})
		if listErr == nil {
			for _, n := range networks {
				if n.Name == name {
					return n.ID, nil
				}
			}
		}
		return "", fmt.Errorf("creating network %s: %w", name, err)
	}
	return resp.ID, nil
}

func (d *DockerRuntime) RemoveNetwork(ctx context.Context, id string) error {
	if err := d.cli.NetworkRemove(ctx, id); err != nil {
		return fmt.Errorf("removing network %s: %w", id, err)
	}
	return nil
}

func (d *DockerRuntime) ListContainers(ctx context.Context, labelFilter map[string]string) ([]ContainerInfo, error) {
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	var result []ContainerInfo
	for _, c := range containers {
		match := true
		for k, v := range labelFilter {
			if c.Labels[k] != v {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		result = append(result, ContainerInfo{
			ID:     c.ID,
			Name:   name,
			Image:  c.Image,
			State:  c.State,
			Labels: c.Labels,
		})
	}
	return result, nil
}

func (d *DockerRuntime) InspectContainer(ctx context.Context, id string) (*ContainerInfo, error) {
	info, err := d.cli.ContainerInspect(ctx, id)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspecting container %s: %w", id, err)
	}
	return &ContainerInfo{
		ID:     info.ID,
		Name:   strings.TrimPrefix(info.Name, "/"),
		Image:  info.Config.Image,
		State:  info.State.Status,
		Labels: info.Config.Labels,
	}, nil
}
