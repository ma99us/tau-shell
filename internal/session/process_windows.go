package session

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

// pidReuseSlack absorbs clock granularity between process creation and the
// runtime row's update time.
const pidReuseSlack = 2 * time.Second

func IsProcessAlive(processID int) (bool, error) {
	if processID < 1 {
		return false, nil
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(processID))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return false, nil
		}
		return false, fmt.Errorf("open session process %d: %w", processID, err)
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, fmt.Errorf("check session process %d: %w", processID, err)
	}
	switch event {
	case uint32(windows.WAIT_TIMEOUT):
		return true, nil
	case windows.WAIT_OBJECT_0:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected process wait result 0x%x for %d", event, processID)
	}
}

// RuntimeProcessAlive reports whether the process recorded for a running
// session is still the one TauShell launched. A live process created after
// the runtime row was marked running is an unrelated process that reused the
// PID, for example after a reboot.
func RuntimeProcessAlive(runtime Runtime) (bool, error) {
	alive, err := IsProcessAlive(runtime.ProcessID)
	if err != nil || !alive {
		return alive, err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(runtime.ProcessID))
	if err != nil {
		// Identity cannot be checked; keep the session rather than fail a live one.
		return true, nil
	}
	defer windows.CloseHandle(handle)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return true, nil
	}
	started := time.Unix(0, creation.Nanoseconds())
	return !started.After(runtime.UpdatedAt.Add(pidReuseSlack)), nil
}
