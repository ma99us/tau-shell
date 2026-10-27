package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const claudeHelp = `Options:
  -r, --resume [value]        Resume a conversation by session ID
  --session-id <uuid>         Use a specific session ID for the conversation
  -v, --version               Output the version number`

func fakeEnvironment(path string, outputs map[string]string, failures map[string]error) Environment {
	return Environment{
		LookPath: func(name string) (string, error) {
			if path == "" {
				return "", errors.New("not found")
			}
			return path, nil
		},
		Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			key := strings.Join(args, " ")
			return []byte(outputs[key]), failures[key]
		},
	}
}

func mustSpec(t *testing.T, id ID) Spec {
	t.Helper()
	spec, ok := Lookup(id)
	if !ok {
		t.Fatalf("spec %q not declared", id)
	}
	return spec
}

func TestDeclaredSpecsAreValid(t *testing.T) {
	for _, spec := range Specs() {
		for _, version := range []string{spec.TestedFrom, spec.TestedBefore} {
			if _, ok := ParseVersion(version); !ok {
				t.Errorf("%s: invalid tested version %q", spec.ID, version)
			}
		}
		if spec.Minimum != "" {
			if _, ok := ParseVersion(spec.Minimum); !ok {
				t.Errorf("%s: invalid minimum %q", spec.ID, spec.Minimum)
			}
		}
		if spec.Install.VendorURL == "" || !strings.HasPrefix(spec.Install.VendorURL, "https://") {
			t.Errorf("%s: vendor URL must be https, got %q", spec.ID, spec.Install.VendorURL)
		}
		if spec.Install.WingetID != "" && spec.Install.Source != "winget" {
			t.Errorf("%s: winget package must declare the winget source", spec.ID)
		}
		if len(spec.Capabilities) == 0 {
			t.Errorf("%s: no capabilities declared", spec.ID)
		}
		if spec.ID != PowerShell {
			widgets := spec.Widgets
			if widgets == nil || len(widgets.Commands) == 0 || 			len(widgets.Models) == 0 || len(widgets.InterruptKeys) == 0 || widgets.CompactText == "" {
							t.Errorf("%s: harness must declare widget commands, models, interrupt keys and compact text", spec.ID)
			}
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := map[string]string{
		"2.1.133 (Claude Code)":                               "2.1.133",
		"GitHub Copilot CLI 1.0.100-1.\nRun 'copilot update'": "1.0.100",
		"PowerShell 7.6.7":                                    "7.6.7",
		"v10.20.30":                                           "10.20.30",
	}
	for input, want := range tests {
		version, ok := ParseVersion(input)
		if !ok || version.String() != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %s", input, version, ok, want)
		}
	}
	if _, ok := ParseVersion("no version here 7.6"); ok {
		t.Error("ParseVersion accepted an incomplete version")
	}
}

func TestClassify(t *testing.T) {
	powershell := mustSpec(t, PowerShell)
	claude := mustSpec(t, ClaudeCode)
	tests := []struct {
		spec    Spec
		version string
		want    Support
	}{
		{powershell, "6.2.7", SupportBelowMinimum},
		{powershell, "7.4.6", SupportUntested},
		{powershell, "7.6.0", SupportTested},
		{powershell, "7.6.7", SupportTested},
		{powershell, "7.7.0", SupportUntested},
		{claude, "2.1.132", SupportUntested},
		{claude, "2.1.133", SupportTested},
		{claude, "2.2.0", SupportUntested},
		{claude, "0.1.0", SupportUntested},
	}
	for _, test := range tests {
		if got := test.spec.Classify(mustParse(test.version)); got != test.want {
			t.Errorf("%s Classify(%s) = %s, want %s", test.spec.ID, test.version, got, test.want)
		}
	}
}

func TestHasOption(t *testing.T) {
	if !HasOption(claudeHelp, "--resume") || !HasOption(claudeHelp, "--session-id") {
		t.Fatal("expected options were not detected")
	}
	if HasOption(claudeHelp, "--session") {
		t.Fatal("option prefix matched a longer option")
	}
	if HasOption("  --no-resume-flag", "--resume") {
		t.Fatal("option matched inside another word")
	}
	if !HasOption("[-NoExit] [-NoLogo] [-NonInteractive]", "-NoLogo") {
		t.Fatal("bracketed PowerShell option was not detected")
	}
	if !HasOption("$ copilot --resume=<session-id>", "--resume") {
		t.Fatal("option with = value was not detected")
	}
}

func TestDetectTestedVersionWithCapabilities(t *testing.T) {
	environment := fakeEnvironment(`C:\tools\claude.exe`, map[string]string{
		"--version": "2.1.133 (Claude Code)\n",
		"--help":    claudeHelp,
	}, nil)
	status := Detect(context.Background(), environment, mustSpec(t, ClaudeCode))
	if !status.Installed || status.ExecutablePath != `C:\tools\claude.exe` || status.Version != "2.1.133" {
		t.Fatalf("status = %+v", status)
	}
	if status.Support != SupportTested || len(status.Warnings) != 0 || !status.Usable() {
		t.Fatalf("expected tested and usable, got %+v", status)
	}
	for _, capability := range status.Capabilities {
		if !capability.Available && !capability.Optional {
			t.Fatalf("capability %s unavailable", capability.Option)
		}
	}
}

func TestDetectMissingOptionalCapabilityIsNotAWarning(t *testing.T) {
	environment := fakeEnvironment(`C:\tools\claude.exe`, map[string]string{
		"--version": "2.1.133 (Claude Code)\n",
		"--help":    claudeHelp,
	}, nil)
	status := Detect(context.Background(), environment, mustSpec(t, ClaudeCode))
	var model *CapabilityStatus
	for index := range status.Capabilities {
		if status.Capabilities[index].Option == "--model" {
			model = &status.Capabilities[index]
		}
	}
	if model == nil || !model.Optional || model.Available {
		t.Fatalf("model capability = %+v", model)
	}
	if !status.Usable() || len(status.Warnings) != 0 {
		t.Fatalf("optional capability affected status: %+v", status)
	}
}

func TestDetectUntestedVersionWarnsButRemainsUsable(t *testing.T) {
	environment := fakeEnvironment(`C:\tools\claude.exe`, map[string]string{
		"--version": "2.3.0 (Claude Code)",
		"--help":    claudeHelp,
	}, nil)
	status := Detect(context.Background(), environment, mustSpec(t, ClaudeCode))
	if status.Support != SupportUntested || !status.Usable() {
		t.Fatalf("status = %+v", status)
	}
	if len(status.Warnings) != 1 || !strings.Contains(status.Warnings[0], "outside the tested range") {
		t.Fatalf("warnings = %v", status.Warnings)
	}
}

func TestDetectMissingCapabilityIsReported(t *testing.T) {
	environment := fakeEnvironment(`C:\tools\copilot.exe`, map[string]string{
		"--version": "GitHub Copilot CLI 1.0.100",
		"--help":    "  --session-id <id>\n",
	}, nil)
	status := Detect(context.Background(), environment, mustSpec(t, CopilotCLI))
	if status.Usable() {
		t.Fatal("tool missing --resume reported usable")
	}
	if !strings.Contains(strings.Join(status.Warnings, " "), "--resume") {
		t.Fatalf("missing capability not reported: %v", status.Warnings)
	}
}

func TestDetectBelowMinimumIsNotUsable(t *testing.T) {
	environment := fakeEnvironment(`C:\pwsh.exe`, map[string]string{
		"--version": "PowerShell 6.2.7",
		"-?":        "-NoLogo -NoProfile -Command",
	}, nil)
	status := Detect(context.Background(), environment, mustSpec(t, PowerShell))
	if status.Support != SupportBelowMinimum || status.Usable() {
		t.Fatalf("status = %+v", status)
	}
}

func TestDetectMissingExecutable(t *testing.T) {
	status := Detect(context.Background(), fakeEnvironment("", nil, nil), mustSpec(t, CopilotCLI))
	if status.Installed || status.Error == "" || status.Usable() || status.Support != SupportUnknown {
		t.Fatalf("status = %+v", status)
	}
	if status.Install.WingetID != "GitHub.Copilot" {
		t.Fatalf("install source not reported: %+v", status.Install)
	}
}

func TestDetectVersionCommandFailure(t *testing.T) {
	environment := fakeEnvironment(`C:\claude.exe`, nil, map[string]error{"--version": errors.New("exit status 1")})
	status := Detect(context.Background(), environment, mustSpec(t, ClaudeCode))
	if !status.Installed || status.Error == "" || status.Usable() {
		t.Fatalf("status = %+v", status)
	}
}

func TestDetectHelpProbeFailureIsReported(t *testing.T) {
	environment := fakeEnvironment(`C:\claude.exe`, map[string]string{"--version": "2.1.133"}, map[string]error{"--help": errors.New("timeout")})
	status := Detect(context.Background(), environment, mustSpec(t, ClaudeCode))
	if status.Usable() || !strings.Contains(strings.Join(status.Warnings, " "), "Capability probe failed") {
		t.Fatalf("status = %+v", status)
	}
}

func TestDetectAllPreservesOrder(t *testing.T) {
	statuses := DetectAll(context.Background(), fakeEnvironment("", nil, nil))
	specs := Specs()
	if len(statuses) != len(specs) {
		t.Fatalf("got %d statuses, want %d", len(statuses), len(specs))
	}
	for index := range specs {
		if statuses[index].ID != specs[index].ID {
			t.Fatalf("status %d = %s, want %s", index, statuses[index].ID, specs[index].ID)
		}
	}
}

type installHarness struct {
	wingetPath   string
	confirm      bool
	confirmErr   error
	runErr       error
	detected     Status
	confirmCalls int
	runCalls     int
	runArgs      []string
	refreshCalls int
	detectCalls  int
}

func (harness *installHarness) installer() Installer {
	return Installer{
		LookPath: func(string) (string, error) {
			if harness.wingetPath == "" {
				return "", errors.New("not found")
			}
			return harness.wingetPath, nil
		},
		Confirm: func(Spec) (bool, error) {
			harness.confirmCalls++
			return harness.confirm, harness.confirmErr
		},
		RunWinget: func(_ context.Context, _ string, args []string) error {
			harness.runCalls++
			harness.runArgs = args
			return harness.runErr
		},
		RefreshPath: func() error {
			harness.refreshCalls++
			return nil
		},
		Detect: func(context.Context, Spec) Status {
			harness.detectCalls++
			return harness.detected
		},
	}
}

func usableStatus() Status {
	return Status{Installed: true, ExecutablePath: `C:\bin\copilot.exe`, Version: "1.0.100", Support: SupportTested,
		Capabilities: []CapabilityStatus{{Capability: Capability{Option: "--resume"}, Available: true}}}
}

func TestInstallCancelledDoesNotRunWinget(t *testing.T) {
	harness := &installHarness{wingetPath: `C:\winget.exe`, confirm: false}
	result, err := harness.installer().Install(context.Background(), CopilotCLI)
	if err != nil || result.Outcome != InstallCancelled {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if harness.runCalls != 0 || harness.detectCalls != 0 {
		t.Fatalf("cancelled install ran winget %d times, detect %d times", harness.runCalls, harness.detectCalls)
	}
}

func TestInstallConfirmationErrorDoesNotRunWinget(t *testing.T) {
	harness := &installHarness{wingetPath: `C:\winget.exe`, confirmErr: errors.New("dialog failed")}
	if _, err := harness.installer().Install(context.Background(), CopilotCLI); err == nil {
		t.Fatal("confirmation failure was not returned")
	}
	if harness.runCalls != 0 {
		t.Fatal("winget ran after confirmation failure")
	}
}

func TestInstallWithoutWingetFallsBackToVendor(t *testing.T) {
	harness := &installHarness{}
	result, err := harness.installer().Install(context.Background(), ClaudeCode)
	if err != nil || result.Outcome != InstallWingetUnavailable {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if result.VendorURL != mustSpec(t, ClaudeCode).Install.VendorURL {
		t.Fatalf("vendor URL = %q", result.VendorURL)
	}
	if harness.confirmCalls != 0 || harness.runCalls != 0 {
		t.Fatal("missing winget still prompted or ran an installer")
	}
}

func TestInstallSucceedsOnlyAfterRedetection(t *testing.T) {
	harness := &installHarness{wingetPath: `C:\winget.exe`, confirm: true, detected: usableStatus()}
	result, err := harness.installer().Install(context.Background(), CopilotCLI)
	if err != nil || result.Outcome != InstallSucceeded || result.Status == nil {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if harness.refreshCalls != 1 || harness.detectCalls != 1 {
		t.Fatalf("refresh/detect calls = %d/%d", harness.refreshCalls, harness.detectCalls)
	}
	want := []string{"install", "--id", "GitHub.Copilot", "--exact", "--source", "winget"}
	if strings.Join(harness.runArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("winget args = %v, want %v", harness.runArgs, want)
	}
}

func TestInstallerFailureWithoutDetectionFails(t *testing.T) {
	harness := &installHarness{wingetPath: `C:\winget.exe`, confirm: true, runErr: errors.New("exit status 0x8A15000F"),
		detected: Status{Error: "copilot was not found on PATH"}}
	result, err := harness.installer().Install(context.Background(), CopilotCLI)
	if err != nil || result.Outcome != InstallFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if !strings.Contains(result.Message, "0x8A15000F") || !strings.Contains(result.Message, "not found") || result.VendorURL == "" {
		t.Fatalf("failure message = %q", result.Message)
	}
}

func TestInstallerExitSuccessIsNotTrustedWithoutDetection(t *testing.T) {
	harness := &installHarness{wingetPath: `C:\winget.exe`, confirm: true, detected: Status{Error: "copilot was not found on PATH"}}
	result, err := harness.installer().Install(context.Background(), CopilotCLI)
	if err != nil || result.Outcome != InstallFailed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestInstallerFailureButToolDetectedSucceedsWithNote(t *testing.T) {
	harness := &installHarness{wingetPath: `C:\winget.exe`, confirm: true, runErr: errors.New("already installed"), detected: usableStatus()}
	result, err := harness.installer().Install(context.Background(), CopilotCLI)
	if err != nil || result.Outcome != InstallSucceeded || !strings.Contains(result.Message, "already installed") {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestInstallUnknownTool(t *testing.T) {
	harness := &installHarness{wingetPath: `C:\winget.exe`, confirm: true}
	if _, err := harness.installer().Install(context.Background(), ID("unknown")); err == nil {
		t.Fatal("unknown tool accepted")
	}
}

func TestMergePathEntries(t *testing.T) {
	got := MergePathEntries(`C:\fixture\bin;C:\Windows`, `C:\windows\;C:\Program Files\PowerShell\7`, `C:\Users\me\AppData\Local\Microsoft\WinGet\Links;;`)
	want := `C:\fixture\bin;C:\Windows;C:\Program Files\PowerShell\7;C:\Users\me\AppData\Local\Microsoft\WinGet\Links`
	if got != want {
		t.Fatalf("MergePathEntries = %q, want %q", got, want)
	}
}
