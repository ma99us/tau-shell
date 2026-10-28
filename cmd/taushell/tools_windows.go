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

const installTimeout = 30 * time.Minute

// InstallTool installs a declared tool with winget after native confirmation.
// The confirmation is shown by the Go host so a WebView caller cannot bypass it.
func (application *App) InstallTool(id string) (result tools.InstallResult, err error) {
	if !application.installMu.TryLock() {
		return tools.InstallResult{}, errors.New("another tool installation is already in progress")
	}
	defer application.installMu.Unlock()
	defer func() {
		if err != nil {
			diagnostics.Error("tool installation failed", err, "tool", id)
			return
		}
		diagnostics.Info("tool installation finished", "tool", id, "outcome", result.Outcome)
		if result.Outcome == tools.InstallFailed {
			diagnostics.Warn("tool installation could not be verified", errors.New(result.Message), "tool", id)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	return application.installer().Install(ctx, tools.ID(id))
}

func (application *App) installer() tools.Installer {
	if application.toolInstaller != nil {
		return *application.toolInstaller
	}
	return tools.Installer{
		LookPath:    exec.LookPath,
		Confirm:     application.confirmInstall,
		RunWinget:   tools.RunWingetInConsole,
		ConfirmUpdate: application.confirmUpdate,
		RefreshPath: tools.RefreshProcessPath,
		Detect: func(ctx context.Context, spec tools.Spec) tools.Status {
			return tools.Detect(ctx, tools.DefaultEnvironment(), spec)
		},
	}
}

func (application *App) confirmInstall(spec tools.Spec) (bool, error) {
	diagnostics.Info("tool installation confirmation requested", "tool", spec.ID, "package", spec.Install.WingetID)
	choice, err := application.messageDialog(application.ctx, installConfirmation(spec))
	if err != nil {
		return false, err
	}
	confirmed := strings.EqualFold(choice, "Yes")
	if !confirmed {
		diagnostics.Info("tool installation cancelled by user", "tool", spec.ID)
	}
	return confirmed, nil
}

func installConfirmation(spec tools.Spec) runtime.MessageDialogOptions {
	command := "winget " + strings.Join(tools.WingetArgs(spec), " ")
	return runtime.MessageDialogOptions{
		Type:  runtime.QuestionDialog,
		Title: "Install " + spec.Name + "?",
		Message: fmt.Sprintf("Install %s with winget?\n\nTauShell will run:\n%s\n\nPackage: %s\nSource: %s\n\n"+
			"winget opens in a separate console window and may ask you to accept package agreements. "+
			"The installer may request administrator approval. TauShell re-detects %s afterwards.",
			spec.Name, command, spec.Install.WingetID, spec.Install.Source, spec.Name),
		Buttons:       []string{"Yes", "No"},
		DefaultButton: "No",
		CancelButton:  "No",
	}
}

// OpenToolSetupPage opens the declared official setup page for a tool in the
// default browser. Only URLs from tool metadata can be opened.
func (application *App) OpenToolSetupPage(id string) error {
	spec, ok := tools.Lookup(tools.ID(id))
	if !ok {
		err := fmt.Errorf("unknown tool %q", id)
		diagnostics.Error("open tool setup page failed", err)
		return err
	}
	diagnostics.Info("opening official tool setup page", "tool", spec.ID)
	if application.openURL != nil {
		application.openURL(application.ctx, spec.Install.VendorURL)
		return nil
	}
	runtime.BrowserOpenURL(application.ctx, spec.Install.VendorURL)
	return nil
}
