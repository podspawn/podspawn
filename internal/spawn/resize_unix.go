//go:build !windows

package spawn

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/podspawn/podspawn/internal/runtime"
	"golang.org/x/term"
)

func handleResize(ctx context.Context, rt runtime.Runtime, execID string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		_ = rt.ResizeExec(ctx, execID, uint(h), uint(w))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			w, h, err := term.GetSize(int(os.Stdin.Fd()))
			if err != nil {
				continue
			}
			_ = rt.ResizeExec(ctx, execID, uint(h), uint(w))
		}
	}
}
