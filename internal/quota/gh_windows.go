//go:build windows

package quota

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const timeout = 15 * time.Second

// GHRunner asks the GitHub CLI for the quota using the user's own sign-in. No
// token is read or stored by TauShell.
func GHRunner(ctx context.Context) ([]byte, error) {
	gh, err := exec.LookPath("gh.exe")
	if err != nil {
		return nil, errors.New("GitHub CLI (gh) not found")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, gh, "api", "/copilot_internal/user")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("gh api failed: %w", err)
	}
	return stdout.Bytes(), nil
}
