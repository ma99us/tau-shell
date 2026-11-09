//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	hostruntime "github.com/ma99us/tau-shell/internal/runtime"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WSLInstallResult reports a WSL install or update attempt. Outcome is one of
// installed, restart-required, cancelled or failed.
type WSLInstallResult struct {
	Outcome string                `json:"outcome"`
	Message string                `json:"message"`
	Status  hostruntime.Readiness `json:"status"`
}

// GetWSLOnlineDistros lists the distributions that can be installed.
func (application *App) GetWSLOnlineDistros() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	names, err := hostruntime.OnlineDistros(ctx, nil)
	if err != nil {
		diagnostics.Error("list installable WSL distros failed", err)
	}
	return names, err
}

// InstallWSL installs WSL, installs a distro, or updates WSL after a native
// confirmation shown by the Go host. TauShell never restarts Windows and never
// elevates itself: wsl.exe asks Windows for approval where it needs it.
func (application *App) InstallWSL(action, distro string) (result WSLInstallResult, err error) {
	if !application.installMu.TryLock() {
		return WSLInstallResult{}, errors.New("another installation is already in progress")
	}
	defer application.installMu.Unlock()
	defer func() {
		if err != nil {
			diagnostics.Error("WSL install failed", err, "action", action)
			return
		}
		diagnostics.Info("WSL install finished", "action", action, "outcome", result.Outcome)
	}()
	args, err := hostruntime.InstallArgs(action, distro)
	if err != nil {
		return WSLInstallResult{}, err
	}
	if action == hostruntime.ActionInstallDistro {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		names, listErr := hostruntime.OnlineDistros(ctx, nil)
		cancel()
		if listErr != nil {
			return WSLInstallResult{}, listErr
		}
		if !containsName(names, distro) {
			return WSLInstallResult{}, fmt.Errorf("%q is not an installable WSL distribution", distro)
		}
	}
	choice, err := application.messageDialog(application.ctx, wslInstallConfirmation(action, distro, args))
	if err != nil {
		return WSLInstallResult{}, err
	}
	if !strings.EqualFold(choice, "Yes") {
		return WSLInstallResult{Outcome: "cancelled", Message: "WSL change cancelled.", Status: detectWSL()}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	if runErr := hostruntime.RunInConsole(ctx, args); runErr != nil {
		return WSLInstallResult{Outcome: "failed", Message: runErr.Error(), Status: detectWSL()}, nil
	}
	return finishWSLInstall(action, distro, detectWSL()), nil
}

func finishWSLInstall(action, distro string, status hostruntime.Readiness) WSLInstallResult {
	result := WSLInstallResult{Status: status}
	switch action {
	case hostruntime.ActionInstallWSL:
		if status.Installed {
			result.Outcome, result.Message = "installed", "WSL is installed. Install a distribution next; restart Windows first if WSL reports that it is required."
		} else {
			result.Outcome, result.Message = "restart-required", "WSL setup finished but WSL is not usable yet. Restart Windows yourself, then press Refresh. TauShell does not restart Windows."
		}
	case hostruntime.ActionInstallDistro:
		if containsDistro(status, distro) {
			result.Outcome, result.Message = "installed", fmt.Sprintf("%s is installed. Open it once (run: wsl -d %s) to create your Linux user before starting sessions in it.", distro, distro)
		} else {
			result.Outcome, result.Message = "failed", fmt.Sprintf("%s was not found after the install. Check the console output, restart Windows if asked, then press Refresh.", distro)
		}
	default:
		if status.Installed {
			result.Outcome, result.Message = "installed", "WSL update finished. Running distros pick it up after wsl --shutdown or a restart."
		} else {
			result.Outcome, result.Message = "failed", "WSL was not detected after the update."
		}
	}
	return result
}

func detectWSL() hostruntime.Readiness {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return hostruntime.Detect(ctx, nil, nil)
}

func containsName(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

func containsDistro(status hostruntime.Readiness, name string) bool {
	for _, distro := range status.Distros {
		if strings.EqualFold(distro.Name, name) {
			return true
		}
	}
	return false
}

func wslInstallConfirmation(action, distro string, args []string) runtime.MessageDialogOptions {
	command := "wsl.exe " + strings.Join(args, " ")
	title, detail := "Update WSL?", "Windows Subsystem for Linux will be updated from Microsoft. Running distributions keep the old version until they are shut down."
	switch action {
	case hostruntime.ActionInstallWSL:
		title, detail = "Install WSL?", "This installs the Windows Subsystem for Linux components (no distribution). Windows will ask for administrator approval, and a Windows restart may be required. TauShell will not restart Windows."
	case hostruntime.ActionInstallDistro:
		title, detail = "Install "+distro+"?", "This downloads the "+distro+" distribution from Microsoft. Windows may ask for administrator approval. You create the Linux user the first time you open it."
	}
	return runtime.MessageDialogOptions{
		Type: runtime.QuestionDialog, Title: title,
		Message:       fmt.Sprintf("%s\n\nTauShell will run, in a separate console:\n%s", detail, command),
		Buttons:       []string{"Yes", "No"},
		DefaultButton: "No",
		CancelButton:  "No",
	}
}
