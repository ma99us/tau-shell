package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingTempFile struct {
	*os.File
	writeErr error
	syncErr  error
	closeErr error
}

func (file *failingTempFile) Write(data []byte) (int, error) {
	if file.writeErr != nil {
		return 0, file.writeErr
	}
	return file.File.Write(data)
}

func (file *failingTempFile) Sync() error {
	if file.syncErr != nil {
		return file.syncErr
	}
	return file.File.Sync()
}

func (file *failingTempFile) Close() error {
	return errors.Join(file.File.Close(), file.closeErr)
}

func TestReplaceFileCreatesAndReplacesWithoutLeftovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	for _, content := range []string{"first", "second"} {
		if err := ReplaceFile(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("content = %q, %v; want %q", got, err, content)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory has %d entries (%v), want only the target", len(entries), err)
	}
}

func TestReplaceFileFailsWhenDirectoryIsMissing(t *testing.T) {
	if err := ReplaceFile(filepath.Join(t.TempDir(), "missing", "x"), []byte("x")); err == nil {
		t.Fatal("expected an error for a missing parent directory")
	}
}

func TestReplaceFilePreservesOriginalAndReportsFailureStages(t *testing.T) {
	writeFailure := errors.New("injected write failure")
	syncFailure := errors.New("injected sync failure")
	closeFailure := errors.New("injected close failure")
	renameFailure := errors.New("injected rename failure")
	cleanupFailure := errors.New("injected cleanup failure")
	for _, test := range []struct {
		name        string
		writeErr    error
		syncErr     error
		closeErr    error
		renameErr   error
		cleanupErr  error
		wantMessage string
	}{
		{name: "write", writeErr: writeFailure, wantMessage: writeFailure.Error()},
		{name: "sync", syncErr: syncFailure, wantMessage: syncFailure.Error()},
		{name: "close", closeErr: closeFailure, wantMessage: closeFailure.Error()},
		{name: "rename", renameErr: renameFailure, wantMessage: renameFailure.Error()},
		{name: "cleanup", renameErr: renameFailure, cleanupErr: cleanupFailure, wantMessage: cleanupFailure.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			var tempPath string
			ops := osReplaceFileOps
			ops.createTemp = func(directory, pattern string) (tempFile, error) {
				file, err := os.CreateTemp(directory, pattern)
				if err != nil {
					return nil, err
				}
				tempPath = file.Name()
				return &failingTempFile{File: file, writeErr: test.writeErr, syncErr: test.syncErr, closeErr: test.closeErr}, nil
			}
			ops.rename = func(oldPath, newPath string) error {
				if test.renameErr != nil {
					return test.renameErr
				}
				return os.Rename(oldPath, newPath)
			}
			ops.remove = func(name string) error {
				if test.cleanupErr != nil {
					return test.cleanupErr
				}
				return os.Remove(name)
			}

			err := replaceFileWithOps(path, []byte("replacement"), ops)
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("replacement error = %v, want text %q", err, test.wantMessage)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil || string(got) != "original" {
				t.Fatalf("original changed after failure: %q, %v", got, readErr)
			}
			_, tempErr := os.Stat(tempPath)
			if test.cleanupErr == nil && !errors.Is(tempErr, os.ErrNotExist) {
				t.Fatalf("temporary file remains after cleanup: %v", tempErr)
			}
			if test.cleanupErr != nil {
				if errors.Is(tempErr, os.ErrNotExist) {
					t.Fatal("injected cleanup failure was not observable")
				}
				if cleanupErr := os.Remove(tempPath); cleanupErr != nil {
					t.Fatalf("remove test temporary file: %v", cleanupErr)
				}
			}
		})
	}
}
