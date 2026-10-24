//go:build windows

package tools

import (
	"context"
	"os/exec"
	"testing"
)

func TestDetectInstalledPowerShell(t *testing.T) {
	if _, err := exec.LookPath("pwsh.exe"); err != nil {
		t.Skip("PowerShell 7 is not installed")
	}
	status := Detect(context.Background(), DefaultEnvironment(), mustSpec(t, PowerShell))
	if !status.Installed || status.Version == "" || status.Support == SupportUnknown {
		t.Fatalf("status = %+v", status)
	}
	for _, capability := range status.Capabilities {
		if !capability.Available {
			t.Errorf("installed PowerShell lacks probed option %s; warnings: %v", capability.Option, status.Warnings)
		}
	}
	t.Logf("PowerShell %s (%s) at %s", status.Version, status.Support, status.ExecutablePath)
}
