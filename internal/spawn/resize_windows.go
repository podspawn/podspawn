//go:build windows

package spawn

import (
	"context"

	"github.com/podspawn/podspawn/internal/runtime"
)

func handleResize(_ context.Context, _ runtime.Runtime, _ string) {
	// SIGWINCH not available on Windows; terminal resize not supported
}
