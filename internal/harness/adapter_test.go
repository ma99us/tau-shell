package harness

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ma99us/tau-shell/internal/session"
)

func TestHarnessArgumentsUseExplicitIDs(t *testing.T) {
	for _, test := range []struct {
		harness session.Harness
		start   []string
		resume  []string
	}{
		{session.ClaudeCode, []string{"--session-id", "explicit-id"}, []string{"--resume", "explicit-id"}},
		{session.CopilotCLI, []string{"--session-id", "explicit-id"}, []string{"--resume", "explicit-id"}},
	} {
		adapter, err := For(test.harness)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(adapter.StartArgs("explicit-id"), test.start) {
			t.Fatalf("%s start arguments = %#v", test.harness, adapter.StartArgs("explicit-id"))
		}
		if !reflect.DeepEqual(adapter.ResumeArgs("explicit-id"), test.resume) {
			t.Fatalf("%s resume arguments = %#v", test.harness, adapter.ResumeArgs("explicit-id"))
		}
		for _, arg := range adapter.ResumeArgs("explicit-id") {
			if arg == "--continue" {
				t.Fatalf("%s must not resume an ambiguous most-recent session", test.harness)
			}
		}
	}
}

func TestDefaultLaunchArgsAreAdapterSpecificAndIndependentCopies(t *testing.T) {
	for _, test := range []struct {
		harness session.Harness
		want    []string
	}{
		{session.ClaudeCode, []string{"--dangerously-skip-permissions"}},
		{session.CopilotCLI, []string{"--yolo"}},
	} {
		adapter, err := For(test.harness)
		if err != nil {
			t.Fatal(err)
		}
		got := adapter.DefaultLaunchArgs()
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%s default launch args = %#v, want %#v", test.harness, got, test.want)
		}
		got[0] = "mutated"
		if again := adapter.DefaultLaunchArgs(); reflect.DeepEqual(again, got) {
			t.Fatalf("%s DefaultLaunchArgs leaked internal slice to caller mutation", test.harness)
		}
	}
}

func TestWindowsHarnessLookupUsesPATHEXTResolution(t *testing.T) {
	for harnessID, executable := range map[session.Harness]string{
		session.ClaudeCode: "claude",
		session.CopilotCLI: "copilot",
	} {
		adapter, err := For(harnessID)
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.(cliAdapter).executable; got != executable {
			t.Errorf("%s lookup name = %q, want %q", harnessID, got, executable)
		}
	}
}

func TestPowerShellInvocationQuotesValuesAndKeepsEnvironmentOffTheCommand(t *testing.T) {
	command := PowerShellInvocation(
		"C:\\Program Files\\Agent\\agent.exe",
		[]string{"--session-id", "session-id", "space ' quote"},
		true,
	)
	for _, expected := range []string{
		"'C:\\Program Files\\Agent\\agent.exe'",
		"'space '' quote'",
		"chcp 65001",
	} {
		if !strings.Contains(command, expected) {
			t.Errorf("PowerShell command missing %q: %s", expected, command)
		}
	}
	if strings.Contains(command, "$env:") {
		t.Errorf("command must not set environment values: %s", command)
	}
}

func TestPowerShellInvocationCanLeaveBootstrapEncodingAlone(t *testing.T) {
	command := PowerShellInvocation("copilot.exe", []string{"--resume", "id"}, false)
	if strings.Contains(command, "chcp") || strings.Contains(command, "Encoding") {
		t.Fatalf("unexpected encoding setup in command: %s", command)
	}
}
