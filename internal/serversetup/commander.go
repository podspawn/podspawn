package serversetup

import "os/exec"

type Commander interface {
	Run(name string, args ...string) error
}

type ExecCommander struct{}

func (ExecCommander) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

type FakeCommander struct {
	Calls   [][]string
	Errors  map[string]error
	CallNum int
	// ErrSequence maps "name" to a slice of errors returned in order per call.
	// If set for a name, overrides Errors.
	ErrSequence map[string][]error
	seqIndex    map[string]int
}

func NewFakeCommander() *FakeCommander {
	return &FakeCommander{
		Errors:      make(map[string]error),
		ErrSequence: make(map[string][]error),
		seqIndex:    make(map[string]int),
	}
}

func (f *FakeCommander) Run(name string, args ...string) error {
	call := make([]string, 0, 1+len(args))
	call = append(call, name)
	call = append(call, args...)
	f.Calls = append(f.Calls, call)

	if seq, ok := f.ErrSequence[name]; ok {
		idx := f.seqIndex[name]
		f.seqIndex[name] = idx + 1
		if idx < len(seq) {
			return seq[idx]
		}
		return nil
	}

	key := name
	if len(args) > 0 {
		key = name + " " + args[0]
	}
	if err, ok := f.Errors[key]; ok {
		return err
	}
	return f.Errors[name]
}
