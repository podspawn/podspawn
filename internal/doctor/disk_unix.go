//go:build !windows

package doctor

import (
	"context"
	"fmt"
	"syscall"
)

func (c CheckConfig) checkDiskSpace(_ context.Context) Result {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/var/lib/podspawn", &stat); err != nil {
		if err := syscall.Statfs("/", &stat); err != nil {
			return Result{"disk space", Warn, "could not check disk space"}
		}
	}
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
	if freeGB < 1.0 {
		return Result{"disk space", Fail, fmt.Sprintf("%.1f GB free (need at least 1 GB for images)", freeGB)}
	}
	if freeGB < 5.0 {
		return Result{"disk space", Warn, fmt.Sprintf("%.1f GB free (recommend 5+ GB for image cache)", freeGB)}
	}
	return Result{"disk space", Pass, fmt.Sprintf("%.1f GB free", freeGB)}
}
