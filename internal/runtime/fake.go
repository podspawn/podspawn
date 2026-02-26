package runtime

import (
	"context"
	"sync"
	"time"
)

// FakeRuntime records calls for test assertions. All methods are
// safe for concurrent use.
type FakeRuntime struct {
	mu sync.Mutex

	Containers map[string]bool // name → running
	ExecCalls  []FakeExecCall
	ExitCode   int // returned by Exec
	ExecErr    error
	CreateErr  error
}

type FakeExecCall struct {
	ContainerID string
	Opts        ExecOpts
}

func NewFakeRuntime() *FakeRuntime {
	return &FakeRuntime{
		Containers: make(map[string]bool),
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
	f.Containers[opts.Name] = false
	return opts.Name, nil
}

func (f *FakeRuntime) StartContainer(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Containers[id] = true
	return nil
}

func (f *FakeRuntime) Exec(_ context.Context, containerID string, opts ExecOpts) (int, error) {
	f.mu.Lock()
	f.ExecCalls = append(f.ExecCalls, FakeExecCall{ContainerID: containerID, Opts: opts})
	exitCode := f.ExitCode
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
