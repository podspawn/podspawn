package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
)

var (
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	red    = color.New(color.FgRed)
	bold   = color.New(color.Bold)
	faint  = color.New(color.Faint)
)

func Success(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(os.Stderr, "%s %s\n", green.Sprint("✓"), msg)
}

func Bold(s string) string  { return bold.Sprint(s) }
func Faint(s string) string { return faint.Sprint(s) }

func ColorStatus(status string) string {
	switch status {
	case "running":
		return green.Sprint(status)
	case "grace_period":
		return yellow.Sprint("grace period")
	default:
		return red.Sprint(status)
	}
}

type Spinner struct {
	msg  string
	done chan struct{}
	once sync.Once
}

func NewSpinner(format string, a ...any) *Spinner {
	s := &Spinner{
		msg:  fmt.Sprintf(format, a...),
		done: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *Spinner) Stop() {
	s.once.Do(func() {
		close(s.done)
		fmt.Fprintf(os.Stderr, "\r%s %s\n", green.Sprint("✓"), s.msg)
	})
}

func (s *Spinner) Fail() {
	s.once.Do(func() {
		close(s.done)
		fmt.Fprintf(os.Stderr, "\r%s %s\n", red.Sprint("✗"), s.msg)
	})
}

func (s *Spinner) run() {
	frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	i := 0
	for {
		select {
		case <-s.done:
			return
		default:
			fmt.Fprintf(os.Stderr, "\r  %c %s", frames[i%len(frames)], s.msg)
			i++
			time.Sleep(80 * time.Millisecond)
		}
	}
}
