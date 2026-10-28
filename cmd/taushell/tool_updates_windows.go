//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
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
		return
	}
	found := map[string]string{}
	for _, status := range tools.DetectAll(ctx, tools.DefaultEnvironment()) {
		spec, ok := tools.Lookup(status.ID)
		if !ok || !status.Installed || spec.Install.WingetID == "" {
			continue
		}
		available, err := tools.CheckUpdate(ctx, winget, spec, tools.RunWingetHidden)
		if err != nil {
			diagnostics.Warn("tool update check failed", err, "tool", spec.ID)
			continue
		}
		if available != "" {
			found[string(spec.ID)] = available
		}
	}
	application.updatesMu.Lock()
	application.toolUpdates = found
	application.updatesMu.Unlock()
	diagnostics.Info("tool update check finished", "updates", len(found))
}

// GetToolUpdates returns the newest available version per tool id, for tools
// installed through winget that have one.
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

// UpdateTool upgrades a tool with winget after native confirmation.
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
	return application.installer().Upgrade(ctx, tools.ID(id), available)
}

func (application *App) confirmUpdate(spec tools.Spec, available string) (bool, error) {
	diagnostics.Info("tool update confirmation requested", "tool", spec.ID, "available", available)
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
