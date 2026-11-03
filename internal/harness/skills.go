package harness

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// PluginDirArgs returns the arguments that load a plugin folder for one
// launch. The option comes from the harness manifest; the folder is a
// separate value.
func PluginDirArgs(option, dir string) []string {
	return []string{option, dir}
}

var (
	pluginProbeMu    sync.Mutex
	pluginProbeCache = map[string]bool{}
)

// SupportsPluginDir reports whether the harness executable lists the option
// in its help output. The answer is cached per executable file version and
// option so an upgrade is probed again.
func SupportsPluginDir(ctx context.Context, executable, option string) bool {
	key := executable + "|" + option
	if info, err := os.Stat(executable); err == nil {
		key += "|" + info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	pluginProbeMu.Lock()
	supported, known := pluginProbeCache[key]
	pluginProbeMu.Unlock()
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
	pluginProbeMu.Lock()
	pluginProbeCache[key] = supported
	pluginProbeMu.Unlock()
	return supported
}
