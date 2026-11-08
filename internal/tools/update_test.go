package tools

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func TestCheckNpmUpdateUsesDeclaredPackageAndExactAvailableVersion(t *testing.T) {
	spec, _ := Lookup(Codex)
	var args []string
	available, err := CheckNpmUpdate(context.Background(), spec, "0.165.0", func(_ context.Context, got []string) (string, error) {
		args = got
		return "0.166.0\n", nil
	})
	if err != nil || available != "0.166.0" {
		t.Fatalf("available = %q, err = %v", available, err)
	}
	if strings.Join(args, " ") != "view @openai/codex version" {
		t.Fatalf("npm check args = %v", args)
	}
	args = nil
	available, err = CheckNpmUpdate(context.Background(), spec, "0.166.0", func(_ context.Context, got []string) (string, error) {
		args = got
		return "\"0.166.0\"", nil
	})
	if err != nil || available != "" {
		t.Fatalf("no-update result = %q, err = %v", available, err)
	}
	if !reflect.DeepEqual(args, NpmCheckArgs(spec)) {
		t.Fatalf("npm check args = %v, want %v", args, NpmCheckArgs(spec))
	}
}

func TestCheckNpmUpdateReportsRegistryAndVersionFailures(t *testing.T) {
	spec, _ := Lookup(OpenCode)
	if _, err := CheckNpmUpdate(context.Background(), spec, "1.19.5", func(context.Context, []string) (string, error) {
		return "", errors.New("registry unavailable")
	}); err == nil {
		t.Fatal("npm registry failure was swallowed")
	}
	if _, err := CheckNpmUpdate(context.Background(), spec, "1.19.5", func(context.Context, []string) (string, error) {
		return "latest", nil
	}); err == nil {
		t.Fatal("invalid registry version was accepted")
	}
}

func TestNpmVersionComparisonHandlesPrereleases(t *testing.T) {
	for _, test := range []struct {
		available string
		installed string
		want      bool
	}{
		{"1.2.3", "1.2.3-beta.1", true},
		{"1.2.3-beta.2", "1.2.3-beta.1", true},
		{"1.2.3-beta.2", "codex-cli 1.2.3-beta.1", true},
		{"1.2.3-beta.1", "1.2.3", false},
		{"1.2.4", "1.2.3", true},
	} {
		got, err := npmVersionIsNewer(test.available, test.installed)
		if err != nil || got != test.want {
			t.Errorf("npmVersionIsNewer(%q, %q) = %v, %v; want %v", test.available, test.installed, got, err, test.want)
		}
	}
}

func TestReleaseVersionIsNewerIncludesGitWindowsBuild(t *testing.T) {
	if !ReleaseVersionIsNewer("2.56.0.windows.4", "git version 2.56.0.windows.3") {
		t.Fatal("newer Git for Windows build was not detected")
	}
	if ReleaseVersionIsNewer("2.56.0.windows.3", "git version 2.56.0.windows.4") {
		t.Fatal("older Git for Windows build was reported as an update")
	}
	if !ReleaseVersionIsNewer("26.14.0", "v26.13.0") {
		t.Fatal("newer Node release was not detected")
	}
	if !ReleaseVersionsEqual("2.56.0.windows.4", "git version 2.56.0.windows.4") {
		t.Fatal("equivalent Git release formats did not compare equal")
	}
}

func TestNpmUpdateArgsPinScopedPackageVersion(t *testing.T) {
	spec, _ := Lookup(GSD)
	want := []string{"install", "--global", "--prefix", `C:\Users\me\npm`, "@opengsd/gsd-pi@1.23.4"}
	if got := NpmUpdateArgs(spec, "1.23.4", `C:\Users\me\npm`); !reflect.DeepEqual(got, want) {
		t.Fatalf("npm update args = %v, want %v", got, want)
	}
}

func TestUpgradeCancelledDoesNotRunWinget(t *testing.T) {
	ran := false
	installer := Installer{
		LookPath:      func(string) (string, error) { return "winget.exe", nil },
		ConfirmUpdate: func(Spec, string, Status) (bool, error) { return false, nil },
		RunWinget:     func(context.Context, string, []string) error { ran = true; return nil },
	}
	result, err := installer.Upgrade(context.Background(), CopilotCLI, "v1.0.105")
	if err != nil || result.Outcome != InstallCancelled || ran {
		t.Fatalf("result %+v err %v ran %v", result, err, ran)
	}
}

func TestWingetProgressMessageNamesInstallAndUpdate(t *testing.T) {
	powershell, _ := Lookup(PowerShell)
	copilot, _ := Lookup(CopilotCLI)
	for _, test := range []struct {
		args []string
		want string
	}{
		{WingetArgs(powershell), "Installing PowerShell 7. Please wait..."},
		{UpgradeArgs(copilot), "Updating GitHub Copilot CLI. Please wait..."},
		{[]string{"upgrade", "--id", "unknown.package"}, "Running WinGet. Please wait..."},
	} {
		if got := wingetProgressMessage(test.args); got != test.want {
			t.Errorf("wingetProgressMessage(%v) = %q, want %q", test.args, got, test.want)
		}
	}
}
