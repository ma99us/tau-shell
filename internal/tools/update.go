package tools

import (
	"context"
	"fmt"
	"strings"
)

// UpgradeCheckArgs lists only the exact declared package when winget knows a
// newer version. Packages not installed through winget produce no row.
func UpgradeCheckArgs(spec Spec) []string {
	return []string{"list", "--id", spec.Install.WingetID, "--exact", "--source", spec.Install.Source, "--upgrade-available", "--disable-interactivity"}
}

// UpgradeArgs returns the discrete winget arguments for an exact-package upgrade.
func UpgradeArgs(spec Spec) []string {
	return []string{"upgrade", "--id", spec.Install.WingetID, "--exact", "--source", spec.Install.Source}
}

// ParseUpgradeAvailable extracts the available version for packageID from
// `winget list --upgrade-available` output, or "" when there is none. The row
// reads: Name Id Version Available Source, and names may contain spaces.
func ParseUpgradeAvailable(output, packageID string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		for index, field := range fields {
			if strings.EqualFold(field, packageID) && index+2 < len(fields) {
				return fields[index+2]
			}
		}
	}
	return ""
}

// CheckUpdate asks winget, without any visible window, whether a newer version
// of the installed tool is available. run returns winget's combined output.
func CheckUpdate(ctx context.Context, winget string, spec Spec, run func(ctx context.Context, winget string, args []string) (string, error)) (string, error) {
	if spec.Install.WingetID == "" {
		return "", nil
	}
	output, err := run(ctx, winget, UpgradeCheckArgs(spec))
	if err != nil && strings.TrimSpace(output) == "" {
		return "", fmt.Errorf("winget list: %w", err)
	}
	return ParseUpgradeAvailable(output, spec.Install.WingetID), nil
}

// Upgrade runs a user-confirmed winget upgrade and verifies the tool is still
// detectable afterwards. Outcomes reuse the install result type.
func (installer Installer) Upgrade(ctx context.Context, id ID, available string) (InstallResult, error) {
	spec, ok := Lookup(id)
	if !ok || spec.Install.WingetID == "" {
		return InstallResult{}, fmt.Errorf("tool %q cannot be updated with winget", id)
	}
	winget, err := installer.LookPath("winget.exe")
	if err != nil {
		return InstallResult{Outcome: InstallWingetUnavailable, Message: "winget is not available.", VendorURL: spec.Install.VendorURL}, nil
	}
	confirmed, err := installer.ConfirmUpdate(spec, available)
	if err != nil {
		return InstallResult{}, fmt.Errorf("confirm %s update: %w", spec.Name, err)
	}
	if !confirmed {
		return InstallResult{Outcome: InstallCancelled, Message: fmt.Sprintf("Update of %s was cancelled.", spec.Name)}, nil
	}
	runErr := installer.RunWinget(ctx, winget, UpgradeArgs(spec))
	if installer.RefreshPath != nil {
		_ = installer.RefreshPath()
	}
	status := installer.Detect(ctx, spec)
	if status.Usable() {
		message := fmt.Sprintf("%s is now %s.", spec.Name, status.Version)
		if runErr != nil {
			message += fmt.Sprintf(" winget reported: %v.", runErr)
		}
		return InstallResult{Outcome: InstallSucceeded, Message: message, Status: &status}, nil
	}
	problem := "the tool could not be verified after the update"
	if runErr != nil {
		problem = fmt.Sprintf("winget failed: %v", runErr)
	}
	return InstallResult{Outcome: InstallFailed, Message: fmt.Sprintf("%s update problem: %s", spec.Name, problem), Status: &status}, nil
}
