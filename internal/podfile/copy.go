package podfile

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/podspawn/podspawn/internal/runtime"
)

// CopyDirToContainer tars the contents of srcDir and copies them into the
// container at destPath using the Docker CopyToContainer API.
func CopyDirToContainer(ctx context.Context, rt runtime.Runtime, containerID, srcDir, destPath string) error {
	tarBuf, err := tarDirectory(srcDir)
	if err != nil {
		return fmt.Errorf("creating tar of %s: %w", srcDir, err)
	}
	if err := rt.CopyToContainer(ctx, containerID, destPath, tarBuf); err != nil {
		return fmt.Errorf("copying to container %s:%s: %w", containerID, destPath, err)
	}
	return nil
}

func tarDirectory(srcDir string) (io.Reader, error) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory for performance
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}

		header, headerErr := tar.FileInfoHeader(info, "")
		if headerErr != nil {
			return headerErr
		}

		relPath, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		header.Name = relPath

		if writeErr := tw.WriteHeader(header); writeErr != nil {
			return writeErr
		}

		if d.IsDir() {
			return nil
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer f.Close() //nolint:errcheck // read-only
		_, copyErr := io.Copy(tw, f)
		return copyErr
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}
