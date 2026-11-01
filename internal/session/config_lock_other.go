//go:build !windows

package session

import (
	"path/filepath"
	"strings"
	"sync"
)

var configLocks sync.Map

func lockConfigPath(path string) (func(), error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	lock, _ := configLocks.LoadOrStore(strings.ToLower(filepath.Clean(absolute)), &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock, nil
}
