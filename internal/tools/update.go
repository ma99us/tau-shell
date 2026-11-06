package tools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var npmVersionPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
var npmVersionOutputPattern = regexp.MustCompile(`v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?`)
var numericVersionPattern = regexp.MustCompile(`[0-9]+`)

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

// NpmCheckArgs asks the registry for the package's current default-tag version.
func NpmCheckArgs(spec Spec) []string {
	return []string{"view", spec.Install.NpmPackage, "version"}
}

// NpmUpdateArgs installs one exact global package version.
func NpmUpdateArgs(spec Spec, version, prefix string) []string {
	return []string{"install", "--global", "--prefix", prefix, spec.Install.NpmPackage + "@" + version}
}

// CheckNpmUpdate compares a declared npm package's latest version with the
// installed harness version. run returns npm's combined output.
func CheckNpmUpdate(ctx context.Context, spec Spec, installed string, run func(context.Context, []string) (string, error)) (string, error) {
	if spec.Install.NpmPackage == "" {
		return "", nil
	}
	output, err := run(ctx, NpmCheckArgs(spec))
	if err != nil {
		return "", fmt.Errorf("npm view: %w", err)
	}
	available := strings.TrimSpace(strings.Trim(strings.TrimSpace(output), `"'`))
	newer, err := npmVersionIsNewer(available, installed)
	if err != nil {
		return "", err
	}
	if !newer {
		return "", nil
	}
	return available, nil
}

func npmVersionIsNewer(available, installed string) (bool, error) {
	availableParts, availablePre, ok := parseNpmVersion(available)
	if !ok {
		return false, fmt.Errorf("npm returned an invalid package version %q", available)
	}
	installedParts, installedPre, ok := parseNpmVersion(installed)
	if !ok {
		return false, fmt.Errorf("installed harness version %q is not a valid npm version", installed)
	}
	for index := range availableParts {
		if availableParts[index] != installedParts[index] {
			return availableParts[index] > installedParts[index], nil
		}
	}
	if availablePre == "" {
		return installedPre != "", nil
	}
	if installedPre == "" {
		return false, nil
	}
	return comparePrerelease(availablePre, installedPre) > 0, nil
}

func parseNpmVersion(version string) ([3]int, string, bool) {
	version = strings.TrimSpace(version)
	match := npmVersionPattern.FindStringSubmatch(version)
	if match == nil {
		versionMatch := npmVersionOutputPattern.FindString(version)
		match = npmVersionPattern.FindStringSubmatch(versionMatch)
	}
	if match == nil {
		return [3]int{}, "", false
	}
	var parts [3]int
	for index := range parts {
		value, err := strconv.Atoi(match[index+1])
		if err != nil {
			return [3]int{}, "", false
		}
		parts[index] = value
	}
	return parts, match[4], true
}

func comparePrerelease(left, right string) int {
	leftParts, rightParts := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < len(leftParts) && index < len(rightParts); index++ {
		leftNumber, leftErr := strconv.Atoi(leftParts[index])
		rightNumber, rightErr := strconv.Atoi(rightParts[index])
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNumber < rightNumber {
				return -1
			}
			if leftNumber > rightNumber {
				return 1
			}
		case leftErr == nil:
			return -1
		case rightErr == nil:
			return 1
		default:
			if leftParts[index] < rightParts[index] {
				return -1
			}
			if leftParts[index] > rightParts[index] {
				return 1
			}
		}
	}
	switch {
	case len(leftParts) < len(rightParts):
		return -1
	case len(leftParts) > len(rightParts):
		return 1
	default:
		return 0
	}
}

// ReleaseVersionIsNewer compares the numeric release components in versions
// such as "v2.56.0.windows.4" and "git version 2.50.1.windows.1".
func ReleaseVersionIsNewer(available, installed string) bool {
	comparison := compareReleaseVersionParts(available, installed)
	return comparison > 0
}

func ReleaseVersionsEqual(left, right string) bool {
	return compareReleaseVersionParts(left, right) == 0
}

func compareReleaseVersionParts(left, right string) int {
	parse := func(value string) []int {
		matches := numericVersionPattern.FindAllString(value, -1)
		parts := make([]int, 0, len(matches))
		for _, match := range matches {
			part, err := strconv.Atoi(match)
			if err != nil {
				return nil
			}
			parts = append(parts, part)
		}
		return parts
	}
	leftParts, rightParts := parse(left), parse(right)
	if len(leftParts) < 3 || len(rightParts) < 3 {
		return -1
	}
	length := len(leftParts)
	if len(rightParts) > length {
		length = len(rightParts)
	}
	for index := 0; index < length; index++ {
		leftPart, rightPart := 0, 0
		if index < len(leftParts) {
			leftPart = leftParts[index]
		}
		if index < len(rightParts) {
			rightPart = rightParts[index]
		}
		if leftPart < rightPart {
			return -1
		}
		if leftPart > rightPart {
			return 1
		}
	}
	return 0
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
	if installer.ConfirmUpdate == nil {
		return InstallResult{}, errors.New("tool update confirmation is not configured")
	}
	confirmed, err := installer.ConfirmUpdate(spec, available, Status{})
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

// Update routes a confirmed version update through the tool's declared
// installer. The available version was discovered before this call.
func (installer Installer) Update(ctx context.Context, id ID, available string) (InstallResult, error) {
	spec, ok := Lookup(id)
	if !ok {
		return InstallResult{}, fmt.Errorf("unknown tool %q", id)
	}
	switch {
	case spec.Install.ManagedStrategy != "":
		return installer.UpgradeManaged(ctx, spec, available)
	case spec.Install.NpmPackage != "":
		return installer.UpgradeNpm(ctx, spec, available)
	default:
		return installer.Upgrade(ctx, id, available)
	}
}

func (installer Installer) UpgradeManaged(ctx context.Context, spec Spec, available string) (InstallResult, error) {
	if installer.Detect == nil {
		return InstallResult{}, errors.New("managed tool cannot be verified")
	}
	current := installer.Detect(ctx, spec)
	if !current.Managed || !current.Installed || current.VersionOutput == "" {
		return InstallResult{}, fmt.Errorf("%s is not a detectable TauShell-managed installation", spec.Name)
	}
	if installer.ConfirmUpdate == nil {
		return InstallResult{}, errors.New("tool update confirmation is not configured")
	}
	confirmed, err := installer.ConfirmUpdate(spec, available, current)
	if err != nil {
		return InstallResult{}, fmt.Errorf("confirm %s update: %w", spec.Name, err)
	}
	if !confirmed {
		return InstallResult{Outcome: InstallCancelled, Message: fmt.Sprintf("Update of %s was cancelled.", spec.Name)}, nil
	}
	if installer.RunManaged == nil {
		return InstallResult{}, fmt.Errorf("no managed installer is configured for %s", spec.Name)
	}
	detail, runErr := installer.RunManaged(ctx, spec, available)
	var refreshErr error
	if installer.RefreshPath != nil {
		refreshErr = installer.RefreshPath()
	}
	status := installer.Detect(ctx, spec)
	if status.Usable() && !ReleaseVersionsEqual(available, status.VersionOutput) {
		problem := fmt.Sprintf("expected %s, detected %s", available, status.VersionOutput)
		if runErr != nil {
			problem += fmt.Sprintf("; managed installer failed: %v", runErr)
		}
		if refreshErr != nil {
			problem += fmt.Sprintf("; PATH refresh failed: %v", refreshErr)
		}
		return InstallResult{Outcome: InstallFailed, Message: fmt.Sprintf("%s update was not verified: %s", spec.Name, problem), Status: &status}, nil
	}
	return verifiedInstallResult(spec, "managed installer", detail, runErr, refreshErr, status), nil
}

func (installer Installer) UpgradeNpm(ctx context.Context, spec Spec, available string) (InstallResult, error) {
	if spec.Install.NpmPackage == "" {
		return InstallResult{}, fmt.Errorf("tool %q has no declared npm package", spec.ID)
	}
	if _, _, ok := parseNpmVersion(available); !ok {
		return InstallResult{}, fmt.Errorf("invalid npm update version %q for %s", available, spec.Name)
	}
	if installer.Detect == nil {
		return InstallResult{}, errors.New("npm harness cannot be verified")
	}
	current := installer.Detect(ctx, spec)
	if !current.Installed || current.VersionOutput == "" {
		return InstallResult{}, fmt.Errorf("%s is not a detectable installed harness", spec.Name)
	}
	if installer.LookPath == nil {
		return InstallResult{}, fmt.Errorf("npm cannot be checked for %s", spec.Name)
	}
	if _, err := installer.LookPath("npm.cmd"); err != nil {
		return InstallResult{Outcome: InstallNpmUnavailable, Message: "npm is not available.", VendorURL: spec.Install.VendorURL}, nil
	}
	if current.ExecutablePath == "" {
		return InstallResult{}, fmt.Errorf("%s executable path is not available", spec.Name)
	}
	if installer.ConfirmUpdate == nil {
		return InstallResult{}, errors.New("tool update confirmation is not configured")
	}
	confirmed, err := installer.ConfirmUpdate(spec, available, current)
	if err != nil {
		return InstallResult{}, fmt.Errorf("confirm %s update: %w", spec.Name, err)
	}
	if !confirmed {
		return InstallResult{Outcome: InstallCancelled, Message: fmt.Sprintf("Update of %s was cancelled.", spec.Name)}, nil
	}
	if installer.RunNpm == nil {
		return InstallResult{}, fmt.Errorf("no npm runner is configured")
	}
	prefix := filepath.Dir(current.ExecutablePath)
	runErr := installer.RunNpm(ctx, NpmUpdateArgs(spec, available, prefix))
	var refreshErr error
	if installer.RefreshPath != nil {
		refreshErr = installer.RefreshPath()
	}
	status := installer.Detect(ctx, spec)
	needsUpdate, compareErr := npmVersionIsNewer(available, status.VersionOutput)
	if status.Usable() && (compareErr != nil || needsUpdate) {
		problem := fmt.Sprintf("installed version %s did not reach requested version %s", status.Version, available)
		if compareErr != nil {
			problem = compareErr.Error()
		}
		if runErr != nil {
			problem += fmt.Sprintf("; npm failed: %v", runErr)
		}
		if refreshErr != nil {
			problem += fmt.Sprintf("; PATH refresh failed: %v", refreshErr)
		}
		return InstallResult{
			Outcome: InstallFailed,
			Message: fmt.Sprintf("%s update was not verified: %s", spec.Name, problem),
			Status:  &status,
		}, nil
	}
	if !status.Usable() {
		return verifiedInstallResult(spec, "npm", "", errors.Join(runErr, compareErr), refreshErr, status), nil
	}
	return verifiedInstallResult(spec, "npm", "", runErr, refreshErr, status), nil
}
