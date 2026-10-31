//go:build windows

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/ma99us/tau-shell/internal/tools"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestInstallToolDeclinedConfirmationDoesNotRunWinget(t *testing.T) {
	var shown runtime.MessageDialogOptions
	application := &App{
		showMessageDialog: func(_ context.Context, options runtime.MessageDialogOptions) (string, error) {
			shown = options
			return "No", nil
		},
	}
	installer := application.installer()
	installer.LookPath = func(string) (string, error) { return `C:\winget.exe`, nil }
	installer.RunWinget = func(context.Context, string, []string) error {
		t.Fatal("winget ran after the user declined")
		return nil
	}
	application.toolInstaller = &installer

	result, err := application.InstallTool(string(tools.CopilotCLI))
	if err != nil || result.Outcome != tools.InstallCancelled {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if shown.DefaultButton != "No" || shown.CancelButton != "No" || len(shown.Buttons) != 2 {
		t.Fatalf("confirmation dialog defaults = %+v", shown)
	}
	for _, want := range []string{"GitHub.Copilot", "winget install --id GitHub.Copilot --exact --source winget", "Source: winget"} {
		if !strings.Contains(shown.Message, want) {
			t.Errorf("confirmation message missing %q:\n%s", want, shown.Message)
		}
	}
}

func TestInstallToolRejectsConcurrentInstall(t *testing.T) {
	application := &App{}
	application.installMu.Lock()
	defer application.installMu.Unlock()
	if _, err := application.InstallTool(string(tools.ClaudeCode)); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("concurrent install error = %v", err)
	}
}

func TestOpenToolSetupPageUsesDeclaredURL(t *testing.T) {
	var opened string
	application := &App{openURL: func(_ context.Context, url string) { opened = url }}
	if err := application.OpenToolSetupPage(string(tools.PowerShell)); err != nil {
		t.Fatal(err)
	}
	spec, _ := tools.Lookup(tools.PowerShell)
	if opened != spec.Install.VendorURL {
		t.Fatalf("opened %q, want %q", opened, spec.Install.VendorURL)
	}
	opened = ""
	if err := application.OpenToolSetupPage("https://example.invalid"); err == nil || opened != "" {
		t.Fatalf("unknown tool opened %q, err = %v", opened, err)
	}
}

func TestManagedInstallConfirmationsDescribePerUserNoAdminSource(t *testing.T) {
	for _, test := range []struct {
		id   tools.ID
		want []string
	}{
		{tools.NodeJS, []string{"nodejs.org", "signed SHA-256 manifest", "per-user", "No administrator rights"}},
		{tools.Git, []string{"PortableGit", "SHA-256 digest", "per-user", "No administrator rights"}},
	} {
		t.Run(string(test.id), func(t *testing.T) {
			spec, ok := tools.Lookup(test.id)
			if !ok {
				t.Fatalf("tool %s was not declared", test.id)
			}
			confirmation := installConfirmation(spec)
			if confirmation.DefaultButton != "No" || confirmation.CancelButton != "No" {
				t.Fatalf("confirmation defaults = %+v", confirmation)
			}
			for _, want := range test.want {
				if !strings.Contains(confirmation.Message, want) {
					t.Errorf("confirmation missing %q: %s", want, confirmation.Message)
				}
			}
		})
	}
}

func TestNpmHarnessConfirmationDisclosesLifecycleScripts(t *testing.T) {
	spec, ok := tools.Lookup(tools.GSD)
	if !ok {
		t.Fatal("GSD spec not found")
	}
	confirmation := installConfirmation(spec)
	for _, want := range []string{"npm install --global @opengsd/gsd-pi", "Node.js/npm pair", "lifecycle scripts as your user"} {
		if !strings.Contains(confirmation.Message, want) {
			t.Errorf("npm confirmation missing %q: %s", want, confirmation.Message)
		}
	}
}
