//go:build windows

package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

func lockConfigPath(path string) (func(), error) {
	// Windows mutex ownership is thread-specific; pin this goroutine until the
	// returned release function runs so the owner can release it correctly.
	runtime.LockOSThread()
	absolute, err := filepath.Abs(path)
	if err != nil {
		runtime.UnlockOSThread()
		return nil, err
	}
	key := strings.ToLower(filepath.Clean(absolute))
	digest := sha256.Sum256([]byte(key))
	name := `Local\TauShell.SessionConfig.` + hex.EncodeToString(digest[:])
	handle, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(name))
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("create config mutex: %w", err)
	}
	wait, err := windows.WaitForSingleObject(handle, windows.INFINITE)
	if err != nil {
		_ = windows.CloseHandle(handle)
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("wait for config mutex: %w", err)
	}
	if wait != windows.WAIT_OBJECT_0 && wait != windows.WAIT_ABANDONED {
		_ = windows.CloseHandle(handle)
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("wait for config mutex returned %d", wait)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = windows.ReleaseMutex(handle)
			_ = windows.CloseHandle(handle)
			runtime.UnlockOSThread()
		})
	}, nil
}
