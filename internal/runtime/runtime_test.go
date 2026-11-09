package runtime

import (
	"errors"
	"strings"
	"testing"
)

func fakePowerShell(lookErr error) powershellRuntime {
	return powershellRuntime{
		lookPath: func(name string) (string, error) {
			if lookErr != nil {
				return "", lookErr
			}
			return `C:\pwsh\` + name, nil
		},
		homeDir: func() (string, error) { return `C:\Users\fake`, nil },
	}
}

func TestPowerShellCommandMatchesLegacyLaunch(t *testing.T) {
	runtime := fakePowerShell(nil)
	command, err := runtime.Command(LaunchSpec{Executable: `C:\tools\claude.exe`, Args: []string{"--resume", "abc"}, UTF8: true})
	if err != nil {
		t.Fatal(err)
	}
	if command.Executable != `C:\pwsh\pwsh.exe` {
		t.Fatalf("executable = %q", command.Executable)
	}
	if len(command.Args) != 4 || strings.Join(command.Args[:3], " ") != "-NoLogo -NoProfile -Command" {
		t.Fatalf("args = %q", command.Args)
	}
	if !strings.Contains(command.Args[3], "claude.exe") || !strings.Contains(command.Args[3], "abc") {
		t.Fatalf("script = %q", command.Args[3])
	}
	if runtime.Kind() != PowerShell {
		t.Fatalf("kind = %q", runtime.Kind())
	}
}

func TestPowerShellMissingIsReportedBeforeAndAtLaunch(t *testing.T) {
	runtime := fakePowerShell(errors.New("not found"))
	if err := runtime.Available(); err == nil || !strings.Contains(err.Error(), "PowerShell 7 (pwsh.exe) is required") {
		t.Fatalf("Available() = %v", err)
	}
	if _, err := runtime.Command(LaunchSpec{Executable: "x"}); err == nil {
		t.Fatal("Command() should fail without pwsh")
	}
}

func TestPowerShellHomeDir(t *testing.T) {
	home, err := fakePowerShell(nil).HomeDir()
	if err != nil || home != `C:\Users\fake` {
		t.Fatalf("HomeDir() = %q, %v", home, err)
	}
}
