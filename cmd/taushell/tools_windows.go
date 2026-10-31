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
		LookPath:  exec.LookPath,
		Confirm:   application.confirmInstall,
		RunWinget: tools.RunWingetInConsole,
		RunNpm:    tools.RunNpmInConsole,
		RunManaged: func(ctx context.Context, spec tools.Spec) (string, error) {
			layout, err := tools.ManagedToolLayoutFromEnvironment()
			if err != nil {
				return "", err
			}
			return tools.InstallManagedTool(ctx, layout, spec)
		},
		ConfirmUpdate: application.confirmUpdate,
		RefreshPath:   tools.RefreshProcessPath,
		Detect: func(ctx context.Context, spec tools.Spec) tools.Status {
			return tools.Detect(ctx, tools.DefaultEnvironment(), spec)
		},
	}
}

func (application *App) confirmInstall(spec tools.Spec) (bool, error) {
	diagnostics.Info("tool installation confirmation requested", "tool", spec.ID, "package", spec.Install.WingetID, "npm_package", spec.Install.NpmPackage, "managed_strategy", spec.Install.ManagedStrategy)
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
	if spec.Install.ManagedStrategy != "" {
		layout, err := tools.ManagedToolLayoutFromEnvironment()
		destination := `%LOCALAPPDATA%\TauShell\tools`
		if err == nil {
			destination = layout.Root
		}
		title := "Install " + spec.Name + "?"
		message := ""
		switch spec.Install.ManagedStrategy {
		case tools.ManagedNodeLTS:
			title = "Install or update Node.js LTS?"
			message = fmt.Sprintf("Download the current Node.js LTS ZIP from nodejs.org, verify its signed SHA-256 manifest, and extract Node.js plus npm to:\n%s\n\nTauShell will use a per-user npm global prefix. No administrator rights or system/user PATH changes are required.", destination)
		case tools.ManagedPortableGit:
			title = "Install or update Git for Windows?"
			message = fmt.Sprintf("Download the full x64 PortableGit archive from the official Git for Windows release, verify its SHA-256 digest, and extract it to this per-user folder:\n%s\n\nTauShell will add its cmd directory to child-process PATH only. No administrator rights or system/user PATH changes are required.", destination)
		default:
			message = fmt.Sprintf("Install %s using TauShell's per-user managed install.", spec.Name)
		}
		return runtime.MessageDialogOptions{
			Type: runtime.QuestionDialog, Title: title, Message: message,
			Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No",
		}
	}
	if spec.Install.WingetID == "" && spec.Install.NpmPackage != "" {
		command := "npm " + strings.Join(tools.NpmArgs(spec), " ")
		return runtime.MessageDialogOptions{
			Type:  runtime.QuestionDialog,
			Title: "Install " + spec.Name + "?",
			Message: fmt.Sprintf("Install %s with npm?\n\nTauShell will run:\n%s\n\nPackage: %s\nSource: npm registry\n\n"+
				"TauShell runs npm through the detected Node.js/npm pair in a separate console. The package may execute lifecycle scripts as your user. TauShell re-detects %s afterwards.",
				spec.Name, command, spec.Install.NpmPackage, spec.Name),
			Buttons:       []string{"Yes", "No"},
			DefaultButton: "No",
			CancelButton:  "No",
		}
	}
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
