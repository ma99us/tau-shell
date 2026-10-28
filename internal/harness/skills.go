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
// launch. Both supported harnesses take the folder as a separate value.
func PluginDirArgs(dir string) []string {
	return []string{"--plugin-dir", dir}
}

var (
	pluginProbeMu    sync.Mutex
	pluginProbeCache = map[string]bool{}
)

// SupportsPluginDir reports whether the harness executable lists --plugin-dir
// in its help output. The answer is cached per executable file version so an
// upgrade is probed again.
func SupportsPluginDir(ctx context.Context, executable string) bool {
	key := executable
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
	supported = strings.Contains(string(output), "--plugin-dir")
	pluginProbeMu.Lock()
	pluginProbeCache[key] = supported
	pluginProbeMu.Unlock()
	return supported
}
