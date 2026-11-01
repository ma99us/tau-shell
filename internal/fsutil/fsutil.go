// Package fsutil holds small file-system helpers shared by TauShell packages.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type tempFile interface {
	Name() string
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type replaceFileOps struct {
	createTemp func(string, string) (tempFile, error)
	rename     func(string, string) error
	remove     func(string) error
}

var osReplaceFileOps = replaceFileOps{
	createTemp: func(directory, pattern string) (tempFile, error) {
		return os.CreateTemp(directory, pattern)
	},
	rename: os.Rename,
	remove: os.Remove,
}

// ReplaceFile writes and syncs a temporary file beside path, then asks the OS
// to rename it over the target. This avoids truncating the target while writing;
// atomic replacement and crash durability depend on the platform/filesystem.
// The parent directory must already exist, and cleanup failures are returned.
func ReplaceFile(path string, data []byte) error {
	return replaceFileWithOps(path, data, osReplaceFileOps)
}

func replaceFileWithOps(path string, data []byte, ops replaceFileOps) error {
	temp, err := ops.createTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	cleanup := func(cause error) error {
		removeErr := ops.remove(name)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return errors.Join(cause, fmt.Errorf("remove temporary file %q: %w", name, removeErr))
		}
		return cause
	}
	written, writeErr := temp.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = temp.Sync()
	}
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		return cleanup(err)
	}
	if err := ops.rename(name, path); err != nil {
		return cleanup(fmt.Errorf("rename temporary file %q over %q: %w", name, path, err))
	}
	return nil
}
