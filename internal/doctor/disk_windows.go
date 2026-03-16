//go:build windows

package doctor

import "context"

func (c CheckConfig) checkDiskSpace(_ context.Context) Result {
	return Result{"disk space", Warn, "disk space check not available on Windows"}
}
