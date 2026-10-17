package session

import (
	"fmt"

	"golang.org/x/sys/windows"
)

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
