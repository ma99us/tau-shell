package session

import (
	"os"
	"testing"
	"time"
)

func TestRuntimeProcessAliveRejectsReusedProcessID(t *testing.T) {
	live := Runtime{ProcessID: os.Getpid(), UpdatedAt: time.Now()}
	if alive, err := RuntimeProcessAlive(live); err != nil || !alive {
		t.Fatalf("live runtime = %v, %v; want alive", alive, err)
	}
	// The row predates this process, so the PID now belongs to something else.
	reused := Runtime{ProcessID: os.Getpid(), UpdatedAt: time.Now().Add(-24 * time.Hour)}
	if alive, err := RuntimeProcessAlive(reused); err != nil || alive {
		t.Fatalf("reused-PID runtime = %v, %v; want not alive", alive, err)
	}
	if alive, err := RuntimeProcessAlive(Runtime{}); err != nil || alive {
		t.Fatalf("empty runtime = %v, %v; want not alive", alive, err)
	}
}
