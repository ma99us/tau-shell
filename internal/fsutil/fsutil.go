// Package fsutil holds small file-system helpers shared by TauShell packages.
package fsutil

import (
	"errors"
	"os"
	"path/filepath"
)

// ReplaceFile writes data to a temporary file beside path, syncs it, and
// renames it over path, so a crash cannot leave a truncated file. The parent
// directory must already exist. The temporary file is removed on failure.
func ReplaceFile(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	_, writeErr := temp.Write(data)
	if writeErr == nil {
		writeErr = temp.Sync()
	}
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
