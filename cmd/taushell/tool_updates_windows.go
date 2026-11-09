//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	hostruntime "github.com/ma99us/tau-shell/internal/runtime"
	"github.com/ma99us/tau-shell/internal/tools"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const toolUpdateInterval = 24 * time.Hour

// startToolUpdateChecks re-checks daily while the app runs; the UI triggers the
// first check on startup (and on Refresh) so the two never overlap. Checks are
// read-only and window-less; nothing is installed without confirmation.
func (application *App) startToolUpdateChecks(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(toolUpdateInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				application.checkToolUpdates(ctx)
			}
		}
	}()
}

func (application *App) checkToolUpdates(ctx context.Context) {
	winget, err := exec.LookPath("winget.exe")
	if err != nil {
		winget = ""
	}
	found := map[string]string{}
	for _, status := range tools.DetectAll(ctx, tools.DefaultEnvironment()) {
		spec, ok := tools.Lookup(status.ID)
		if !ok || !status.Installed {
			continue
		}
		if winget != "" && spec.Install.WingetID != "" {
			available, checkErr := tools.CheckUpdate(ctx, winget, spec, tools.RunWingetHidden)
			if checkErr != nil {
				diagnostics.Warn("tool update check failed", checkErr, "tool", spec.ID)
			} else if available != "" {
				found[string(spec.ID)] = available
			}
		}

		if status.Managed && status.VersionOutput != "" && spec.Install.ManagedStrategy != "" {
			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			available, checkErr := tools.LatestManagedToolVersion(checkCtx, spec)
			cancel()
			if checkErr != nil {
				diagnostics.Warn("managed tool update check failed", checkErr, "tool", spec.ID)
			} else if tools.ReleaseVersionIsNewer(available, status.VersionOutput) {
				found[string(spec.ID)] = available
			}
		}

		if spec.Install.NpmPackage != "" && status.VersionOutput != "" {
			if _, lookupErr := exec.LookPath("npm.cmd"); lookupErr != nil {
				continue
			}
			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			available, checkErr := tools.CheckNpmUpdate(checkCtx, spec, status.VersionOutput, func(runCtx context.Context, args []string) (string, error) {
				return tools.RunNpmOutput(runCtx, args...)
			})
			cancel()
			if checkErr != nil {
				diagnostics.Warn("npm harness update check failed", checkErr, "tool", spec.ID)
			} else if available != "" {
				found[string(spec.ID)] = available
			}
		}
	}
	if latest := checkWSLUpdate(ctx); latest != "" {
		found[wslUpdateKey] = latest
	}
	application.updatesMu.Lock()
	application.toolUpdates = found
	application.updatesMu.Unlock()
	diagnostics.Info("tool update check finished", "updates", len(found))
}

// wslUpdateKey is the toolUpdates entry for WSL, which is not a tools.Spec and
// is updated through InstallWSL rather than UpdateTool.
const wslUpdateKey = "wsl"

// checkWSLUpdate compares the installed WSL with Microsoft's latest stable
// release. It is read-only and returns "" when WSL is absent, current, or the check fails.
func checkWSLUpdate(ctx context.Context) string {
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	status := hostruntime.Detect(checkCtx, nil, nil)
	if !status.Installed {
		return ""
	}
	latest, err := hostruntime.LatestWSLVersion(checkCtx, nil)
	if err != nil {
		diagnostics.Warn("WSL update check failed", err)
		return ""
	}
	if hostruntime.UpdateAvailable(status.Version, latest) {
		return latest
	}
	return ""
}

// GetToolUpdates returns the newest available version per tool id.
func (application *App) GetToolUpdates() map[string]string {
	application.updatesMu.Lock()
	defer application.updatesMu.Unlock()
	copied := make(map[string]string, len(application.toolUpdates))
	for id, version := range application.toolUpdates {
		copied[id] = version
	}
	return copied
}

// CheckToolUpdates re-checks now (the Refresh button) and returns the result.
func (application *App) CheckToolUpdates() map[string]string {
	application.checkToolUpdates(context.Background())
	return application.GetToolUpdates()
}

// UpdateTool upgrades a tool through its declared update strategy after
// native confirmation.
func (application *App) UpdateTool(id string) (result tools.InstallResult, err error) {
	if !application.installMu.TryLock() {
		return tools.InstallResult{}, errors.New("another tool installation or update is already in progress")
	}
	defer application.installMu.Unlock()
	application.updatesMu.Lock()
	available := application.toolUpdates[id]
	application.updatesMu.Unlock()
	if available == "" {
		return tools.InstallResult{}, fmt.Errorf("no update is known for %q", id)
	}
	defer func() {
		if err != nil {
			diagnostics.Error("tool update failed", err, "tool", id)
			return
		}
		diagnostics.Info("tool update finished", "tool", id, "outcome", result.Outcome)
		if result.Outcome == tools.InstallSucceeded {
			application.updatesMu.Lock()
			delete(application.toolUpdates, id)
			application.updatesMu.Unlock()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	return application.installer().Update(ctx, tools.ID(id), available)
}

func (application *App) confirmUpdate(spec tools.Spec, available string, current tools.Status) (bool, error) {
	diagnostics.Info("tool update confirmation requested", "tool", spec.ID, "available", available)
	if spec.Install.ManagedStrategy != "" {
		message := fmt.Sprintf("Update %s to %s using TauShell's per-user managed install?\n\n"+
			"The release archive will be downloaded from its official source, verified, and activated as a new version. The current version will remain in the managed tools folder. No administrator rights or system/user PATH changes are required.",
			spec.Name, available)
		if spec.Install.ManagedStrategy == tools.ManagedNodeLTS {
			message = fmt.Sprintf("Update Node.js to LTS %s?\n\nTauShell downloads the official Node.js ZIP, verifies its signed SHA-256 manifest, and activates the new per-user managed version. Your existing npm global prefix is preserved. No administrator rights or system/user PATH changes are required.", available)
		} else if spec.Install.ManagedStrategy == tools.ManagedPortableGit {
			message = fmt.Sprintf("Update Git for Windows to %s?\n\nTauShell downloads the full x64 PortableGit archive from the official Git for Windows release, verifies its SHA-256 digest, and activates the new per-user managed version. The current version remains available. No administrator rights or system/user PATH changes are required.", available)
		}
		choice, err := application.messageDialog(application.ctx, runtime.MessageDialogOptions{
			Type: runtime.QuestionDialog, Title: "Update " + spec.Name + "?", Message: message,
			Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No",
		})
		if err != nil {
			return false, err
		}
		return strings.EqualFold(choice, "Yes"), nil
	}
	if spec.Install.NpmPackage != "" {
		prefix := filepath.Dir(current.ExecutablePath)
		command := "npm " + strings.Join(tools.NpmUpdateArgs(spec, available, prefix), " ")
		choice, err := application.messageDialog(application.ctx, runtime.MessageDialogOptions{
			Type: runtime.QuestionDialog, Title: "Update " + spec.Name + "?",
			Message: fmt.Sprintf("Update %s to %s with npm?\n\nTauShell will run:\n%s\n\n"+
				"The package will be installed into the same global bin folder as the detected executable. npm may execute package lifecycle scripts as your user. Running %s sessions may be affected; stop them first if the update fails.",
				spec.Name, available, command, spec.Name),
			Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No",
		})
		if err != nil {
			return false, err
		}
		return strings.EqualFold(choice, "Yes"), nil
	}
	command := "winget " + strings.Join(tools.UpgradeArgs(spec), " ")
	choice, err := application.messageDialog(application.ctx, runtime.MessageDialogOptions{
		Type:  runtime.QuestionDialog,
		Title: "Update " + spec.Name + "?",
		Message: fmt.Sprintf("Update %s to %s with winget?\n\nTauShell will run:\n%s\n\n"+
			"winget opens in a separate console window and may ask you to accept package agreements. "+
			"Running %s sessions may block the update; stop them first if it fails.",
			spec.Name, available, command, spec.Name),
		Buttons:       []string{"Yes", "No"},
		DefaultButton: "No",
		CancelButton:  "No",
	})
	if err != nil {
		return false, err
	}
	return strings.EqualFold(choice, "Yes"), nil
}
