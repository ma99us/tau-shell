package harness

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SkillDirectoryArgs attaches a harness-specific skill folder for one launch.
func SkillDirectoryArgs(option, dir string) []string {
	return []string{option, dir}
}

var (
	skillOptionProbeMu    sync.Mutex
	skillOptionProbeCache = map[string]bool{}
)

// SupportsSkillDirectoryOption reports whether the executable lists the option
// in its help output. The answer is cached per executable file version and
// option so an upgrade is probed again.
func SupportsSkillDirectoryOption(ctx context.Context, executable, option string) bool {
	key := executable + "|" + option
	if info, err := os.Stat(executable); err == nil {
		key += "|" + info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	skillOptionProbeMu.Lock()
	supported, known := skillOptionProbeCache[key]
	skillOptionProbeMu.Unlock()
	if known {
		return supported
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	command := exec.CommandContext(probeCtx, executable, "--help")
	hideWindow(command)
	output, err := command.CombinedOutput()
	if err != nil && len(output) == 0 {
		return false
	}
	supported = strings.Contains(string(output), option)
	skillOptionProbeMu.Lock()
	skillOptionProbeCache[key] = supported
	skillOptionProbeMu.Unlock()
	return supported
}
