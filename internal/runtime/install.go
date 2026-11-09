package runtime

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Install actions TauShell can start. Each runs wsl.exe itself in a visible
// console; Windows asks for administrator approval where it is needed.
const (
	ActionInstallWSL    = "wsl"
	ActionInstallDistro = "distro"
	ActionUpdateWSL     = "update"
)

var onlineDistroName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// InstallArgs returns the discrete wsl.exe arguments for an action. A distro
// name must come from the online catalog and is only used by the distro action.
func InstallArgs(action, distro string) ([]string, error) {
	switch action {
	case ActionInstallWSL:
		return []string{"--install", "--no-distro"}, nil
	case ActionUpdateWSL:
		return []string{"--update"}, nil
	case ActionInstallDistro:
		if !onlineDistroName.MatchString(distro) {
			return nil, fmt.Errorf("invalid distro name %q", distro)
		}
		return []string{"--install", "-d", distro, "--no-launch"}, nil
	}
	return nil, fmt.Errorf("unknown WSL action %q", action)
}

// ParseOnlineDistros reads `wsl --list --online`. Headers are localized, so
// the entries are the lines of the last blank-separated block after its header.
func ParseOnlineDistros(text string) []string {
	var block []string
	var names []string
	flush := func() {
		if len(block) > 1 {
			names = nil
			for _, line := range block[1:] {
				if fields := strings.Fields(line); len(fields) > 0 && onlineDistroName.MatchString(fields[0]) {
					names = append(names, fields[0])
				}
			}
		}
		block = nil
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		block = append(block, line)
	}
	flush()
	return names
}

// OnlineDistros lists the distros `wsl --install -d` accepts.
func OnlineDistros(ctx context.Context, run Runner) ([]string, error) {
	if run == nil {
		run = execRunner
	}
	output, err := run(ctx, "--list", "--online")
	if err != nil {
		return nil, err
	}
	names := ParseOnlineDistros(DecodeOutput(output))
	if len(names) == 0 {
		return nil, errors.New("wsl.exe returned no installable distributions")
	}
	return names, nil
}
