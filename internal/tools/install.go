package tools

import (
	"context"
	"fmt"
	"strings"
)

type InstallOutcome string

const (
	InstallSucceeded         InstallOutcome = "installed"
	InstallCancelled         InstallOutcome = "cancelled"
	InstallWingetUnavailable InstallOutcome = "winget-unavailable"
	InstallFailed            InstallOutcome = "failed"
)

type InstallResult struct {
	Outcome   InstallOutcome `json:"outcome"`
	Message   string         `json:"message"`
	VendorURL string         `json:"vendorUrl,omitempty"`
	Status    *Status        `json:"status,omitempty"`
}

// Installer runs a user-confirmed winget installation and verifies the result
// by re-detecting the tool. The installer exit code alone is never treated as
// success.
type Installer struct {
	LookPath  func(string) (string, error)
	Confirm   func(spec Spec) (bool, error)
	RunWinget func(ctx context.Context, winget string, args []string) error
	// ConfirmUpdate asks the user before an upgrade; only Upgrade uses it.
	ConfirmUpdate func(spec Spec, available string) (bool, error)
	// RefreshPath picks up PATH entries added by the installer.
	RefreshPath func() error
	Detect      func(ctx context.Context, spec Spec) Status
}

// WingetArgs returns the discrete winget arguments for an exact package from
// the declared source. No agreement-acceptance or silent flags are added, so
// winget shows its own prompts in the visible installer console.
func WingetArgs(spec Spec) []string {
	return []string{"install", "--id", spec.Install.WingetID, "--exact", "--source", spec.Install.Source}
}

// NpmArgs returns the discrete npm arguments for a global package install.
func NpmArgs(spec Spec) []string {
	return []string{"install", "--global", spec.Install.NpmPackage}
}

// installCommand picks the package manager for spec: winget when a package is
// declared, otherwise npm for npm-distributed harnesses.
func installCommand(spec Spec) (executable, label string, args []string) {
	if spec.Install.WingetID != "" {
		return "winget.exe", "winget", WingetArgs(spec)
	}
	if spec.Install.NpmPackage != "" {
		return "npm.cmd", "npm", NpmArgs(spec)
	}
	return "", "", nil
}

func (installer Installer) Install(ctx context.Context, id ID) (InstallResult, error) {
	spec, ok := Lookup(id)
	if !ok {
		return InstallResult{}, fmt.Errorf("unknown tool %q", id)
	}
	executable, label, args := installCommand(spec)
	if executable == "" {
		return InstallResult{
			Outcome:   InstallWingetUnavailable,
			Message:   fmt.Sprintf("%s has no maintained winget package. Follow the official setup instructions.", spec.Name),
			VendorURL: spec.Install.VendorURL,
		}, nil
	}
	winget, err := installer.LookPath(executable)
	if err != nil {
		return InstallResult{
			Outcome:   InstallWingetUnavailable,
			Message:   fmt.Sprintf("%s is not available. Install %s using the official setup instructions.", label, spec.Name),
			VendorURL: spec.Install.VendorURL,
		}, nil
	}
	confirmed, err := installer.Confirm(spec)
	if err != nil {
		return InstallResult{}, fmt.Errorf("confirm %s installation: %w", spec.Name, err)
	}
	if !confirmed {
		return InstallResult{Outcome: InstallCancelled, Message: fmt.Sprintf("Installation of %s was cancelled.", spec.Name)}, nil
	}

	runErr := installer.RunWinget(ctx, winget, args)
	var refreshErr error
	if installer.RefreshPath != nil {
		refreshErr = installer.RefreshPath()
	}
	status := installer.Detect(ctx, spec)
	if status.Usable() {
		message := fmt.Sprintf("%s %s was detected at %s.", spec.Name, status.Version, status.ExecutablePath)
		if runErr != nil {
			message += fmt.Sprintf(" %s reported: %v.", label, runErr)
		}
		if refreshErr != nil {
			message += fmt.Sprintf(" PATH refresh reported: %v.", refreshErr)
		}
		if len(status.Warnings) > 0 {
			message += " " + strings.Join(status.Warnings, " ")
		}
		return InstallResult{Outcome: InstallSucceeded, Message: message, Status: &status}, nil
	}

	var problems []string
	if runErr != nil {
		problems = append(problems, fmt.Sprintf("%s failed: %v", label, runErr))
	}
	if refreshErr != nil {
		problems = append(problems, fmt.Sprintf("PATH refresh failed: %v", refreshErr))
	}
	if status.Error != "" {
		problems = append(problems, status.Error)
	}
	problems = append(problems, status.Warnings...)
	if len(problems) == 0 {
		problems = append(problems, "the tool could not be verified after installation")
	}
	return InstallResult{
		Outcome:   InstallFailed,
		Message:   fmt.Sprintf("%s could not be verified after installation: %s", spec.Name, strings.Join(problems, "; ")),
		VendorURL: spec.Install.VendorURL,
		Status:    &status,
	}, nil
}

// MergePathEntries appends entries from additions that are not already present
// in current, preserving the existing order. Comparison is case-insensitive
// and ignores trailing separators.
func MergePathEntries(current string, additions ...string) string {
	seen := map[string]struct{}{}
	var merged []string
	add := func(entry string) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return
		}
		key := strings.ToLower(strings.TrimRight(entry, `\/`))
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, entry)
	}
	for _, entry := range strings.Split(current, ";") {
		add(entry)
	}
	for _, addition := range additions {
		for _, entry := range strings.Split(addition, ";") {
			add(entry)
		}
	}
	return strings.Join(merged, ";")
}
