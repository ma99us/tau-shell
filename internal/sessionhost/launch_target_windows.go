package sessionhost

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ma99us/tau-shell/internal/conpty"
	"github.com/ma99us/tau-shell/internal/harness"
	hostruntime "github.com/ma99us/tau-shell/internal/runtime"
	"github.com/ma99us/tau-shell/internal/session"
)

// launchTarget is where and how one launch runs, decided once per launch.
type launchTarget struct {
	runtime    hostruntime.Runtime
	executable string
	// hostDirectory is the Windows directory the host process starts in.
	hostDirectory string
	wsl           *hostruntime.WSL
}

// resolveLaunchTarget picks the session's runtime and finds the harness for it.
// PowerShell sessions keep the Windows lookup; WSL sessions look inside the
// distro, and harnesses WSL does not support yet fail before any state changes.
func resolveLaunchTarget(ctx context.Context, config session.Config, adapter harness.Adapter) (launchTarget, error) {
	selected, err := hostruntime.For(config)
	if err != nil {
		return launchTarget{}, err
	}
	target := launchTarget{runtime: selected, hostDirectory: config.WorkingDir}
	if wsl, ok := selected.(*hostruntime.WSL); ok {
		if adapter.DiscoversSessionID() {
			return launchTarget{}, fmt.Errorf("%s sessions are not supported in WSL yet (conversation discovery reads Windows folders)", config.Tool)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return launchTarget{}, err
		}
		name, err := harness.ExecutableName(config.Tool)
		if err != nil {
			return launchTarget{}, err
		}
		if err := wsl.Available(); err != nil {
			return launchTarget{}, err
		}
		lookupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		path, err := wsl.FindTool(lookupCtx, name)
		if err != nil {
			return launchTarget{}, err
		}
		target.executable, target.hostDirectory, target.wsl = path, home, wsl
		return target, nil
	}
	executable, err := adapter.FindExecutable()
	if err != nil {
		return launchTarget{}, err
	}
	if err := selected.Available(); err != nil {
		return launchTarget{}, err
	}
	target.executable = executable
	return target, nil
}

// childEnvironment builds the host process environment. For WSL it adds the
// launch marker and WSLENV so session variables cross into the distro.
func (target launchTarget) childEnvironment(environment map[string]string, launchToken string) ([]string, error) {
	if target.wsl != nil {
		var err error
		if environment, err = target.wsl.Environment(environment, launchToken); err != nil {
			return nil, err
		}
	}
	return conpty.ChildEnvironment(environment), nil
}

// cleanup returns the post-exit cleanup for this launch, or nil when the job
// object already owns everything the launch started.
func (target launchTarget) cleanup(launchToken string) func() (int, error) {
	if target.wsl == nil {
		return nil
	}
	return func() (int, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return target.wsl.Cleanup(ctx, launchToken)
	}
}

// runCleanup stops leftovers after the host process ended and reports them.
func runCleanup(cleanup func() (int, error)) (note string, err error) {
	if cleanup == nil {
		return "", nil
	}
	count, err := cleanup()
	if err != nil {
		return "", fmt.Errorf("WSL leftover cleanup failed: %w", err)
	}
	if count > 0 {
		return fmt.Sprintf("Stopped %d leftover WSL process(es) after exit.", count), nil
	}
	return "", nil
}
