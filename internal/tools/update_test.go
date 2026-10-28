package tools

import (
	"context"
	"errors"
	"testing"
)

const upgradeListOutput = "Failed when searching source; results will not be included: msstore\r\nName        Id             Version Available Source\r\n---------------------------------------------------\r\nCopilot CLI GitHub.Copilot v1.0.3  v1.0.105  winget\r\n1 upgrades available.\r\n"

func TestParseUpgradeAvailable(t *testing.T) {
	if got := ParseUpgradeAvailable(upgradeListOutput, "GitHub.Copilot"); got != "v1.0.105" {
		t.Fatalf("available = %q", got)
	}
	for _, none := range []string{"No installed package found matching input criteria.", "", "No applicable upgrade found.\n"} {
		if got := ParseUpgradeAvailable(none, "GitHub.Copilot"); got != "" {
			t.Fatalf("%q parsed as %q", none, got)
		}
	}
}

func TestCheckUpdateUsesExactPackageAndReportsFailures(t *testing.T) {
	spec, _ := Lookup(CopilotCLI)
	var args []string
	got, err := CheckUpdate(context.Background(), "winget", spec, func(_ context.Context, _ string, a []string) (string, error) {
		args = a
		return upgradeListOutput, nil
	})
	if err != nil || got != "v1.0.105" || args[2] != "GitHub.Copilot" {
		t.Fatalf("got %q err %v args %v", got, err, args)
	}
	if _, err := CheckUpdate(context.Background(), "winget", spec, func(context.Context, string, []string) (string, error) {
		return "", errors.New("boom")
	}); err == nil {
		t.Fatal("silent failure")
	}
}

func TestUpgradeCancelledDoesNotRunWinget(t *testing.T) {
	ran := false
	installer := Installer{
		LookPath:      func(string) (string, error) { return "winget.exe", nil },
		ConfirmUpdate: func(Spec, string) (bool, error) { return false, nil },
		RunWinget:     func(context.Context, string, []string) error { ran = true; return nil },
	}
	result, err := installer.Upgrade(context.Background(), CopilotCLI, "v1.0.105")
	if err != nil || result.Outcome != InstallCancelled || ran {
		t.Fatalf("result %+v err %v ran %v", result, err, ran)
	}
}
