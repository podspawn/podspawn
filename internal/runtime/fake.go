package runtime

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

type FakeRuntime struct {
	mu sync.Mutex

	Containers  map[string]bool // name → running
	CreateCalls []ContainerOpts
	ExecCalls   []FakeExecCall
	ExitCode    int   // default exit code for Exec
	ExitCodes   []int // per-call exit codes (consumed in order, then falls back to ExitCode)
	ExecErr     error
	CreateErr   error
	StartErr    error

	Images             map[string]bool
	BuildCalls         []string // tags passed to BuildImage
	BuildErr           error
	Networks           map[string]bool
	CreateNetworkCalls []string
	RemoveNetworkCalls []string
	networkCounter     int
}

type FakeExecCall struct {
	ContainerID string
	Opts        ExecOpts
}

func NewFakeRuntime() *FakeRuntime {
	return &FakeRuntime{
		Containers: make(map[string]bool),
		Images:     make(map[string]bool),
		Networks:   make(map[string]bool),
	}
}

var _ Runtime = (*FakeRuntime)(nil)

func (f *FakeRuntime) ContainerExists(_ context.Context, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.Containers[name]
	return ok, nil
}

func (f *FakeRuntime) CreateContainer(_ context.Context, opts ContainerOpts) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CreateErr != nil {
		return "", f.CreateErr
	}
	f.CreateCalls = append(f.CreateCalls, opts)
	f.Containers[opts.Name] = false
	return opts.Name, nil
}

func (f *FakeRuntime) StartContainer(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.StartErr != nil {
		return f.StartErr
	}
	f.Containers[id] = true
	return nil
}

func (f *FakeRuntime) Exec(_ context.Context, containerID string, opts ExecOpts) (int, error) {
	f.mu.Lock()
	f.ExecCalls = append(f.ExecCalls, FakeExecCall{ContainerID: containerID, Opts: opts})
	exitCode := f.ExitCode
	if len(f.ExitCodes) > 0 {
		exitCode = f.ExitCodes[0]
		f.ExitCodes = f.ExitCodes[1:]
	}
	execErr := f.ExecErr
	cb := opts.ExecIDCallback
	f.mu.Unlock()

	if cb != nil {
		cb("fake-exec-id")
	}

	return exitCode, execErr
}

func (f *FakeRuntime) StopContainer(_ context.Context, id string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Containers[id] = false
	return nil
}

func (f *FakeRuntime) RemoveContainer(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Containers, id)
	return nil
}

func (f *FakeRuntime) ResizeExec(_ context.Context, _ string, _, _ uint) error {
	return nil
}

func (f *FakeRuntime) ImageExists(_ context.Context, ref string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Images[ref], nil
}

func (f *FakeRuntime) BuildImage(_ context.Context, _ io.Reader, tag string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.BuildErr != nil {
		return f.BuildErr
	}
	f.BuildCalls = append(f.BuildCalls, tag)
	f.Images[tag] = true
	return nil
}

func (f *FakeRuntime) CreateNetwork(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.networkCounter++
	id := fmt.Sprintf("net-%s-%d", name, f.networkCounter)
	f.Networks[id] = true
	f.CreateNetworkCalls = append(f.CreateNetworkCalls, name)
	return id, nil
}

func (f *FakeRuntime) RemoveNetwork(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Networks, id)
	f.RemoveNetworkCalls = append(f.RemoveNetworkCalls, id)
	return nil
}

func (f *FakeRuntime) ListContainers(_ context.Context, labelFilter map[string]string) ([]ContainerInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []ContainerInfo
	for name, running := range f.Containers {
		labels := map[string]string{"managed-by": "podspawn"}
		match := true
		for k, v := range labelFilter {
			if labels[k] != v {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		state := "running"
		if !running {
			state = "exited"
		}
		result = append(result, ContainerInfo{
			ID:     name,
			Name:   name,
			State:  state,
			Labels: labels,
		})
	}
	return result, nil
}

func (f *FakeRuntime) InspectContainer(_ context.Context, id string) (*ContainerInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	running, ok := f.Containers[id]
	if !ok {
		return nil, nil
	}
	state := "running"
	if !running {
		state = "exited"
	}
	return &ContainerInfo{
		ID:    id,
		Name:  id,
		State: state,
		Labels: map[string]string{
			"managed-by": "podspawn",
		},
	}, nil
}
