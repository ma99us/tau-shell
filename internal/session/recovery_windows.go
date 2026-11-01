//go:build windows

package session

import (
	"context"
	"fmt"
	"time"
)

const StartupRecoveryDelay = time.Minute

// RecoverRuntime reconciles abandoned process records consistently for CLI
// and desktop entrypoints without overwriting a newer runtime state.
func RecoverRuntime(ctx context.Context, store *Store, sessionID string) (Runtime, error) {
	for attempt := 0; attempt < 3; attempt++ {
		runtime, err := store.Runtime(ctx, sessionID)
		if err != nil {
			return Runtime{}, err
		}
		switch runtime.Status {
		case StatusStarting:
			if time.Since(runtime.UpdatedAt) < StartupRecoveryDelay {
				return runtime, nil
			}
		case StatusRunning:
			alive, err := RuntimeProcessAlive(runtime)
			if err != nil {
				return Runtime{}, err
			}
			if alive {
				return runtime, nil
			}
		default:
			return runtime, nil
		}

		detail := "TauShell exited unexpectedly; use restore to resume the saved harness session."
		if runtime.Status == StatusStarting {
			detail = "TauShell stopped while starting the session; use start or restore to retry."
		}
		updated, err := store.FailIfCurrent(ctx, runtime, detail)
		if err != nil {
			return Runtime{}, err
		}
		if updated {
			runtime.Status = StatusFailed
			runtime.ProcessID = 0
			runtime.LastError = detail
			return runtime, nil
		}
	}
	return Runtime{}, fmt.Errorf("session %q runtime state changed repeatedly during recovery; retry the command", sessionID)
}
